package store

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"
)

type CreateDashboardAuditEventParams struct {
	Actor       string
	EventType   string
	EntityType  string
	EntityID    string
	DocumentKey string
	Method      string
	Path        string
	RemoteIP    string
	UserAgent   string
	Metadata    map[string]any
}

func (q *Queries) CreateDashboardAuditEvent(ctx context.Context, p CreateDashboardAuditEventParams) error {
	metadata := []byte(`{}`)
	if p.Metadata != nil {
		b, err := json.Marshal(p.Metadata)
		if err != nil {
			return err
		}
		metadata = b
	}

	_, err := q.db.Exec(ctx, `
		INSERT INTO dashboard_audit_events (
		  actor, event_type, entity_type, entity_id, document_key,
		  method, path, remote_ip, user_agent, metadata
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb)`,
		p.Actor, p.EventType, p.EntityType, p.EntityID, p.DocumentKey,
		p.Method, p.Path, p.RemoteIP, p.UserAgent, string(metadata),
	)
	return err
}

type DashboardAuditEvent struct {
	ID          string
	Actor       string
	EventType   string
	EntityType  string
	EntityID    string
	DocumentKey string
	Method      string
	Path        string
	RemoteIP    string
	UserAgent   string
	Metadata    []byte
	CreatedAt   pgtype.Timestamptz
}

func (q *Queries) ListDashboardAuditEvents(ctx context.Context, limit int32) ([]DashboardAuditEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := q.db.Query(ctx, `
		SELECT id, actor, event_type, entity_type, entity_id, document_key,
		       method, path, remote_ip, user_agent, metadata, created_at
		FROM dashboard_audit_events
		ORDER BY created_at DESC
		LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DashboardAuditEvent
	for rows.Next() {
		var e DashboardAuditEvent
		if err := rows.Scan(
			&e.ID, &e.Actor, &e.EventType, &e.EntityType, &e.EntityID, &e.DocumentKey,
			&e.Method, &e.Path, &e.RemoteIP, &e.UserAgent, &e.Metadata, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
