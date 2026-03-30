CREATE TABLE IF NOT EXISTS processing_activities (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  key TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  purpose TEXT NOT NULL DEFAULT '',
  legal_basis TEXT NOT NULL DEFAULT '',
  data_subject_categories TEXT NOT NULL DEFAULT '',
  personal_data_categories TEXT NOT NULL DEFAULT '',
  recipient_categories TEXT NOT NULL DEFAULT '',
  transfer_notes TEXT NOT NULL DEFAULT '',
  retention_summary TEXT NOT NULL DEFAULT '',
  security_measures TEXT NOT NULL DEFAULT '',
  owner TEXT NOT NULL DEFAULT '',
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS dpia_records (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  key TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'in_review', 'approved', 'retired')),
  summary TEXT NOT NULL DEFAULT '',
  scope TEXT NOT NULL DEFAULT '',
  risk_level TEXT NOT NULL DEFAULT 'medium' CHECK (risk_level IN ('low', 'medium', 'high', 'very_high')),
  mitigating_measures TEXT NOT NULL DEFAULT '',
  owner TEXT NOT NULL DEFAULT '',
  review_due_at TIMESTAMPTZ,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_processing_activities_active
  ON processing_activities(is_active, display_name);

CREATE INDEX IF NOT EXISTS idx_dpia_records_status_active
  ON dpia_records(status, is_active, display_name);
