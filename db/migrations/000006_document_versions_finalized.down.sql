DROP INDEX IF EXISTS idx_document_versions_finalized;
ALTER TABLE document_versions DROP COLUMN IF EXISTS is_finalized;
