package dashboard

import "net/http"

func (s *Server) auditTrail(w http.ResponseWriter, r *http.Request) {
	events, err := s.queries.ListDashboardAuditEvents(r.Context(), 200)
	if err != nil {
		s.log.Error("dashboard audit list failed", "error", err)
		s.renderError(w, http.StatusInternalServerError, "could not load audit trail")
		return
	}
	s.render(w, r, "audit.html", map[string]any{
		"Title":  "Audit trail",
		"Events": events,
	})
}
