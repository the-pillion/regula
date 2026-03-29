CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS documents (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  key TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  category TEXT NOT NULL DEFAULT 'legal',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS document_versions (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  version TEXT NOT NULL,
  locale TEXT NOT NULL,
  audience TEXT NOT NULL DEFAULT 'all',
  content_type TEXT NOT NULL DEFAULT 'markdown',
  content_text TEXT NOT NULL,
  content_sha256 TEXT NOT NULL,
  is_published BOOLEAN NOT NULL DEFAULT FALSE,
  effective_from TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_by TEXT NOT NULL DEFAULT 'system',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(document_id, version, locale, audience)
);

CREATE TABLE IF NOT EXISTS consent_purposes (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  key TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS acceptance_events (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  subject_ref TEXT NOT NULL,
  document_version_id TEXT NOT NULL REFERENCES document_versions(id) ON DELETE RESTRICT,
  accepted_at TIMESTAMPTZ NOT NULL,
  source_service TEXT NOT NULL,
  source_app TEXT NOT NULL DEFAULT '',
  ip_address TEXT NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT '',
  evidence_sha256 TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS consent_events (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  subject_ref TEXT NOT NULL,
  consent_purpose_id TEXT NOT NULL REFERENCES consent_purposes(id) ON DELETE RESTRICT,
  document_version_id TEXT NOT NULL REFERENCES document_versions(id) ON DELETE RESTRICT,
  status TEXT NOT NULL CHECK (status IN ('granted', 'revoked')),
  legal_basis TEXT NOT NULL DEFAULT 'consent',
  changed_at TIMESTAMPTZ NOT NULL,
  source_service TEXT NOT NULL,
  source_app TEXT NOT NULL DEFAULT '',
  evidence_sha256 TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_document_versions_lookup
  ON document_versions(document_id, locale, audience, is_published, effective_from DESC);
CREATE INDEX IF NOT EXISTS idx_acceptance_events_subject_time
  ON acceptance_events(subject_ref, accepted_at DESC, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_consent_events_subject_time
  ON consent_events(subject_ref, changed_at DESC, created_at DESC);
