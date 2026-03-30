-- name: UpsertProcessor :one
INSERT INTO processors (
  key,
  display_name,
  relationship_type,
  service_area,
  website_url,
  primary_country,
  data_location,
  transfer_mechanism,
  dpa_status,
  notes,
  is_active,
  updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
ON CONFLICT (key) DO UPDATE
SET display_name = EXCLUDED.display_name,
    relationship_type = EXCLUDED.relationship_type,
    service_area = EXCLUDED.service_area,
    website_url = EXCLUDED.website_url,
    primary_country = EXCLUDED.primary_country,
    data_location = EXCLUDED.data_location,
    transfer_mechanism = EXCLUDED.transfer_mechanism,
    dpa_status = EXCLUDED.dpa_status,
    notes = EXCLUDED.notes,
    is_active = EXCLUDED.is_active,
    updated_at = NOW()
RETURNING *;

-- name: ListProcessors :many
SELECT *
FROM processors
ORDER BY is_active DESC, display_name ASC, created_at ASC;

-- name: UpsertRetentionPolicy :one
INSERT INTO retention_policies (
  key,
  display_name,
  data_category,
  description,
  retention_days,
  trigger_event,
  storage_scope,
  deletion_method,
  legal_basis,
  notes,
  is_active,
  updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
ON CONFLICT (key) DO UPDATE
SET display_name = EXCLUDED.display_name,
    data_category = EXCLUDED.data_category,
    description = EXCLUDED.description,
    retention_days = EXCLUDED.retention_days,
    trigger_event = EXCLUDED.trigger_event,
    storage_scope = EXCLUDED.storage_scope,
    deletion_method = EXCLUDED.deletion_method,
    legal_basis = EXCLUDED.legal_basis,
    notes = EXCLUDED.notes,
    is_active = EXCLUDED.is_active,
    updated_at = NOW()
RETURNING *;

-- name: ListRetentionPolicies :many
SELECT *
FROM retention_policies
ORDER BY is_active DESC, data_category ASC, display_name ASC, created_at ASC;

-- name: UpsertProcessingActivity :one
INSERT INTO processing_activities (
  key,
  display_name,
  purpose,
  legal_basis,
  data_subject_categories,
  personal_data_categories,
  recipient_categories,
  transfer_notes,
  retention_summary,
  security_measures,
  owner,
  is_active,
  updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())
ON CONFLICT (key) DO UPDATE
SET display_name = EXCLUDED.display_name,
    purpose = EXCLUDED.purpose,
    legal_basis = EXCLUDED.legal_basis,
    data_subject_categories = EXCLUDED.data_subject_categories,
    personal_data_categories = EXCLUDED.personal_data_categories,
    recipient_categories = EXCLUDED.recipient_categories,
    transfer_notes = EXCLUDED.transfer_notes,
    retention_summary = EXCLUDED.retention_summary,
    security_measures = EXCLUDED.security_measures,
    owner = EXCLUDED.owner,
    is_active = EXCLUDED.is_active,
    updated_at = NOW()
RETURNING *;

-- name: ListProcessingActivities :many
SELECT *
FROM processing_activities
ORDER BY is_active DESC, display_name ASC, created_at ASC;

-- name: UpsertDPIARecord :one
INSERT INTO dpia_records (
  key,
  display_name,
  status,
  summary,
  scope,
  risk_level,
  mitigating_measures,
  owner,
  review_due_at,
  is_active,
  updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
ON CONFLICT (key) DO UPDATE
SET display_name = EXCLUDED.display_name,
    status = EXCLUDED.status,
    summary = EXCLUDED.summary,
    scope = EXCLUDED.scope,
    risk_level = EXCLUDED.risk_level,
    mitigating_measures = EXCLUDED.mitigating_measures,
    owner = EXCLUDED.owner,
    review_due_at = EXCLUDED.review_due_at,
    is_active = EXCLUDED.is_active,
    updated_at = NOW()
RETURNING *;

-- name: ListDPIARecords :many
SELECT *
FROM dpia_records
ORDER BY is_active DESC, status ASC, display_name ASC, created_at ASC;
