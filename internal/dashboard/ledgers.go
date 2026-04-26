package dashboard

import (
	"net/http"
	"strings"
)

// Ledgers are append-only and presented read-only here.
// The dashboard never edits acceptance_events or consent_events —
// it can only look them up by subject_ref.

func (s *Server) ledgersIndex(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "ledgers_index.html", map[string]any{
		"Title": "Ledgers (read-only)",
	})
}

func (s *Server) ledgerForSubject(w http.ResponseWriter, r *http.Request) {
	subject := strings.TrimSpace(r.URL.Query().Get("subject_ref"))
	if subject == "" {
		s.renderError(w, http.StatusBadRequest, "subject_ref is required")
		return
	}

	acceptances, err := s.queries.ListAcceptanceEventsBySubject(r.Context(), subject)
	if err != nil {
		s.log.Error("dashboard list acceptances", "error", err)
		s.renderError(w, http.StatusInternalServerError, "could not load acceptances")
		return
	}
	consents, err := s.queries.ListConsentEventsBySubject(r.Context(), subject)
	if err != nil {
		s.log.Error("dashboard list consents", "error", err)
		s.renderError(w, http.StatusInternalServerError, "could not load consents")
		return
	}
	current, err := s.queries.GetCurrentConsentsBySubject(r.Context(), subject)
	if err != nil {
		s.log.Error("dashboard current consents", "error", err)
		s.renderError(w, http.StatusInternalServerError, "could not load current consents")
		return
	}

	s.render(w, r, "ledgers_subject.html", map[string]any{
		"Title":       "Ledger — " + subject,
		"Subject":     subject,
		"Acceptances": acceptances,
		"Consents":    consents,
		"Current":     current,
	})
}
