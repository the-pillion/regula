-- Split lifecycle: is_published controls public visibility; is_finalized
-- locks the row from further edits. A version can be published *and*
-- still editable until the lawyer finalises it. Subprocessor snapshot
-- moves to finalise time, not publish time.
ALTER TABLE document_versions
  ADD COLUMN IF NOT EXISTS is_finalized BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_document_versions_finalized
  ON document_versions(is_finalized);
