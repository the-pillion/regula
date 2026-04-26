package dashboard

import "net/http"

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
	})
}
