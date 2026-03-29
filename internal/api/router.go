package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pillion/regula/internal/auth"
	"github.com/pillion/regula/internal/cache"
	"github.com/pillion/regula/internal/config"
	"github.com/pillion/regula/internal/store"
)

type API struct {
	cfg      *config.Config
	log      *slog.Logger
	queries  *store.Queries
	docCache *cache.LatestDocumentCache
}

func NewRouter(cfg *config.Config, log *slog.Logger, queries *store.Queries) (http.Handler, error) {
	api := &API{
		cfg:      cfg,
		log:      log,
		queries:  queries,
		docCache: cache.NewLatestDocumentCache(cfg.DocumentCacheTTL, cfg.DocumentCacheMaxItems),
	}

	validator, err := auth.NewZitadelIntrospectionValidator(auth.ZitadelIntrospectionConfig{
		Issuer:            cfg.ZitadelIssuer,
		IntrospectionURI:  cfg.ZitadelIntrospectionURI,
		CacheTTL:          cfg.ZitadelIntrospectionTTL,
		AllowedAudiences:  cfg.ZitadelAllowedAudiences,
		AllowedServiceIDs: cfg.ZitadelAllowedServiceIDs,
		APIClientID:       cfg.ZitadelAPIClientID,
		APIClientSecret:   cfg.ZitadelAPIClientSecret,
	})
	if err != nil {
		return nil, err
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", api.health)
	r.Get("/readyz", api.ready)

	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(validator))

		r.Post("/v1/documents", api.createDocument)
		r.Post("/v1/documents/{key}/versions", api.createDocumentVersion)
		r.Get("/v1/documents/{key}/versions/latest", api.getLatestDocumentVersion)

		r.Post("/v1/consent-purposes", api.upsertConsentPurpose)
		r.Post("/v1/acceptances", api.createAcceptance)
		r.Post("/v1/consents", api.createConsent)

		r.Get("/v1/subjects/{subjectRef}/acceptances", api.listAcceptances)
		r.Get("/v1/subjects/{subjectRef}/consents/history", api.listConsentHistory)
		r.Get("/v1/subjects/{subjectRef}/consents/current", api.listCurrentConsents)
		r.Get("/v1/subjects/{subjectRef}/bundle", api.getSubjectBundle)
	})

	return r, nil
}

type createDocumentRequest struct {
	Key         string `json:"key"`
	DisplayName string `json:"display_name"`
	Category    string `json:"category"`
}

type createDocumentVersionRequest struct {
	Version       string    `json:"version"`
	Locale        string    `json:"locale"`
	Audience      string    `json:"audience"`
	ContentType   string    `json:"content_type"`
	Content       string    `json:"content"`
	IsPublished   bool      `json:"is_published"`
	EffectiveFrom time.Time `json:"effective_from"`
	CreatedBy     string    `json:"created_by"`
}

type upsertConsentPurposeRequest struct {
	Key         string `json:"key"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
}

type createAcceptanceRequest struct {
	SubjectRef      string          `json:"subject_ref"`
	DocumentKey     string          `json:"document_key"`
	DocumentVersion string          `json:"document_version"`
	Locale          string          `json:"locale"`
	Audience        string          `json:"audience"`
	AcceptedAt      time.Time       `json:"accepted_at"`
	SourceService   string          `json:"source_service"`
	SourceApp       string          `json:"source_app"`
	IPAddress       string          `json:"ip_address"`
	UserAgent       string          `json:"user_agent"`
	Metadata        json.RawMessage `json:"metadata"`
}

type createConsentRequest struct {
	SubjectRef      string          `json:"subject_ref"`
	PurposeKey      string          `json:"purpose_key"`
	Status          string          `json:"status"`
	LegalBasis      string          `json:"legal_basis"`
	DocumentKey     string          `json:"document_key"`
	DocumentVersion string          `json:"document_version"`
	Locale          string          `json:"locale"`
	Audience        string          `json:"audience"`
	ChangedAt       time.Time       `json:"changed_at"`
	SourceService   string          `json:"source_service"`
	SourceApp       string          `json:"source_app"`
	Metadata        json.RawMessage `json:"metadata"`
}

func (api *API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": api.cfg.ServiceName})
}

func (api *API) ready(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
}

func (api *API) createDocument(w http.ResponseWriter, r *http.Request) {
	var req createDocumentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	req.Key = normalizeKey(req.Key)
	if req.Key == "" || strings.TrimSpace(req.DisplayName) == "" {
		writeError(w, http.StatusBadRequest, errors.New("key and display_name are required"))
		return
	}
	if strings.TrimSpace(req.Category) == "" {
		req.Category = "legal"
	}

	doc, err := api.queries.CreateDocument(r.Context(), store.CreateDocumentParams{
		Key:         req.Key,
		DisplayName: strings.TrimSpace(req.DisplayName),
		Category:    strings.TrimSpace(req.Category),
	})
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}

	writeJSON(w, http.StatusCreated, doc)
}

func (api *API) createDocumentVersion(w http.ResponseWriter, r *http.Request) {
	key := normalizeKey(chi.URLParam(r, "key"))
	if key == "" {
		writeError(w, http.StatusBadRequest, errors.New("document key is required"))
		return
	}

	var req createDocumentVersionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	doc, err := api.queries.GetDocumentByKey(r.Context(), key)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	if req.Version == "" || req.Locale == "" || req.Content == "" {
		writeError(w, http.StatusBadRequest, errors.New("version, locale, and content are required"))
		return
	}
	if req.Audience == "" {
		req.Audience = "all"
	}

	contentType, err := normalizeContentType(req.ContentType)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if req.CreatedBy == "" {
		req.CreatedBy = api.cfg.ServiceName
	}
	if req.EffectiveFrom.IsZero() {
		req.EffectiveFrom = time.Now().UTC()
	}

	version, err := api.queries.CreateDocumentVersion(r.Context(), store.CreateDocumentVersionParams{
		DocumentID:    doc.ID,
		Version:       strings.TrimSpace(req.Version),
		Locale:        strings.ToLower(strings.TrimSpace(req.Locale)),
		Audience:      strings.ToLower(strings.TrimSpace(req.Audience)),
		ContentType:   contentType,
		ContentText:   req.Content,
		ContentSha256: sha256Hex(req.Content),
		IsPublished:   req.IsPublished,
		EffectiveFrom: asTimestamptz(req.EffectiveFrom),
		CreatedBy:     strings.TrimSpace(req.CreatedBy),
	})
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}

	api.invalidateDocumentCache(key)
	writeJSON(w, http.StatusCreated, version)
}

func (api *API) getLatestDocumentVersion(w http.ResponseWriter, r *http.Request) {
	key := normalizeKey(chi.URLParam(r, "key"))
	locale := strings.ToLower(defaultString(r.URL.Query().Get("locale"), "en"))
	audience := strings.ToLower(defaultString(r.URL.Query().Get("audience"), "all"))
	cacheKey := buildLatestVersionCacheKey(key, locale, audience)

	if cached, ok := api.docCache.Get(cacheKey); ok {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	version, err := api.queries.GetLatestPublishedDocumentVersion(r.Context(), store.GetLatestPublishedDocumentVersionParams{
		Key:      key,
		Locale:   locale,
		Audience: audience,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	api.docCache.Set(cacheKey, version)
	writeJSON(w, http.StatusOK, version)
}

func (api *API) upsertConsentPurpose(w http.ResponseWriter, r *http.Request) {
	var req upsertConsentPurposeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	req.Key = normalizeKey(req.Key)
	if req.Key == "" || strings.TrimSpace(req.DisplayName) == "" {
		writeError(w, http.StatusBadRequest, errors.New("key and display_name are required"))
		return
	}

	purpose, err := api.queries.UpsertConsentPurpose(r.Context(), store.UpsertConsentPurposeParams{
		Key:         req.Key,
		DisplayName: strings.TrimSpace(req.DisplayName),
		Description: strings.TrimSpace(req.Description),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, purpose)
}

func (api *API) createAcceptance(w http.ResponseWriter, r *http.Request) {
	var req createAcceptanceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	version, err := api.lookupDocumentVersion(r, req.DocumentKey, req.DocumentVersion, req.Locale, req.Audience)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	if req.SubjectRef == "" || req.SourceService == "" {
		writeError(w, http.StatusBadRequest, errors.New("subject_ref and source_service are required"))
		return
	}
	if req.AcceptedAt.IsZero() {
		req.AcceptedAt = time.Now().UTC()
	}
	metadata := ensureJSON(req.Metadata)

	event, err := api.queries.CreateAcceptanceEvent(r.Context(), store.CreateAcceptanceEventParams{
		SubjectRef:        strings.TrimSpace(req.SubjectRef),
		DocumentVersionID: version.ID,
		AcceptedAt:        asTimestamptz(req.AcceptedAt),
		SourceService:     strings.TrimSpace(req.SourceService),
		SourceApp:         strings.TrimSpace(req.SourceApp),
		IpAddress:         strings.TrimSpace(req.IPAddress),
		UserAgent:         strings.TrimSpace(req.UserAgent),
		EvidenceSha256:    sha256Hex(fmt.Sprintf("%s|%s|%s|%s|%s", req.SubjectRef, req.DocumentKey, req.DocumentVersion, req.SourceService, req.AcceptedAt.UTC().Format(time.RFC3339Nano))),
		Metadata:          metadata,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusCreated, event)
}

func (api *API) createConsent(w http.ResponseWriter, r *http.Request) {
	var req createConsentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if req.SubjectRef == "" || req.SourceService == "" || req.PurposeKey == "" {
		writeError(w, http.StatusBadRequest, errors.New("subject_ref, source_service, and purpose_key are required"))
		return
	}
	if req.Status != "granted" && req.Status != "revoked" {
		writeError(w, http.StatusBadRequest, errors.New("status must be granted or revoked"))
		return
	}
	if req.LegalBasis == "" {
		req.LegalBasis = "consent"
	}
	if req.ChangedAt.IsZero() {
		req.ChangedAt = time.Now().UTC()
	}

	purpose, err := api.queries.GetConsentPurposeByKey(r.Context(), normalizeKey(req.PurposeKey))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, fmt.Errorf("unknown consent purpose %q — register it via POST /v1/consent-purposes first", req.PurposeKey))
		return
	}

	version, err := api.lookupDocumentVersion(r, req.DocumentKey, req.DocumentVersion, req.Locale, req.Audience)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	event, err := api.queries.CreateConsentEvent(r.Context(), store.CreateConsentEventParams{
		SubjectRef:        strings.TrimSpace(req.SubjectRef),
		ConsentPurposeID:  purpose.ID,
		DocumentVersionID: version.ID,
		Status:            req.Status,
		LegalBasis:        strings.TrimSpace(req.LegalBasis),
		ChangedAt:         asTimestamptz(req.ChangedAt),
		SourceService:     strings.TrimSpace(req.SourceService),
		SourceApp:         strings.TrimSpace(req.SourceApp),
		EvidenceSha256:    sha256Hex(fmt.Sprintf("%s|%s|%s|%s|%s", req.SubjectRef, req.PurposeKey, req.Status, req.SourceService, req.ChangedAt.UTC().Format(time.RFC3339Nano))),
		Metadata:          ensureJSON(req.Metadata),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusCreated, event)
}

func (api *API) listAcceptances(w http.ResponseWriter, r *http.Request) {
	subjectRef := chi.URLParam(r, "subjectRef")
	rows, err := api.queries.ListAcceptanceEventsBySubject(r.Context(), subjectRef)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (api *API) listConsentHistory(w http.ResponseWriter, r *http.Request) {
	subjectRef := chi.URLParam(r, "subjectRef")
	rows, err := api.queries.ListConsentEventsBySubject(r.Context(), subjectRef)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (api *API) listCurrentConsents(w http.ResponseWriter, r *http.Request) {
	subjectRef := chi.URLParam(r, "subjectRef")
	rows, err := api.queries.GetCurrentConsentsBySubject(r.Context(), subjectRef)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (api *API) getSubjectBundle(w http.ResponseWriter, r *http.Request) {
	subjectRef := chi.URLParam(r, "subjectRef")
	acceptances, err := api.queries.ListAcceptanceEventsBySubject(r.Context(), subjectRef)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	consentHistory, err := api.queries.ListConsentEventsBySubject(r.Context(), subjectRef)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	currentConsents, err := api.queries.GetCurrentConsentsBySubject(r.Context(), subjectRef)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"subject_ref":      subjectRef,
		"acceptances":      acceptances,
		"consent_history":  consentHistory,
		"current_consents": currentConsents,
	})
}

func (api *API) lookupDocumentVersion(r *http.Request, documentKey, documentVersion, locale, audience string) (store.DocumentVersion, error) {
	if normalizeKey(documentKey) == "" || strings.TrimSpace(documentVersion) == "" || strings.TrimSpace(locale) == "" {
		return store.DocumentVersion{}, errors.New("document_key, document_version, and locale are required")
	}

	if strings.TrimSpace(audience) == "" {
		audience = "all"
	}

	return api.queries.GetDocumentVersionByNaturalKey(r.Context(), store.GetDocumentVersionByNaturalKeyParams{
		Key:      normalizeKey(documentKey),
		Version:  strings.TrimSpace(documentVersion),
		Locale:   strings.ToLower(strings.TrimSpace(locale)),
		Audience: strings.ToLower(strings.TrimSpace(audience)),
	})
}

func (api *API) invalidateDocumentCache(documentKey string) {
	api.docCache.InvalidatePrefix(normalizeKey(documentKey) + "|")
}

func decodeJSON(r *http.Request, dst any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if decoder.More() {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

func ensureJSON(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return []byte(`{}`)
	}
	return raw
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{
		"error":   http.StatusText(status),
		"message": err.Error(),
	})
}

func normalizeKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func normalizeContentType(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "html", nil
	}

	switch normalized {
	case "html", "text/html":
		return "html", nil
	case "markdown", "md", "text/markdown":
		return "markdown", nil
	default:
		return "", fmt.Errorf("unsupported content_type %q: use html or markdown", value)
	}
}

func buildLatestVersionCacheKey(documentKey, locale, audience string) string {
	return normalizeKey(documentKey) + "|" + strings.ToLower(strings.TrimSpace(locale)) + "|" + strings.ToLower(strings.TrimSpace(audience))
}

func asTimestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func sha256Hex(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
