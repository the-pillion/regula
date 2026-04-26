ALTER TABLE documents
  ADD COLUMN IF NOT EXISTS is_publicly_visible BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE documents
   SET is_publicly_visible = TRUE
 WHERE key IN ('privacy-policy', 'terms-of-service', 'cookie-policy', 'impressum');

CREATE INDEX IF NOT EXISTS idx_documents_public_visibility
  ON documents(is_publicly_visible)
  WHERE is_publicly_visible = TRUE;
