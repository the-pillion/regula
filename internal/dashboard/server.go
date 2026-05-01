package dashboard

import (
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pillion/regula/internal/config"
	"github.com/pillion/regula/internal/store"
)

// Server is the admin dashboard. Owns templates, queries, the pool
// (needed for transactional writes — publish and processor revisions),
// and a back-pointer to the root router used only by the routes view.
type Server struct {
	cfg     config.DashboardConfig
	log     *slog.Logger
	queries *store.Queries
	pool    *pgxpool.Pool
	tpl     *templates
	root    chi.Router
}

func NewServer(cfg config.DashboardConfig, log *slog.Logger, queries *store.Queries, pool *pgxpool.Pool, root chi.Router) (*Server, error) {
	tpl, err := loadTemplates()
	if err != nil {
		return nil, err
	}
	return &Server{
		cfg:     cfg,
		log:     log,
		queries: queries,
		pool:    pool,
		tpl:     tpl,
		root:    root,
	}, nil
}

func (s *Server) Mount(r chi.Router) {
	r.Get("/", s.home)
	r.Handle("/assets/*", dashboardAssets())

	r.Get("/documents", s.listDocuments)
	r.Get("/documents/new", s.newDocumentForm)
	r.Post("/documents", s.createDocument)
	r.Post("/documents/{key}/visibility", s.toggleVisibility)
	r.Get("/documents/{key}", s.documentDetail)
	r.Get("/documents/{key}/versions/new", s.newVersionForm)
	r.Post("/documents/{key}/versions", s.createVersion)
	r.Get("/documents/{key}/versions/{id}", s.editVersion)
	r.Post("/documents/{key}/versions/{id}", s.updateVersion)
	r.Post("/documents/{key}/versions/{id}/publish", s.publishVersion)
	r.Post("/documents/{key}/versions/{id}/unpublish", s.unpublishVersion)
	r.Post("/documents/{key}/versions/{id}/finalize", s.finalizeVersion)
	r.Post("/documents/{key}/versions/{id}/delete", s.deleteVersion)

	r.Get("/processors", s.listProcessors)
	r.Get("/processors/new", s.newProcessorForm)
	r.Post("/processors", s.createProcessor)
	r.Get("/processors/{id}", s.editProcessorForm)
	r.Post("/processors/{id}", s.updateProcessor)
	r.Get("/processors/{id}/history", s.processorHistory)
	r.Get("/processors/{id}/usage", s.processorUsage)

	r.Get("/ledgers", s.ledgersIndex)
	r.Get("/ledgers/subject", s.ledgerForSubject)
	r.Get("/audit", s.auditTrail)
	r.Get("/routes", s.routesOverview)
}

func dashboardAssets() http.Handler {
	sub, err := fs.Sub(assetFS, "assets")
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.StripPrefix("/admin/assets/", http.FileServer(http.FS(sub)))
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "home.html", map[string]any{"Title": "Regula admin"})
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
