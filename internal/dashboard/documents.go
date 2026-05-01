package dashboard

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pillion/regula/internal/store"
)

func (s *Server) listDocuments(w http.ResponseWriter, r *http.Request) {
	items, err := s.queries.ListDocumentsWithVisibility(r.Context())
	if err != nil {
		s.log.Error("dashboard list documents", "error", err)
		s.renderError(w, http.StatusInternalServerError, "could not list documents")
		return
	}
	s.render(w, r, "documents.html", map[string]any{
		"Title":     "Documents",
		"Documents": items,
		"Flash":     r.URL.Query().Get("flash"),
	})
}

func (s *Server) toggleVisibility(w http.ResponseWriter, r *http.Request) {
	key := normalizeKey(chi.URLParam(r, "key"))
	if key == "" {
		s.renderError(w, http.StatusBadRequest, "key is required")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderError(w, http.StatusBadRequest, "invalid form")
		return
	}
	visible := r.FormValue("visible") == "true"
	if err := s.queries.SetDocumentVisibility(r.Context(), key, visible); err != nil {
		s.log.Error("dashboard set visibility", "key", key, "error", err)
		s.renderError(w, http.StatusInternalServerError, "could not update visibility")
		return
	}
	s.log.Info("dashboard document visibility changed",
		"event", "dashboard_visibility_change",
		"document_key", key,
		"is_publicly_visible", visible,
		"actor", basicAuthUser(r),
	)
	s.audit(r, auditEvent{
		Type:        "document.visibility_changed",
		EntityType:  "document",
		EntityID:    key,
		DocumentKey: key,
		Metadata: map[string]any{
			"is_publicly_visible": visible,
		},
	})
	http.Redirect(w, r, "/admin/documents?flash="+url.QueryEscape("visibility updated"), http.StatusSeeOther)
}

func (s *Server) newDocumentForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "document_new.html", map[string]any{
		"Title": "New document",
	})
}

func (s *Server) createDocument(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderError(w, http.StatusBadRequest, "invalid form")
		return
	}
	key := normalizeKey(r.FormValue("key"))
	name := strings.TrimSpace(r.FormValue("display_name"))
	category := strings.TrimSpace(r.FormValue("category"))
	if category == "" {
		category = "legal"
	}
	if key == "" || name == "" {
		s.renderError(w, http.StatusBadRequest, "key and display_name are required")
		return
	}

	if _, err := s.queries.CreateDocument(r.Context(), store.CreateDocumentParams{
		Key:         key,
		DisplayName: name,
		Category:    category,
	}); err != nil {
		s.log.Error("dashboard create document", "error", err)
		s.renderError(w, http.StatusConflict, "could not create document — key may already exist")
		return
	}

	s.log.Info("dashboard document created", "event", "dashboard_document_create",
		"document_key", key, "actor", basicAuthUser(r))
	s.audit(r, auditEvent{
		Type:        "document.created",
		EntityType:  "document",
		EntityID:    key,
		DocumentKey: key,
		Metadata: map[string]any{
			"display_name": name,
			"category":     category,
		},
	})
	http.Redirect(w, r, "/admin/documents/"+key+"?flash="+url.QueryEscape("document created"), http.StatusSeeOther)
}

func (s *Server) documentDetail(w http.ResponseWriter, r *http.Request) {
	key := normalizeKey(chi.URLParam(r, "key"))
	doc, err := s.queries.GetDocumentByKey(r.Context(), key)
	if err != nil {
		s.renderError(w, http.StatusNotFound, "document not found")
		return
	}
	versions, err := s.queries.ListDocumentVersionsForKey(r.Context(), key)
	if err != nil {
		s.log.Error("dashboard list versions", "error", err)
		s.renderError(w, http.StatusInternalServerError, "could not list versions")
		return
	}
	s.render(w, r, "document_detail.html", map[string]any{
		"Title":    doc.DisplayName,
		"Document": doc,
		"Versions": versions,
		"Flash":    r.URL.Query().Get("flash"),
	})
}

func (s *Server) newVersionForm(w http.ResponseWriter, r *http.Request) {
	key := normalizeKey(chi.URLParam(r, "key"))
	doc, err := s.queries.GetDocumentByKey(r.Context(), key)
	if err != nil {
		s.renderError(w, http.StatusNotFound, "document not found")
		return
	}
	s.render(w, r, "document_version_new.html", map[string]any{
		"Title":        "New version — " + doc.DisplayName,
		"Document":     doc,
		"Locale":       r.URL.Query().Get("locale"),
		"Audience":     r.URL.Query().Get("audience"),
		"EditorAssets": true,
	})
}

func (s *Server) createVersion(w http.ResponseWriter, r *http.Request) {
	key := normalizeKey(chi.URLParam(r, "key"))
	doc, err := s.queries.GetDocumentByKey(r.Context(), key)
	if err != nil {
		s.renderError(w, http.StatusNotFound, "document not found")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderError(w, http.StatusBadRequest, "invalid form")
		return
	}
	version := strings.TrimSpace(r.FormValue("version"))
	locale := strings.ToLower(strings.TrimSpace(r.FormValue("locale")))
	audience := strings.ToLower(strings.TrimSpace(r.FormValue("audience")))
	body := r.FormValue("content_text")
	if version == "" || locale == "" || body == "" {
		s.renderError(w, http.StatusBadRequest, "version, locale, content_text are required")
		return
	}
	if audience == "" {
		audience = "all"
	}

	row, err := s.queries.CreateDocumentVersion(r.Context(), store.CreateDocumentVersionParams{
		DocumentID:    doc.ID,
		Version:       version,
		Locale:        locale,
		Audience:      audience,
		ContentType:   "html",
		ContentText:   body,
		ContentSha256: sha256Hex(body),
		IsPublished:   false,
		EffectiveFrom: nowTS(),
		CreatedBy:     basicAuthUser(r),
	})
	if err != nil {
		s.log.Error("dashboard create version", "error", err)
		s.renderError(w, http.StatusConflict, "could not create version (duplicate?)")
		return
	}

	s.log.Info("dashboard version created", "event", "dashboard_version_create",
		"document_key", key, "version", version, "locale", locale, "audience", audience,
		"actor", basicAuthUser(r))
	s.audit(r, auditEvent{
		Type:        "document_version.created",
		EntityType:  "document_version",
		EntityID:    row.ID,
		DocumentKey: key,
		Metadata: map[string]any{
			"version":  version,
			"locale":   locale,
			"audience": audience,
		},
	})
	http.Redirect(w, r, "/admin/documents/"+key+"/versions/"+row.ID+"?flash="+url.QueryEscape("draft created"), http.StatusSeeOther)
}

func (s *Server) editVersion(w http.ResponseWriter, r *http.Request) {
	key := normalizeKey(chi.URLParam(r, "key"))
	id := chi.URLParam(r, "id")
	d, err := s.queries.GetDocumentVersionByID(r.Context(), id)
	if err != nil || d.DocumentKey != key {
		s.renderError(w, http.StatusNotFound, "version not found")
		return
	}
	editorContent, err := editorHTML(d)
	if err != nil {
		s.log.Error("dashboard render editor content", "version_id", id, "error", err)
		s.renderError(w, http.StatusInternalServerError, "could not prepare editor")
		return
	}
	processors, _ := s.queries.ListProcessorsForDocumentVersion(r.Context(), id)
	s.render(w, r, "document_version_edit.html", map[string]any{
		"Title":         d.DocumentName + " — " + d.Version + " (" + d.Locale + ")",
		"Version":       d,
		"EditorContent": editorContent,
		"Processors":    processors,
		"Flash":         r.URL.Query().Get("flash"),
		"EditorAssets":  true,
	})
}

func (s *Server) updateVersion(w http.ResponseWriter, r *http.Request) {
	key := normalizeKey(chi.URLParam(r, "key"))
	id := chi.URLParam(r, "id")
	d, err := s.queries.GetDocumentVersionByID(r.Context(), id)
	if err != nil || d.DocumentKey != key {
		s.renderError(w, http.StatusNotFound, "version not found")
		return
	}
	if d.IsFinalized {
		s.renderError(w, http.StatusConflict, "version is finalised — create a new version instead")
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderError(w, http.StatusBadRequest, "invalid form")
		return
	}
	body := r.FormValue("content_text")
	if body == "" {
		s.renderError(w, http.StatusBadRequest, "content_text is required")
		return
	}
	if err := s.queries.UpdateDocumentVersionDraft(r.Context(), store.UpdateDocumentVersionDraftParams{
		ID:            id,
		ContentType:   "html",
		ContentText:   body,
		ContentSha256: sha256Hex(body),
		EffectiveFrom: nowTS(),
		CreatedBy:     basicAuthUser(r),
	}); err != nil {
		if errors.Is(err, store.ErrAlreadyFinalized) {
			s.renderError(w, http.StatusConflict, err.Error())
			return
		}
		s.log.Error("dashboard update version", "error", err)
		s.renderError(w, http.StatusInternalServerError, "could not save")
		return
	}
	s.log.Info("dashboard version updated", "event", "dashboard_version_update",
		"document_key", key, "version_id", id, "actor", basicAuthUser(r))
	s.audit(r, auditEvent{
		Type:        "document_version.updated",
		EntityType:  "document_version",
		EntityID:    id,
		DocumentKey: key,
		Metadata: map[string]any{
			"version":        d.Version,
			"locale":         d.Locale,
			"audience":       d.Audience,
			"content_sha256": sha256Hex(body),
		},
	})
	http.Redirect(w, r, "/admin/documents/"+key+"/versions/"+id+"?flash="+url.QueryEscape("saved"), http.StatusSeeOther)
}

func (s *Server) publishVersion(w http.ResponseWriter, r *http.Request) {
	s.togglePublished(w, r, true, "published")
}

func (s *Server) unpublishVersion(w http.ResponseWriter, r *http.Request) {
	s.togglePublished(w, r, false, "unpublished")
}

func (s *Server) togglePublished(w http.ResponseWriter, r *http.Request, want bool, label string) {
	key := normalizeKey(chi.URLParam(r, "key"))
	id := chi.URLParam(r, "id")
	d, err := s.queries.GetDocumentVersionByID(r.Context(), id)
	if err != nil || d.DocumentKey != key {
		s.renderError(w, http.StatusNotFound, "version not found")
		return
	}
	if err := s.queries.SetDocumentVersionPublished(r.Context(), id, want); err != nil {
		if errors.Is(err, store.ErrAlreadyFinalized) {
			s.renderError(w, http.StatusConflict, err.Error())
			return
		}
		s.log.Error("dashboard toggle published", "error", err)
		s.renderError(w, http.StatusInternalServerError, "could not update")
		return
	}
	s.log.Info("dashboard version visibility changed", "event", "dashboard_version_publish",
		"document_key", key, "version_id", id, "version", d.Version,
		"locale", d.Locale, "audience", d.Audience, "is_published", want,
		"actor", basicAuthUser(r))
	s.audit(r, auditEvent{
		Type:        "document_version.publication_changed",
		EntityType:  "document_version",
		EntityID:    id,
		DocumentKey: key,
		Metadata: map[string]any{
			"version":      d.Version,
			"locale":       d.Locale,
			"audience":     d.Audience,
			"is_published": want,
		},
	})
	http.Redirect(w, r, "/admin/documents/"+key+"/versions/"+id+"?flash="+url.QueryEscape(label), http.StatusSeeOther)
}

func (s *Server) finalizeVersion(w http.ResponseWriter, r *http.Request) {
	key := normalizeKey(chi.URLParam(r, "key"))
	id := chi.URLParam(r, "id")
	d, err := s.queries.GetDocumentVersionByID(r.Context(), id)
	if err != nil || d.DocumentKey != key {
		s.renderError(w, http.StatusNotFound, "version not found")
		return
	}
	if d.IsFinalized {
		http.Redirect(w, r, "/admin/documents/"+key+"/versions/"+id, http.StatusSeeOther)
		return
	}
	if err := store.FinalizeDocumentVersion(r.Context(), s.pool, id); err != nil {
		s.log.Error("dashboard finalize version", "error", err)
		s.renderError(w, http.StatusInternalServerError, "could not finalise")
		return
	}
	s.log.Info("dashboard version finalised", "event", "dashboard_version_finalize",
		"document_key", key, "version_id", id, "version", d.Version,
		"locale", d.Locale, "audience", d.Audience, "actor", basicAuthUser(r))
	s.audit(r, auditEvent{
		Type:        "document_version.finalized",
		EntityType:  "document_version",
		EntityID:    id,
		DocumentKey: key,
		Metadata: map[string]any{
			"version":  d.Version,
			"locale":   d.Locale,
			"audience": d.Audience,
		},
	})
	http.Redirect(w, r, "/admin/documents/"+key+"/versions/"+id+"?flash="+url.QueryEscape("finalised — locked & subprocessor snapshot taken"), http.StatusSeeOther)
}

func (s *Server) deleteVersion(w http.ResponseWriter, r *http.Request) {
	key := normalizeKey(chi.URLParam(r, "key"))
	id := chi.URLParam(r, "id")
	d, err := s.queries.GetDocumentVersionByID(r.Context(), id)
	if err != nil || d.DocumentKey != key {
		s.renderError(w, http.StatusNotFound, "version not found")
		return
	}
	if d.IsFinalized {
		s.renderError(w, http.StatusConflict, "finalised versions cannot be deleted")
		return
	}
	if err := s.queries.DeleteDocumentVersion(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrAlreadyFinalized) {
			s.renderError(w, http.StatusConflict, "finalised versions cannot be deleted")
			return
		}
		s.log.Error("dashboard delete version", "error", err)
		s.renderError(w, http.StatusInternalServerError, "could not delete version")
		return
	}
	s.log.Info("dashboard version deleted", "event", "dashboard_version_delete",
		"document_key", key, "version_id", id, "version", d.Version,
		"locale", d.Locale, "audience", d.Audience, "actor", basicAuthUser(r))
	s.audit(r, auditEvent{
		Type:        "document_version.deleted",
		EntityType:  "document_version",
		EntityID:    id,
		DocumentKey: key,
		Metadata: map[string]any{
			"version":  d.Version,
			"locale":   d.Locale,
			"audience": d.Audience,
		},
	})
	http.Redirect(w, r, "/admin/documents/"+key+"?flash="+url.QueryEscape("version deleted"), http.StatusSeeOther)
}

func basicAuthUser(r *http.Request) string {
	user, _, _ := r.BasicAuth()
	if user == "" {
		return "dashboard"
	}
	return user
}

func normalizeKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func sha256Hex(value string) string {
	d := sha256.Sum256([]byte(value))
	return hex.EncodeToString(d[:])
}

func nowTS() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
}

func editorHTML(d store.DocumentVersionDetail) (string, error) {
	switch strings.ToLower(strings.TrimSpace(d.ContentType)) {
	case "", "html":
		return d.ContentText, nil
	case "markdown":
		rendered, err := renderMarkdown(d.ContentText)
		if err != nil {
			return "", err
		}
		return string(rendered), nil
	default:
		return "", fmt.Errorf("unsupported content type %q", d.ContentType)
	}
}
