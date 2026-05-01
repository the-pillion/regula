CREATE TABLE IF NOT EXISTS dashboard_audit_events (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  actor TEXT NOT NULL,
  event_type TEXT NOT NULL,
  entity_type TEXT NOT NULL DEFAULT '',
  entity_id TEXT NOT NULL DEFAULT '',
  document_key TEXT NOT NULL DEFAULT '',
  method TEXT NOT NULL DEFAULT '',
  path TEXT NOT NULL DEFAULT '',
  remote_ip TEXT NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dashboard_audit_events_created_at
  ON dashboard_audit_events(created_at DESC);

CREATE INDEX IF NOT EXISTS idx_dashboard_audit_events_entity
  ON dashboard_audit_events(entity_type, entity_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_dashboard_audit_events_document_key
  ON dashboard_audit_events(document_key, created_at DESC);
