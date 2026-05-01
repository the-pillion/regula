-- processor_revisions: point-in-time snapshots of a processor record.
-- Every create/update writes a new row here. The processors table stays
-- as the "current" view; revisions are append-only audit history.
CREATE TABLE IF NOT EXISTS processor_revisions (
  id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
  processor_id TEXT NOT NULL REFERENCES processors(id) ON DELETE CASCADE,
  display_name TEXT NOT NULL,
  relationship_type TEXT NOT NULL CHECK (relationship_type IN ('processor', 'subprocessor')),
  service_area TEXT NOT NULL DEFAULT '',
  website_url TEXT NOT NULL DEFAULT '',
  primary_country TEXT NOT NULL DEFAULT '',
  data_location TEXT NOT NULL DEFAULT '',
  transfer_mechanism TEXT NOT NULL DEFAULT '',
  dpa_status TEXT NOT NULL DEFAULT 'unknown' CHECK (dpa_status IN ('unknown', 'pending', 'signed', 'not_required')),
  notes TEXT NOT NULL DEFAULT '',
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  changed_by TEXT NOT NULL DEFAULT 'system',
  change_reason TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_processor_revisions_processor_time
  ON processor_revisions(processor_id, created_at DESC);

-- document_version_processors: which processor revision was attached to
-- which document version at publish time. Snapshot, not live link — old
-- doc versions keep referencing the revision that was current when they
-- were published, even after the processor is later edited.
CREATE TABLE IF NOT EXISTS document_version_processors (
  document_version_id TEXT NOT NULL REFERENCES document_versions(id) ON DELETE CASCADE,
  processor_revision_id TEXT NOT NULL REFERENCES processor_revisions(id) ON DELETE RESTRICT,
  attached_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (document_version_id, processor_revision_id)
);

CREATE INDEX IF NOT EXISTS idx_dvp_revision
  ON document_version_processors(processor_revision_id);

-- Backfill: every existing processor gets one initial revision so the
-- foreign key target exists for any future attach. Without this, the
-- first publish after deploy would have no revision to point at.
INSERT INTO processor_revisions (
  processor_id, display_name, relationship_type, service_area, website_url,
  primary_country, data_location, transfer_mechanism, dpa_status, notes,
  is_active, changed_by, change_reason, created_at
)
SELECT
  id, display_name, relationship_type, service_area, website_url,
  primary_country, data_location, transfer_mechanism, dpa_status, notes,
  is_active, 'migration', 'initial backfill', created_at
FROM processors
WHERE NOT EXISTS (
  SELECT 1 FROM processor_revisions pr WHERE pr.processor_id = processors.id
);
