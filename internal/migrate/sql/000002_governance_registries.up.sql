CREATE TABLE IF NOT EXISTS processors (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  key TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  relationship_type TEXT NOT NULL DEFAULT 'processor' CHECK (relationship_type IN ('processor', 'subprocessor')),
  service_area TEXT NOT NULL DEFAULT '',
  website_url TEXT NOT NULL DEFAULT '',
  primary_country TEXT NOT NULL DEFAULT '',
  data_location TEXT NOT NULL DEFAULT '',
  transfer_mechanism TEXT NOT NULL DEFAULT '',
  dpa_status TEXT NOT NULL DEFAULT 'unknown' CHECK (dpa_status IN ('unknown', 'pending', 'signed', 'not_required')),
  notes TEXT NOT NULL DEFAULT '',
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS retention_policies (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  key TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  data_category TEXT NOT NULL DEFAULT 'general',
  description TEXT NOT NULL DEFAULT '',
  retention_days INTEGER CHECK (retention_days IS NULL OR retention_days >= 0),
  trigger_event TEXT NOT NULL DEFAULT '',
  storage_scope TEXT NOT NULL DEFAULT '',
  deletion_method TEXT NOT NULL DEFAULT '',
  legal_basis TEXT NOT NULL DEFAULT '',
  notes TEXT NOT NULL DEFAULT '',
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_processors_relationship_active
  ON processors(relationship_type, is_active, display_name);

CREATE INDEX IF NOT EXISTS idx_retention_policies_category_active
  ON retention_policies(data_category, is_active, display_name);
