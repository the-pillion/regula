package seed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pillion/regula/internal/store"
)

type Manifest struct {
	Documents       []DocumentSeed       `json:"documents"`
	ConsentPurposes []ConsentPurposeSeed `json:"consent_purposes"`
}

type DocumentSeed struct {
	Key           string `json:"key"`
	DisplayName   string `json:"display_name"`
	Category      string `json:"category"`
	Version       string `json:"version"`
	Locale        string `json:"locale"`
	Audience      string `json:"audience"`
	ContentType   string `json:"content_type"`
	ContentPath   string `json:"content_path"`
	IsPublished   bool   `json:"is_published"`
	EffectiveFrom string `json:"effective_from"`
	CreatedBy     string `json:"created_by"`
}

type ConsentPurposeSeed struct {
	Key         string `json:"key"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
}

func Apply(ctx context.Context, queries *store.Queries, manifestPath string) error {
	manifest, baseDir, err := LoadManifest(manifestPath)
	if err != nil {
		return err
	}

	for _, purpose := range manifest.ConsentPurposes {
		if _, err := queries.UpsertConsentPurpose(ctx, store.UpsertConsentPurposeParams{
			Key:         normalizeKey(purpose.Key),
			DisplayName: strings.TrimSpace(purpose.DisplayName),
			Description: strings.TrimSpace(purpose.Description),
		}); err != nil {
			return fmt.Errorf("upsert consent purpose %s: %w", purpose.Key, err)
		}
	}

	for _, document := range manifest.Documents {
		contentPath := filepath.Join(baseDir, document.ContentPath)
		contentBytes, err := os.ReadFile(contentPath)
		if err != nil {
			return fmt.Errorf("read seed content %s: %w", contentPath, err)
		}

		doc, err := queries.UpsertDocument(ctx, store.UpsertDocumentParams{
			Key:         normalizeKey(document.Key),
			DisplayName: strings.TrimSpace(document.DisplayName),
			Category:    defaultString(strings.TrimSpace(document.Category), "legal"),
		})
		if err != nil {
			return fmt.Errorf("upsert document %s: %w", document.Key, err)
		}

		effectiveFrom, err := parseTime(document.EffectiveFrom)
		if err != nil {
			return fmt.Errorf("parse effective_from for %s: %w", document.Key, err)
		}

		if _, err := queries.UpsertDocumentVersion(ctx, store.UpsertDocumentVersionParams{
			DocumentID:    doc.ID,
			Version:       strings.TrimSpace(document.Version),
			Locale:        strings.ToLower(strings.TrimSpace(document.Locale)),
			Audience:      strings.ToLower(defaultString(strings.TrimSpace(document.Audience), "all")),
			ContentType:   normalizeContentType(document.ContentType),
			ContentText:   string(contentBytes),
			ContentSha256: sha256Hex(contentBytes),
			IsPublished:   document.IsPublished,
			EffectiveFrom: pgtype.Timestamptz{Time: effectiveFrom.UTC(), Valid: true},
			CreatedBy:     defaultString(strings.TrimSpace(document.CreatedBy), "seed"),
		}); err != nil {
			return fmt.Errorf("upsert document version %s@%s: %w", document.Key, document.Version, err)
		}
	}

	return nil
}

func LoadManifest(path string) (*Manifest, string, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}

	var manifest Manifest
	if err := json.Unmarshal(bytes, &manifest); err != nil {
		return nil, "", err
	}

	return &manifest, filepath.Dir(path), nil
}

func parseTime(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Now().UTC(), nil
	}
	return time.Parse(time.RFC3339, value)
}

func normalizeContentType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "markdown", "md", "text/markdown":
		return "markdown"
	default:
		return "html"
	}
}

func normalizeKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func sha256Hex(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}
