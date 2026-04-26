package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// Hand-written queries for the admin dashboard and the visibility-aware
// public surface. Kept separate from sqlc-generated files on purpose:
// regenerating sqlc must never clobber these.

type DocumentListItem struct {
	ID                string             `json:"id"`
	Key               string             `json:"key"`
	DisplayName       string             `json:"display_name"`
	Category          string             `json:"category"`
	IsPubliclyVisible bool               `json:"is_publicly_visible"`
	CreatedAt         pgtype.Timestamptz `json:"created_at"`
	VersionCount      int64              `json:"version_count"`
	PublishedCount    int64              `json:"published_count"`
}

const listDocumentsWithVisibility = `
SELECT
  d.id,
  d.key,
  d.display_name,
  d.category,
  d.is_publicly_visible,
  d.created_at,
  COALESCE(v.total, 0) AS version_count,
  COALESCE(v.published, 0) AS published_count
FROM documents d
LEFT JOIN (
  SELECT document_id,
         COUNT(*) AS total,
         COUNT(*) FILTER (WHERE is_published) AS published
  FROM document_versions
  GROUP BY document_id
) v ON v.document_id = d.id
ORDER BY d.key
`

func (q *Queries) ListDocumentsWithVisibility(ctx context.Context) ([]DocumentListItem, error) {
	rows, err := q.db.Query(ctx, listDocumentsWithVisibility)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []DocumentListItem
	for rows.Next() {
		var i DocumentListItem
		if err := rows.Scan(
			&i.ID,
			&i.Key,
			&i.DisplayName,
			&i.Category,
			&i.IsPubliclyVisible,
			&i.CreatedAt,
			&i.VersionCount,
			&i.PublishedCount,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

const isDocumentPubliclyVisible = `
SELECT is_publicly_visible FROM documents WHERE key = $1
`

func (q *Queries) IsDocumentPubliclyVisible(ctx context.Context, key string) (bool, error) {
	var visible bool
	err := q.db.QueryRow(ctx, isDocumentPubliclyVisible, key).Scan(&visible)
	return visible, err
}

const setDocumentVisibility = `
UPDATE documents SET is_publicly_visible = $2 WHERE key = $1
RETURNING id
`

func (q *Queries) SetDocumentVisibility(ctx context.Context, key string, visible bool) error {
	var id string
	return q.db.QueryRow(ctx, setDocumentVisibility, key, visible).Scan(&id)
}

const listPublicDocumentKeys = `
SELECT key FROM documents WHERE is_publicly_visible = TRUE ORDER BY key
`

func (q *Queries) ListPublicDocumentKeys(ctx context.Context) ([]string, error) {
	rows, err := q.db.Query(ctx, listPublicDocumentKeys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}
