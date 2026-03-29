-- name: CreateAcceptanceEvent :one
INSERT INTO acceptance_events (
  subject_ref,
  document_version_id,
  accepted_at,
  source_service,
  source_app,
  ip_address,
  user_agent,
  evidence_sha256,
  metadata
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: ListAcceptanceEventsBySubject :many
SELECT
  ae.id,
  ae.subject_ref,
  ae.accepted_at,
  ae.source_service,
  ae.source_app,
  ae.ip_address,
  ae.user_agent,
  ae.evidence_sha256,
  ae.metadata,
  ae.created_at,
  d.key AS document_key,
  d.display_name,
  d.category,
  dv.version AS document_version,
  dv.locale,
  dv.audience,
  dv.content_type,
  dv.content_sha256
FROM acceptance_events ae
JOIN document_versions dv ON dv.id = ae.document_version_id
JOIN documents d ON d.id = dv.document_id
WHERE ae.subject_ref = $1
ORDER BY ae.accepted_at DESC, ae.created_at DESC;
