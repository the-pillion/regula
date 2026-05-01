package dashboard

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/pillion/regula/internal/store"
)

func (s *Server) listProcessors(w http.ResponseWriter, r *http.Request) {
	rows, err := s.queries.ListProcessors(r.Context())
	if err != nil {
		s.log.Error("dashboard list processors", "error", err)
		s.renderError(w, http.StatusInternalServerError, "could not list processors")
		return
	}
	s.render(w, r, "processors.html", map[string]any{
		"Title":      "Processors & subprocessors",
		"Processors": rows,
		"Flash":      r.URL.Query().Get("flash"),
	})
}

func (s *Server) newProcessorForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "processor_edit.html", map[string]any{
		"Title": "New processor",
		"New":   true,
	})
}

func (s *Server) createProcessor(w http.ResponseWriter, r *http.Request) {
	p, err := parseProcessorForm(r)
	if err != nil {
		s.renderError(w, http.StatusBadRequest, err.Error())
		return
	}
	p.ChangedBy = basicAuthUser(r)
	id, err := store.UpsertProcessorWithRevision(r.Context(), s.pool, p)
	if err != nil {
		s.log.Error("dashboard create processor", "error", err)
		s.renderError(w, http.StatusInternalServerError, "could not save processor")
		return
	}
	s.log.Info("dashboard processor created", "event", "dashboard_processor_create",
		"processor_key", p.Key, "actor", basicAuthUser(r))
	s.audit(r, auditEvent{
		Type:       "processor.created",
		EntityType: "processor",
		EntityID:   id,
		Metadata: map[string]any{
			"key":               p.Key,
			"relationship_type": p.RelationshipType,
			"is_active":         p.IsActive,
			"dpa_status":        p.DPAStatus,
		},
	})
	http.Redirect(w, r, "/admin/processors/"+id+"?flash="+url.QueryEscape("processor created"), http.StatusSeeOther)
}

func (s *Server) editProcessorForm(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	row, err := s.queries.GetProcessorByID(r.Context(), id)
	if err != nil {
		s.renderError(w, http.StatusNotFound, "processor not found")
		return
	}
	s.render(w, r, "processor_edit.html", map[string]any{
		"Title":     row.DisplayName,
		"Processor": row,
		"Flash":     r.URL.Query().Get("flash"),
	})
}

func (s *Server) updateProcessor(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	row, err := s.queries.GetProcessorByID(r.Context(), id)
	if err != nil {
		s.renderError(w, http.StatusNotFound, "processor not found")
		return
	}
	p, err := parseProcessorForm(r)
	if err != nil {
		s.renderError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Force the existing key — key is the natural identity, not editable here.
	p.Key = row.Key
	p.ChangedBy = basicAuthUser(r)
	if _, err := store.UpsertProcessorWithRevision(r.Context(), s.pool, p); err != nil {
		s.log.Error("dashboard update processor", "error", err)
		s.renderError(w, http.StatusInternalServerError, "could not save processor")
		return
	}
	s.log.Info("dashboard processor updated", "event", "dashboard_processor_update",
		"processor_key", p.Key, "change_reason", p.ChangeReason, "actor", basicAuthUser(r))
	s.audit(r, auditEvent{
		Type:       "processor.updated",
		EntityType: "processor",
		EntityID:   id,
		Metadata: map[string]any{
			"key":               p.Key,
			"relationship_type": p.RelationshipType,
			"is_active":         p.IsActive,
			"dpa_status":        p.DPAStatus,
			"change_reason":     p.ChangeReason,
		},
	})
	http.Redirect(w, r, "/admin/processors/"+id+"?flash="+url.QueryEscape("revision saved"), http.StatusSeeOther)
}

func (s *Server) processorHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	row, err := s.queries.GetProcessorByID(r.Context(), id)
	if err != nil {
		s.renderError(w, http.StatusNotFound, "processor not found")
		return
	}
	revs, err := s.queries.ListProcessorRevisions(r.Context(), id)
	if err != nil {
		s.log.Error("dashboard list revisions", "error", err)
		s.renderError(w, http.StatusInternalServerError, "could not load history")
		return
	}
	s.render(w, r, "processor_history.html", map[string]any{
		"Title":     row.DisplayName + " — history",
		"Processor": row,
		"Revisions": revs,
	})
}

func (s *Server) processorUsage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	row, err := s.queries.GetProcessorByID(r.Context(), id)
	if err != nil {
		s.renderError(w, http.StatusNotFound, "processor not found")
		return
	}
	usage, err := s.queries.ListDocumentVersionsForProcessor(r.Context(), id)
	if err != nil {
		s.log.Error("dashboard processor usage", "error", err)
		s.renderError(w, http.StatusInternalServerError, "could not load usage")
		return
	}
	s.render(w, r, "processor_usage.html", map[string]any{
		"Title":     row.DisplayName + " — used in",
		"Processor": row,
		"Usage":     usage,
	})
}

// parseProcessorForm validates and normalises the multipart form fields
// shared by create + update. Defaults match the DB CHECK constraints.
func parseProcessorForm(r *http.Request) (store.UpsertProcessorWithRevisionParams, error) {
	if err := r.ParseForm(); err != nil {
		return store.UpsertProcessorWithRevisionParams{}, err
	}
	get := func(k string) string { return strings.TrimSpace(r.FormValue(k)) }
	p := store.UpsertProcessorWithRevisionParams{
		Key:               normalizeKey(get("key")),
		DisplayName:       get("display_name"),
		RelationshipType:  normaliseRel(get("relationship_type")),
		ServiceArea:       get("service_area"),
		WebsiteURL:        get("website_url"),
		PrimaryCountry:    get("primary_country"),
		DataLocation:      get("data_location"),
		TransferMechanism: get("transfer_mechanism"),
		DPAStatus:         normaliseDPA(get("dpa_status")),
		Notes:             get("notes"),
		IsActive:          get("is_active") != "false",
		ChangeReason:      get("change_reason"),
	}
	if p.Key == "" || p.DisplayName == "" {
		return p, &formError{msg: "key and display_name are required"}
	}
	return p, nil
}

func normaliseRel(v string) string {
	if strings.ToLower(v) == "subprocessor" {
		return "subprocessor"
	}
	return "processor"
}

func normaliseDPA(v string) string {
	switch strings.ToLower(v) {
	case "pending", "signed", "not_required":
		return strings.ToLower(v)
	default:
		return "unknown"
	}
}

type formError struct{ msg string }

func (e *formError) Error() string { return e.msg }
