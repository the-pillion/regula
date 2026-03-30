-- name: CreateDocument :one
INSERT INTO documents (key, display_name, category)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpsertDocument :one
INSERT INTO documents (key, display_name, category)
VALUES ($1, $2, $3)
ON CONFLICT (key) DO UPDATE
SET display_name = EXCLUDED.display_name,
    category = EXCLUDED.category
RETURNING *;

-- name: GetDocumentByKey :one
SELECT *
FROM documents
WHERE key = $1
LIMIT 1;

-- name: CreateDocumentVersion :one
INSERT INTO document_versions (
  document_id,
  version,
  locale,
  audience,
  content_type,
  content_text,
  content_sha256,
  is_published,
  effective_from,
  created_by
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: UpsertDocumentVersion :one
INSERT INTO document_versions (
  document_id,
  version,
  locale,
  audience,
  content_type,
  content_text,
  content_sha256,
  is_published,
  effective_from,
  created_by
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (document_id, version, locale, audience) DO UPDATE
SET content_type = EXCLUDED.content_type,
    content_text = EXCLUDED.content_text,
    content_sha256 = EXCLUDED.content_sha256,
    is_published = EXCLUDED.is_published,
    effective_from = EXCLUDED.effective_from,
    created_by = EXCLUDED.created_by
RETURNING *;

-- name: GetDocumentVersionByNaturalKey :one
SELECT dv.*
FROM document_versions dv
JOIN documents d ON d.id = dv.document_id
WHERE d.key = $1
  AND dv.version = $2
  AND dv.locale = $3
  AND dv.audience = $4
LIMIT 1;

-- name: ListDocumentVersionsByKey :many
SELECT
  d.key AS document_key,
  d.display_name,
  dv.id,
  dv.version,
  dv.locale,
  dv.audience,
  dv.content_type,
  dv.is_published,
  dv.effective_from,
  dv.created_by,
  dv.created_at
FROM document_versions dv
JOIN documents d ON d.id = dv.document_id
WHERE d.key = $1
ORDER BY dv.effective_from DESC, dv.created_at DESC;

-- name: GetLatestPublishedDocumentVersion :one
SELECT
  d.key AS document_key,
  d.display_name,
  d.category,
  dv.id,
  dv.version,
  dv.locale,
  dv.audience,
  dv.content_type,
  dv.content_text,
  dv.content_sha256,
  dv.is_published,
  dv.effective_from,
  dv.created_by,
  dv.created_at
FROM document_versions dv
JOIN documents d ON d.id = dv.document_id
WHERE d.key = $1
  AND dv.locale = $2
  AND dv.audience = $3
  AND dv.is_published = TRUE
  AND dv.effective_from <= NOW()
ORDER BY dv.effective_from DESC, dv.created_at DESC
LIMIT 1;
