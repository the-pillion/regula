package dashboard

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/pillion/regula/internal/config"
	"github.com/pillion/regula/internal/store"
)

// Server is the admin dashboard. It is intentionally small:
// it owns templates, queries, and a back-pointer to the root router
// (used only by the routes-overview page). Handlers live in
// sibling files (documents.go, processors.go, ledgers.go, routes.go)
// and are kept narrow on purpose — no god-handler.
type Server struct {
	cfg     config.DashboardConfig
	log     *slog.Logger
	queries *store.Queries
	tpl     *templates
	root    chi.Router
}

func NewServer(cfg config.DashboardConfig, log *slog.Logger, queries *store.Queries, root chi.Router) (*Server, error) {
	tpl, err := loadTemplates()
	if err != nil {
		return nil, err
	}
	return &Server{
		cfg:     cfg,
		log:     log,
		queries: queries,
		tpl:     tpl,
		root:    root,
	}, nil
}

// Mount attaches dashboard routes under the supplied router group.
// The caller is responsible for applying BasicAuth middleware on
// the same group.
func (s *Server) Mount(r chi.Router) {
	r.Get("/", s.home)
	r.Get("/documents", s.listDocuments)
	r.Post("/documents/{key}/visibility", s.toggleVisibility)
	r.Get("/processors", s.listProcessors)
	r.Get("/ledgers", s.ledgersIndex)
	r.Get("/ledgers/subject", s.ledgerForSubject)
	r.Get("/routes", s.routesOverview)
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "home.html", map[string]any{
		"Title": "Regula admin",
	})
}

func (s *Server) render(w http.ResponseWriter, _ *http.Request, name string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	if _, ok := data["Title"]; !ok {
		data["Title"] = "Regula admin"
	}
	if err := s.tpl.render(w, name, data); err != nil {
		s.log.Error("dashboard render failed", "template", name, "error", err)
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}

func (s *Server) renderError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	_ = s.tpl.render(w, "error.html", map[string]any{
		"Title":   "Error",
		"Status":  status,
		"Message": msg,
	})
}
