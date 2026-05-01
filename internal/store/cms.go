package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Hand-written queries for the dashboard CMS surface (document editor +
// processor revisions). Kept out of sqlc-generated files so regenerating
// sqlc never clobbers them. The transactional ones (publish + processor
// upsert-with-revision) take a *pgxpool.Pool directly because they need
// BEGIN/COMMIT semantics.

// ----- Documents -----

type DocumentVersionListItem struct {
	ID            string             `json:"id"`
	DocumentKey   string             `json:"document_key"`
	DocumentName  string             `json:"document_name"`
	Version       string             `json:"version"`
	Locale        string             `json:"locale"`
	Audience      string             `json:"audience"`
	ContentType   string             `json:"content_type"`
	IsPublished   bool               `json:"is_published"`
	IsFinalized   bool               `json:"is_finalized"`
	EffectiveFrom pgtype.Timestamptz `json:"effective_from"`
	CreatedBy     string             `json:"created_by"`
	CreatedAt     pgtype.Timestamptz `json:"created_at"`
	ProcessorN    int64              `json:"processor_count"`
}

const listDocumentVersionsForKey = `
SELECT
  dv.id,
  d.key,
  d.display_name,
  dv.version,
  dv.locale,
  dv.audience,
  dv.content_type,
  dv.is_published,
  dv.is_finalized,
  dv.effective_from,
  dv.created_by,
  dv.created_at,
  COALESCE(p.n, 0) AS processor_count
FROM document_versions dv
JOIN documents d ON d.id = dv.document_id
LEFT JOIN (
  SELECT document_version_id, COUNT(*) AS n
  FROM document_version_processors
  GROUP BY document_version_id
) p ON p.document_version_id = dv.id
WHERE d.key = $1
ORDER BY dv.locale, dv.audience, dv.effective_from DESC, dv.created_at DESC
`

func (q *Queries) ListDocumentVersionsForKey(ctx context.Context, key string) ([]DocumentVersionListItem, error) {
	rows, err := q.db.Query(ctx, listDocumentVersionsForKey, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []DocumentVersionListItem
	for rows.Next() {
		var i DocumentVersionListItem
		if err := rows.Scan(
			&i.ID, &i.DocumentKey, &i.DocumentName,
			&i.Version, &i.Locale, &i.Audience, &i.ContentType,
			&i.IsPublished, &i.IsFinalized, &i.EffectiveFrom, &i.CreatedBy, &i.CreatedAt,
			&i.ProcessorN,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

const getDocumentVersionByID = `
SELECT dv.id, dv.document_id, d.key, d.display_name,
       dv.version, dv.locale, dv.audience, dv.content_type,
       dv.content_text, dv.content_sha256, dv.is_published, dv.is_finalized,
       dv.effective_from, dv.created_by, dv.created_at
FROM document_versions dv
JOIN documents d ON d.id = dv.document_id
WHERE dv.id = $1
`

type DocumentVersionDetail struct {
	ID            string
	DocumentID    string
	DocumentKey   string
	DocumentName  string
	Version       string
	Locale        string
	Audience      string
	ContentType   string
	ContentText   string
	ContentSha256 string
	IsPublished   bool
	IsFinalized   bool
	EffectiveFrom pgtype.Timestamptz
	CreatedBy     string
	CreatedAt     pgtype.Timestamptz
}

func (q *Queries) GetDocumentVersionByID(ctx context.Context, id string) (DocumentVersionDetail, error) {
	var d DocumentVersionDetail
	err := q.db.QueryRow(ctx, getDocumentVersionByID, id).Scan(
		&d.ID, &d.DocumentID, &d.DocumentKey, &d.DocumentName,
		&d.Version, &d.Locale, &d.Audience, &d.ContentType,
		&d.ContentText, &d.ContentSha256, &d.IsPublished, &d.IsFinalized,
		&d.EffectiveFrom, &d.CreatedBy, &d.CreatedAt,
	)
	return d, err
}

// updateDocumentVersionEditable mutates a row only while is_finalized is
// FALSE. Published rows are still editable until finalisation — that is
// the v0 ergonomics trade-off: a finalised version becomes the immutable
// audit anchor; everything before that is fair game for the lawyer.
const updateDocumentVersionEditable = `
UPDATE document_versions
SET content_type = $2,
    content_text = $3,
    content_sha256 = $4,
    effective_from = $5,
    created_by = $6
WHERE id = $1
  AND is_finalized = FALSE
RETURNING id
`

type UpdateDocumentVersionDraftParams struct {
	ID            string
	ContentType   string
	ContentText   string
	ContentSha256 string
	EffectiveFrom pgtype.Timestamptz
	CreatedBy     string
}

var ErrAlreadyFinalized = errors.New("document version is finalised; create a new version instead")

func (q *Queries) UpdateDocumentVersionDraft(ctx context.Context, p UpdateDocumentVersionDraftParams) error {
	var id string
	err := q.db.QueryRow(ctx, updateDocumentVersionEditable,
		p.ID, p.ContentType, p.ContentText, p.ContentSha256, p.EffectiveFrom, p.CreatedBy,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAlreadyFinalized
	}
	return err
}

// SetDocumentVersionPublished flips is_published. Public surface honours
// this flag immediately. Editable until finalisation.
func (q *Queries) SetDocumentVersionPublished(ctx context.Context, versionID string, published bool) error {
	var id string
	err := q.db.QueryRow(ctx,
		`UPDATE document_versions SET is_published = $2 WHERE id = $1 AND is_finalized = FALSE RETURNING id`,
		versionID, published,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAlreadyFinalized
	}
	return err
}

// DeleteDocumentVersion removes a non-finalised document version. Finalised
// versions are immutable legal evidence and must remain in the ledger.
func (q *Queries) DeleteDocumentVersion(ctx context.Context, versionID string) error {
	var id string
	err := q.db.QueryRow(ctx,
		`DELETE FROM document_versions
		 WHERE id = $1 AND is_finalized = FALSE
		 RETURNING id`,
		versionID,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAlreadyFinalized
	}
	return err
}

// FinalizeDocumentVersion locks the row and snapshots every active
// processor's current revision into document_version_processors. After
// this call the row is immutable; subprocessor history is also locked.
func FinalizeDocumentVersion(ctx context.Context, pool *pgxpool.Pool, versionID string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var id string
	err = tx.QueryRow(ctx,
		`UPDATE document_versions
		 SET is_finalized = TRUE, is_published = TRUE
		 WHERE id = $1 AND is_finalized = FALSE
		 RETURNING id`,
		versionID,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAlreadyFinalized
	}
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO document_version_processors (document_version_id, processor_revision_id)
		 SELECT $1, pr.id
		 FROM processors p
		 JOIN LATERAL (
		   SELECT id FROM processor_revisions
		   WHERE processor_id = p.id
		   ORDER BY created_at DESC LIMIT 1
		 ) pr ON TRUE
		 WHERE p.is_active = TRUE
		 ON CONFLICT DO NOTHING`,
		versionID,
	); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

const listProcessorsForDocumentVersion = `
SELECT pr.id, pr.processor_id, p.key, pr.display_name, pr.relationship_type,
       pr.service_area, pr.website_url, pr.primary_country, pr.data_location,
       pr.transfer_mechanism, pr.dpa_status, pr.notes, pr.is_active,
       pr.changed_by, pr.change_reason, pr.created_at
FROM document_version_processors dvp
JOIN processor_revisions pr ON pr.id = dvp.processor_revision_id
JOIN processors p ON p.id = pr.processor_id
WHERE dvp.document_version_id = $1
ORDER BY pr.relationship_type, pr.display_name
`

type ProcessorRevisionView struct {
	ID                string
	ProcessorID       string
	ProcessorKey      string
	DisplayName       string
	RelationshipType  string
	ServiceArea       string
	WebsiteURL        string
	PrimaryCountry    string
	DataLocation      string
	TransferMechanism string
	DPAStatus         string
	Notes             string
	IsActive          bool
	ChangedBy         string
	ChangeReason      string
	CreatedAt         pgtype.Timestamptz
}

func (q *Queries) ListProcessorsForDocumentVersion(ctx context.Context, versionID string) ([]ProcessorRevisionView, error) {
	return scanRevisionViews(q.db.Query(ctx, listProcessorsForDocumentVersion, versionID))
}

// ----- Processors with revisions -----

type UpsertProcessorWithRevisionParams struct {
	Key               string
	DisplayName       string
	RelationshipType  string
	ServiceArea       string
	WebsiteURL        string
	PrimaryCountry    string
	DataLocation      string
	TransferMechanism string
	DPAStatus         string
	Notes             string
	IsActive          bool
	ChangedBy         string
	ChangeReason      string
}

// UpsertProcessorWithRevision creates or updates a processor row and
// always writes a new processor_revisions row capturing the new state.
// The revision history is what lets old document versions keep
// referencing accurate point-in-time data after the live row is edited.
func UpsertProcessorWithRevision(ctx context.Context, pool *pgxpool.Pool, p UpsertProcessorWithRevisionParams) (string, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var processorID string
	err = tx.QueryRow(ctx, `
		INSERT INTO processors (
		  key, display_name, relationship_type, service_area, website_url,
		  primary_country, data_location, transfer_mechanism, dpa_status,
		  notes, is_active, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NOW())
		ON CONFLICT (key) DO UPDATE SET
		  display_name = EXCLUDED.display_name,
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
		RETURNING id`,
		p.Key, p.DisplayName, p.RelationshipType, p.ServiceArea, p.WebsiteURL,
		p.PrimaryCountry, p.DataLocation, p.TransferMechanism, p.DPAStatus,
		p.Notes, p.IsActive,
	).Scan(&processorID)
	if err != nil {
		return "", err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO processor_revisions (
		  processor_id, display_name, relationship_type, service_area, website_url,
		  primary_country, data_location, transfer_mechanism, dpa_status, notes,
		  is_active, changed_by, change_reason
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		processorID, p.DisplayName, p.RelationshipType, p.ServiceArea, p.WebsiteURL,
		p.PrimaryCountry, p.DataLocation, p.TransferMechanism, p.DPAStatus, p.Notes,
		p.IsActive, p.ChangedBy, p.ChangeReason,
	); err != nil {
		return "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return processorID, nil
}

const getProcessorByID = `
SELECT id, key, display_name, relationship_type, service_area, website_url,
       primary_country, data_location, transfer_mechanism, dpa_status, notes,
       is_active, created_at, updated_at
FROM processors WHERE id = $1
`

func (q *Queries) GetProcessorByID(ctx context.Context, id string) (Processor, error) {
	var p Processor
	err := q.db.QueryRow(ctx, getProcessorByID, id).Scan(
		&p.ID, &p.Key, &p.DisplayName, &p.RelationshipType, &p.ServiceArea, &p.WebsiteUrl,
		&p.PrimaryCountry, &p.DataLocation, &p.TransferMechanism, &p.DpaStatus, &p.Notes,
		&p.IsActive, &p.CreatedAt, &p.UpdatedAt,
	)
	return p, err
}

const listProcessorRevisions = `
SELECT pr.id, pr.processor_id, p.key, pr.display_name, pr.relationship_type,
       pr.service_area, pr.website_url, pr.primary_country, pr.data_location,
       pr.transfer_mechanism, pr.dpa_status, pr.notes, pr.is_active,
       pr.changed_by, pr.change_reason, pr.created_at
FROM processor_revisions pr
JOIN processors p ON p.id = pr.processor_id
WHERE pr.processor_id = $1
ORDER BY pr.created_at DESC
`

func (q *Queries) ListProcessorRevisions(ctx context.Context, processorID string) ([]ProcessorRevisionView, error) {
	return scanRevisionViews(q.db.Query(ctx, listProcessorRevisions, processorID))
}

type ProcessorUsageRow struct {
	DocumentVersionID string
	DocumentKey       string
	DocumentName      string
	Version           string
	Locale            string
	Audience          string
	IsPublished       bool
	AttachedAt        pgtype.Timestamptz
	RevisionAt        pgtype.Timestamptz
}

const listDocumentVersionsForProcessor = `
SELECT dv.id, d.key, d.display_name, dv.version, dv.locale, dv.audience,
       dv.is_published, dvp.attached_at, pr.created_at
FROM document_version_processors dvp
JOIN processor_revisions pr ON pr.id = dvp.processor_revision_id
JOIN document_versions dv ON dv.id = dvp.document_version_id
JOIN documents d ON d.id = dv.document_id
WHERE pr.processor_id = $1
ORDER BY dvp.attached_at DESC
`

func (q *Queries) ListDocumentVersionsForProcessor(ctx context.Context, processorID string) ([]ProcessorUsageRow, error) {
	rows, err := q.db.Query(ctx, listDocumentVersionsForProcessor, processorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProcessorUsageRow
	for rows.Next() {
		var r ProcessorUsageRow
		if err := rows.Scan(
			&r.DocumentVersionID, &r.DocumentKey, &r.DocumentName,
			&r.Version, &r.Locale, &r.Audience, &r.IsPublished,
			&r.AttachedAt, &r.RevisionAt,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func scanRevisionViews(rows pgx.Rows, scanErr error) ([]ProcessorRevisionView, error) {
	if scanErr != nil {
		return nil, scanErr
	}
	defer rows.Close()
	var out []ProcessorRevisionView
	for rows.Next() {
		var v ProcessorRevisionView
		if err := rows.Scan(
			&v.ID, &v.ProcessorID, &v.ProcessorKey, &v.DisplayName, &v.RelationshipType,
			&v.ServiceArea, &v.WebsiteURL, &v.PrimaryCountry, &v.DataLocation,
			&v.TransferMechanism, &v.DPAStatus, &v.Notes, &v.IsActive,
			&v.ChangedBy, &v.ChangeReason, &v.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
