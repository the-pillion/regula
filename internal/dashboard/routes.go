package dashboard

import (
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
)

// RouteEntry is one row in the routes-overview page.
type RouteEntry struct {
	Method     string
	Pattern    string
	Visibility string // "public", "internal", "admin", "infra"
}

func (s *Server) routesOverview(w http.ResponseWriter, r *http.Request) {
	entries := walkRoutes(s.root)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Pattern == entries[j].Pattern {
			return entries[i].Method < entries[j].Method
		}
		return entries[i].Pattern < entries[j].Pattern
	})

	publicKeys, err := s.queries.ListPublicDocumentKeys(r.Context())
	if err != nil {
		s.log.Error("dashboard list public keys", "error", err)
		// Render anyway with empty list — routes view stays useful.
		publicKeys = nil
	}

	s.render(w, r, "routes.html", map[string]any{
		"Title":              "Routes",
		"Routes":             entries,
		"PublicDocumentKeys": publicKeys,
	})
}

// walkRoutes flattens a chi router into a list of (method, pattern, visibility).
// Visibility is derived from the pattern prefix — cheap and accurate
// because the router enforces those prefixes.
func walkRoutes(router chi.Router) []RouteEntry {
	var entries []RouteEntry
	walker := func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		entries = append(entries, RouteEntry{
			Method:     method,
			Pattern:    route,
			Visibility: classify(route),
		})
		return nil
	}
	if err := chi.Walk(router, walker); err != nil {
		return entries
	}
	return entries
}

func classify(pattern string) string {
	switch {
	case strings.HasPrefix(pattern, "/public/"):
		return "public"
	case strings.HasPrefix(pattern, "/v1/"):
		return "internal"
	case strings.HasPrefix(pattern, "/admin"):
		return "admin"
	default:
		return "infra"
	}
}
