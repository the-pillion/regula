package api

import (
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pillion/regula/internal/store"
)

// Public read-only surface.
//
// Anonymous, GET-only, published-only, whitelisted-keys-only.
// Never exposes acceptance/consent ledgers, processor internal notes,
// DPA status, retention policy details, Article 30 records, or DPIA records.

func (api *API) isPublicLegalKey(key string) bool {
	key = normalizeKey(key)
	for _, allowed := range api.cfg.PublicLegalKeys {
		if key == allowed {
			return true
		}
	}
	return false
}

func (api *API) publicCacheHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		maxAge := int(api.cfg.PublicCacheMaxAge.Seconds())
		sMaxAge := int(api.cfg.PublicCacheSharedMaxAge.Seconds())
		swr := int(api.cfg.PublicCacheStaleRevalidate.Seconds())
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d, s-maxage=%d, stale-while-revalidate=%d", maxAge, sMaxAge, swr))
		w.Header().Set("Vary", "Accept-Language, Origin")
		next.ServeHTTP(w, r)
	})
}

func (api *API) publicCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && api.isAllowedOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Accept-Language")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (api *API) isAllowedOrigin(origin string) bool {
	for _, allowed := range api.cfg.PublicCORSAllowedOrigins {
		if origin == allowed {
			return true
		}
	}
	return false
}

func splitExt(suffix string) (key, ext string) {
	idx := strings.LastIndex(suffix, ".")
	if idx <= 0 {
		return suffix, ""
	}
	return suffix[:idx], strings.ToLower(suffix[idx+1:])
}

func (api *API) getPublicLegalDocument(w http.ResponseWriter, r *http.Request) {
	rawKey := chi.URLParam(r, "keyExt")
	key, ext := splitExt(rawKey)
	key = normalizeKey(key)

	if !api.isPublicLegalKey(key) {
		writeError(w, http.StatusNotFound, errors.New("document not found"))
		return
	}

	locale := strings.ToLower(defaultString(r.URL.Query().Get("lang"), defaultString(r.URL.Query().Get("locale"), "en")))
	audience := strings.ToLower(defaultString(r.URL.Query().Get("audience"), "all"))

	cacheKey := buildLatestVersionCacheKey(key, locale, audience)
	version, ok := api.docCache.Get(cacheKey)
	if !ok {
		v, err := api.queries.GetLatestPublishedDocumentVersion(r.Context(), store.GetLatestPublishedDocumentVersionParams{
			Key:      key,
			Locale:   locale,
			Audience: audience,
		})
		if err != nil {
			writeError(w, http.StatusNotFound, errors.New("no published version for the requested locale"))
			return
		}
		version = v
		api.docCache.Set(cacheKey, version)
	}

	writePublicDocument(w, r, ext, version)
}

func (api *API) getPublicLegalDocumentPinned(w http.ResponseWriter, r *http.Request) {
	key := normalizeKey(chi.URLParam(r, "key"))
	rawVersion := chi.URLParam(r, "versionExt")
	versionStr, ext := splitExt(rawVersion)

	if !api.isPublicLegalKey(key) {
		writeError(w, http.StatusNotFound, errors.New("document not found"))
		return
	}
	if strings.TrimSpace(versionStr) == "" {
		writeError(w, http.StatusBadRequest, errors.New("version is required"))
		return
	}

	locale := strings.ToLower(defaultString(r.URL.Query().Get("lang"), defaultString(r.URL.Query().Get("locale"), "en")))
	audience := strings.ToLower(defaultString(r.URL.Query().Get("audience"), "all"))

	dv, err := api.queries.GetDocumentVersionByNaturalKey(r.Context(), store.GetDocumentVersionByNaturalKeyParams{
		Key:      key,
		Version:  versionStr,
		Locale:   locale,
		Audience: audience,
	})
	if err != nil || !dv.IsPublished {
		writeError(w, http.StatusNotFound, errors.New("no published version found"))
		return
	}

	doc, err := api.queries.GetDocumentByKey(r.Context(), key)
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("document not found"))
		return
	}

	row := store.GetLatestPublishedDocumentVersionRow{
		DocumentKey:   doc.Key,
		DisplayName:   doc.DisplayName,
		Category:      doc.Category,
		ID:            dv.ID,
		Version:       dv.Version,
		Locale:        dv.Locale,
		Audience:      dv.Audience,
		ContentType:   dv.ContentType,
		ContentText:   dv.ContentText,
		ContentSha256: dv.ContentSha256,
		IsPublished:   dv.IsPublished,
		EffectiveFrom: dv.EffectiveFrom,
		CreatedBy:     dv.CreatedBy,
		CreatedAt:     dv.CreatedAt,
	}

	writePublicDocument(w, r, ext, row)
}

func (api *API) listPublicLegalVersions(w http.ResponseWriter, r *http.Request) {
	key := normalizeKey(chi.URLParam(r, "key"))
	if !api.isPublicLegalKey(key) {
		writeError(w, http.StatusNotFound, errors.New("document not found"))
		return
	}

	rows, err := api.queries.ListDocumentVersionsByKey(r.Context(), key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	wantLocale := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("lang")))
	if wantLocale == "" {
		wantLocale = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("locale")))
	}

	type publicVersion struct {
		Version       string    `json:"version"`
		Locale        string    `json:"locale"`
		Audience      string    `json:"audience"`
		ContentType   string    `json:"content_type"`
		EffectiveFrom time.Time `json:"effective_from"`
	}

	out := make([]publicVersion, 0, len(rows))
	for _, row := range rows {
		if !row.IsPublished {
			continue
		}
		if wantLocale != "" && row.Locale != wantLocale {
			continue
		}
		if !row.EffectiveFrom.Valid || row.EffectiveFrom.Time.After(time.Now().UTC()) {
			continue
		}
		out = append(out, publicVersion{
			Version:       row.Version,
			Locale:        row.Locale,
			Audience:      row.Audience,
			ContentType:   row.ContentType,
			EffectiveFrom: row.EffectiveFrom.Time,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"document_key": key,
		"items":        out,
	})
}

func writePublicDocument(w http.ResponseWriter, r *http.Request, ext string, row store.GetLatestPublishedDocumentVersionRow) {
	w.Header().Set("ETag", `"`+row.ContentSha256+`"`)

	if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, row.ContentSha256) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	switch ext {
	case "html":
		if row.ContentType == "html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(row.ContentText))
			return
		}
		// markdown stored, html requested: serve as preformatted to avoid silent transformation
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<pre>" + html.EscapeString(row.ContentText) + "</pre>"))
	case "md", "markdown":
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = w.Write([]byte(row.ContentText))
	case "json", "":
		writeJSON(w, http.StatusOK, map[string]any{
			"document_key":   row.DocumentKey,
			"display_name":   row.DisplayName,
			"version":        row.Version,
			"locale":         row.Locale,
			"audience":       row.Audience,
			"content_type":   row.ContentType,
			"content":        row.ContentText,
			"content_sha256": row.ContentSha256,
			"effective_from": row.EffectiveFrom.Time,
		})
	default:
		writeError(w, http.StatusNotFound, errors.New("unsupported format: use html, md, or json"))
	}
}

func (api *API) getPublicSubprocessors(w http.ResponseWriter, r *http.Request) {
	ext := "json"
	if strings.HasSuffix(r.URL.Path, ".html") {
		ext = "html"
	}

	rows, err := api.queries.ListProcessors(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	type publicProcessor struct {
		Key              string `json:"key"`
		DisplayName      string `json:"display_name"`
		RelationshipType string `json:"relationship_type"`
		ServiceArea      string `json:"service_area"`
		WebsiteURL       string `json:"website_url"`
		PrimaryCountry   string `json:"primary_country"`
		DataLocation     string `json:"data_location"`
	}

	out := make([]publicProcessor, 0, len(rows))
	for _, row := range rows {
		if !row.IsActive {
			continue
		}
		out = append(out, publicProcessor{
			Key:              row.Key,
			DisplayName:      row.DisplayName,
			RelationshipType: row.RelationshipType,
			ServiceArea:      row.ServiceArea,
			WebsiteURL:       row.WebsiteUrl,
			PrimaryCountry:   row.PrimaryCountry,
			DataLocation:     row.DataLocation,
		})
	}

	switch ext {
	case "json":
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	case "html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		var b strings.Builder
		b.WriteString(`<table class="regula-subprocessors"><thead><tr>`)
		b.WriteString(`<th>Name</th><th>Type</th><th>Service area</th><th>Country</th><th>Data location</th><th>Website</th>`)
		b.WriteString(`</tr></thead><tbody>`)
		for _, p := range out {
			b.WriteString("<tr>")
			b.WriteString("<td>" + html.EscapeString(p.DisplayName) + "</td>")
			b.WriteString("<td>" + html.EscapeString(p.RelationshipType) + "</td>")
			b.WriteString("<td>" + html.EscapeString(p.ServiceArea) + "</td>")
			b.WriteString("<td>" + html.EscapeString(p.PrimaryCountry) + "</td>")
			b.WriteString("<td>" + html.EscapeString(p.DataLocation) + "</td>")
			if p.WebsiteURL != "" {
				b.WriteString(`<td><a href="` + html.EscapeString(p.WebsiteURL) + `" rel="noopener noreferrer" target="_blank">` + html.EscapeString(p.WebsiteURL) + `</a></td>`)
			} else {
				b.WriteString("<td></td>")
			}
			b.WriteString("</tr>")
		}
		b.WriteString("</tbody></table>")
		_, _ = w.Write([]byte(b.String()))
	default:
		writeError(w, http.StatusNotFound, errors.New("unsupported format: use html or json"))
	}
}
