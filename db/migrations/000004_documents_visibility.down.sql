DROP INDEX IF EXISTS idx_documents_public_visibility;
ALTER TABLE documents DROP COLUMN IF EXISTS is_publicly_visible;
