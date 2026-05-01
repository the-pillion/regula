package dashboard

import (
	"net/http"

	"github.com/pillion/regula/internal/store"
)

type auditEvent struct {
	Type        string
	EntityType  string
	EntityID    string
	DocumentKey string
	Metadata    map[string]any
}

func (s *Server) audit(r *http.Request, event auditEvent) {
	if err := s.queries.CreateDashboardAuditEvent(r.Context(), store.CreateDashboardAuditEventParams{
		Actor:       basicAuthUser(r),
		EventType:   event.Type,
		EntityType:  event.EntityType,
		EntityID:    event.EntityID,
		DocumentKey: event.DocumentKey,
		Method:      r.Method,
		Path:        r.URL.Path,
		RemoteIP:    r.RemoteAddr,
		UserAgent:   r.UserAgent(),
		Metadata:    event.Metadata,
	}); err != nil {
		s.log.Error("dashboard audit write failed", "event", event.Type, "error", err)
	}
}
