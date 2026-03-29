-- name: GetConsentPurposeByKey :one
SELECT id, key, display_name, description, created_at
FROM consent_purposes
WHERE key = $1;

-- name: UpsertConsentPurpose :one
INSERT INTO consent_purposes (key, display_name, description)
VALUES ($1, $2, $3)
ON CONFLICT (key) DO UPDATE
SET display_name = EXCLUDED.display_name,
    description = EXCLUDED.description
RETURNING *;

-- name: CreateConsentEvent :one
INSERT INTO consent_events (
  subject_ref,
  consent_purpose_id,
  document_version_id,
  status,
  legal_basis,
  changed_at,
  source_service,
  source_app,
  evidence_sha256,
  metadata
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: ListConsentEventsBySubject :many
SELECT
  ce.id,
  ce.subject_ref,
  ce.status,
  ce.legal_basis,
  ce.changed_at,
  ce.source_service,
  ce.source_app,
  ce.evidence_sha256,
  ce.metadata,
  ce.created_at,
  cp.key AS purpose_key,
  cp.display_name AS purpose_display_name,
  cp.description AS purpose_description,
  d.key AS document_key,
  d.display_name,
  dv.version AS document_version,
  dv.locale,
  dv.audience
FROM consent_events ce
JOIN consent_purposes cp ON cp.id = ce.consent_purpose_id
JOIN document_versions dv ON dv.id = ce.document_version_id
JOIN documents d ON d.id = dv.document_id
WHERE ce.subject_ref = $1
ORDER BY ce.changed_at DESC, ce.created_at DESC;

-- name: GetCurrentConsentsBySubject :many
SELECT DISTINCT ON (cp.key)
  ce.id,
  ce.subject_ref,
  ce.status,
  ce.legal_basis,
  ce.changed_at,
  ce.source_service,
  ce.source_app,
  ce.evidence_sha256,
  ce.metadata,
  ce.created_at,
  cp.key AS purpose_key,
  cp.display_name AS purpose_display_name,
  cp.description AS purpose_description,
  d.key AS document_key,
  d.display_name,
  dv.version AS document_version,
  dv.locale,
  dv.audience
FROM consent_events ce
JOIN consent_purposes cp ON cp.id = ce.consent_purpose_id
JOIN document_versions dv ON dv.id = ce.document_version_id
JOIN documents d ON d.id = dv.document_id
WHERE ce.subject_ref = $1
ORDER BY cp.key, ce.changed_at DESC, ce.created_at DESC;
