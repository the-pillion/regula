package dashboard

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
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

// toggleVisibility flips documents.is_publicly_visible for one key.
// POST-only; redirects back to the list. Form supplies "visible"=true|false.
func (s *Server) toggleVisibility(w http.ResponseWriter, r *http.Request) {
	key := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "key")))
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

	flash := "visibility updated"
	http.Redirect(w, r, "/admin/documents?flash="+flash, http.StatusSeeOther)
}

func basicAuthUser(r *http.Request) string {
	user, _, _ := r.BasicAuth()
	return user
}
