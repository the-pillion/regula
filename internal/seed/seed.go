package seed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pillion/regula/internal/store"
)

var placeholderPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_]+)\s*\}\}`)

const companyBaseFile = "company.base.json"

type Manifest struct {
	Documents            []DocumentSeed           `json:"documents"`
	ConsentPurposes      []ConsentPurposeSeed     `json:"consent_purposes"`
	Processors           []ProcessorSeed          `json:"processors"`
	RetentionPolicies    []RetentionPolicySeed    `json:"retention_policies"`
	ProcessingActivities []ProcessingActivitySeed `json:"processing_activities"`
	DPIARecords          []DPIARecordSeed         `json:"dpia_records"`
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

type ProcessorSeed struct {
	Key               string `json:"key"`
	DisplayName       string `json:"display_name"`
	RelationshipType  string `json:"relationship_type"`
	ServiceArea       string `json:"service_area"`
	WebsiteURL        string `json:"website_url"`
	PrimaryCountry    string `json:"primary_country"`
	DataLocation      string `json:"data_location"`
	TransferMechanism string `json:"transfer_mechanism"`
	DPAStatus         string `json:"dpa_status"`
	Notes             string `json:"notes"`
	IsActive          *bool  `json:"is_active"`
}

type RetentionPolicySeed struct {
	Key            string `json:"key"`
	DisplayName    string `json:"display_name"`
	DataCategory   string `json:"data_category"`
	Description    string `json:"description"`
	RetentionDays  *int32 `json:"retention_days"`
	TriggerEvent   string `json:"trigger_event"`
	StorageScope   string `json:"storage_scope"`
	DeletionMethod string `json:"deletion_method"`
	LegalBasis     string `json:"legal_basis"`
	Notes          string `json:"notes"`
	IsActive       *bool  `json:"is_active"`
}

type ProcessingActivitySeed struct {
	Key                    string `json:"key"`
	DisplayName            string `json:"display_name"`
	Purpose                string `json:"purpose"`
	LegalBasis             string `json:"legal_basis"`
	DataSubjectCategories  string `json:"data_subject_categories"`
	PersonalDataCategories string `json:"personal_data_categories"`
	RecipientCategories    string `json:"recipient_categories"`
	TransferNotes          string `json:"transfer_notes"`
	RetentionSummary       string `json:"retention_summary"`
	SecurityMeasures       string `json:"security_measures"`
	Owner                  string `json:"owner"`
	IsActive               *bool  `json:"is_active"`
}

type DPIARecordSeed struct {
	Key                string `json:"key"`
	DisplayName        string `json:"display_name"`
	Status             string `json:"status"`
	Summary            string `json:"summary"`
	Scope              string `json:"scope"`
	RiskLevel          string `json:"risk_level"`
	MitigatingMeasures string `json:"mitigating_measures"`
	Owner              string `json:"owner"`
	ReviewDueAt        string `json:"review_due_at"`
	IsActive           *bool  `json:"is_active"`
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

	for _, processor := range manifest.Processors {
		if _, err := queries.UpsertProcessor(ctx, store.UpsertProcessorParams{
			Key:               normalizeKey(processor.Key),
			DisplayName:       strings.TrimSpace(processor.DisplayName),
			RelationshipType:  normalizeRelationshipType(processor.RelationshipType),
			ServiceArea:       strings.TrimSpace(processor.ServiceArea),
			WebsiteUrl:        strings.TrimSpace(processor.WebsiteURL),
			PrimaryCountry:    strings.TrimSpace(processor.PrimaryCountry),
			DataLocation:      strings.TrimSpace(processor.DataLocation),
			TransferMechanism: strings.TrimSpace(processor.TransferMechanism),
			DpaStatus:         normalizeDPAStatus(processor.DPAStatus),
			Notes:             strings.TrimSpace(processor.Notes),
			IsActive:          defaultBool(processor.IsActive, true),
		}); err != nil {
			return fmt.Errorf("upsert processor %s: %w", processor.Key, err)
		}
	}

	for _, policy := range manifest.RetentionPolicies {
		retentionDays := pgtype.Int4{}
		if policy.RetentionDays != nil {
			retentionDays = pgtype.Int4{Int32: *policy.RetentionDays, Valid: true}
		}

		if _, err := queries.UpsertRetentionPolicy(ctx, store.UpsertRetentionPolicyParams{
			Key:            normalizeKey(policy.Key),
			DisplayName:    strings.TrimSpace(policy.DisplayName),
			DataCategory:   strings.TrimSpace(defaultString(policy.DataCategory, "general")),
			Description:    strings.TrimSpace(policy.Description),
			RetentionDays:  retentionDays,
			TriggerEvent:   strings.TrimSpace(policy.TriggerEvent),
			StorageScope:   strings.TrimSpace(policy.StorageScope),
			DeletionMethod: strings.TrimSpace(policy.DeletionMethod),
			LegalBasis:     strings.TrimSpace(policy.LegalBasis),
			Notes:          strings.TrimSpace(policy.Notes),
			IsActive:       defaultBool(policy.IsActive, true),
		}); err != nil {
			return fmt.Errorf("upsert retention policy %s: %w", policy.Key, err)
		}
	}

	for _, activity := range manifest.ProcessingActivities {
		if _, err := queries.UpsertProcessingActivity(ctx, store.UpsertProcessingActivityParams{
			Key:                    normalizeKey(activity.Key),
			DisplayName:            strings.TrimSpace(activity.DisplayName),
			Purpose:                strings.TrimSpace(activity.Purpose),
			LegalBasis:             strings.TrimSpace(activity.LegalBasis),
			DataSubjectCategories:  strings.TrimSpace(activity.DataSubjectCategories),
			PersonalDataCategories: strings.TrimSpace(activity.PersonalDataCategories),
			RecipientCategories:    strings.TrimSpace(activity.RecipientCategories),
			TransferNotes:          strings.TrimSpace(activity.TransferNotes),
			RetentionSummary:       strings.TrimSpace(activity.RetentionSummary),
			SecurityMeasures:       strings.TrimSpace(activity.SecurityMeasures),
			Owner:                  strings.TrimSpace(activity.Owner),
			IsActive:               defaultBool(activity.IsActive, true),
		}); err != nil {
			return fmt.Errorf("upsert processing activity %s: %w", activity.Key, err)
		}
	}

	for _, record := range manifest.DPIARecords {
		reviewDueAt := pgtype.Timestamptz{}
		if strings.TrimSpace(record.ReviewDueAt) != "" {
			parsed, err := parseTime(record.ReviewDueAt)
			if err != nil {
				return fmt.Errorf("parse review_due_at for %s: %w", record.Key, err)
			}
			reviewDueAt = pgtype.Timestamptz{Time: parsed.UTC(), Valid: true}
		}

		if _, err := queries.UpsertDPIARecord(ctx, store.UpsertDPIARecordParams{
			Key:                normalizeKey(record.Key),
			DisplayName:        strings.TrimSpace(record.DisplayName),
			Status:             normalizeDPIAStatus(record.Status),
			Summary:            strings.TrimSpace(record.Summary),
			Scope:              strings.TrimSpace(record.Scope),
			RiskLevel:          normalizeRiskLevel(record.RiskLevel),
			MitigatingMeasures: strings.TrimSpace(record.MitigatingMeasures),
			Owner:              strings.TrimSpace(record.Owner),
			ReviewDueAt:        reviewDueAt,
			IsActive:           defaultBool(record.IsActive, true),
		}); err != nil {
			return fmt.Errorf("upsert dpia record %s: %w", record.Key, err)
		}
	}

	companyVarsByLocale := map[string]map[string]string{}

	for _, document := range manifest.Documents {
		contentPath := filepath.Join(baseDir, document.ContentPath)
		contentBytes, err := os.ReadFile(contentPath)
		if err != nil {
			return fmt.Errorf("read seed content %s: %w", contentPath, err)
		}

		locale := strings.ToLower(strings.TrimSpace(document.Locale))
		vars, ok := companyVarsByLocale[locale]
		if !ok {
			vars, err = loadCompanyVars(baseDir, locale)
			if err != nil {
				return fmt.Errorf("load company vars for %s: %w", document.Key, err)
			}
			companyVarsByLocale[locale] = vars
		}

		rendered, err := applyPlaceholders(string(contentBytes), vars)
		if err != nil {
			return fmt.Errorf("render %s: %w", document.ContentPath, err)
		}
		contentBytes = []byte(rendered)

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

// loadCompanyVars merges the language-neutral company.base.json with the
// language overlay company.<lang>.json (the latter wins on conflict). The
// language is the primary subtag of the document locale (it-IT -> it).
func loadCompanyVars(baseDir, locale string) (map[string]string, error) {
	vars := map[string]string{}

	if err := mergeCompanyFile(filepath.Join(baseDir, companyBaseFile), vars, true); err != nil {
		return nil, err
	}

	lang := strings.SplitN(strings.ToLower(strings.TrimSpace(locale)), "-", 2)[0]
	if lang != "" {
		overlay := filepath.Join(baseDir, fmt.Sprintf("company.%s.json", lang))
		if err := mergeCompanyFile(overlay, vars, false); err != nil {
			return nil, err
		}
	}

	return vars, nil
}

func mergeCompanyFile(path string, into map[string]string, required bool) error {
	bytes, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && !required {
			return nil
		}
		return fmt.Errorf("read company file %s: %w", path, err)
	}

	values := map[string]string{}
	if err := json.Unmarshal(bytes, &values); err != nil {
		return fmt.Errorf("parse company file %s (all values must be strings): %w", path, err)
	}

	for key, value := range values {
		into[key] = value
	}
	return nil
}

// applyPlaceholders substitutes every {{ key }} token with its company value.
// It fails loud on an unknown key (typo guard) so a malformed legal document
// can never be seeded silently. A declared-but-empty value is allowed: that is
// the fillable-template state before official figures are entered.
func applyPlaceholders(content string, vars map[string]string) (string, error) {
	missing := map[string]struct{}{}

	rendered := placeholderPattern.ReplaceAllStringFunc(content, func(token string) string {
		key := placeholderPattern.FindStringSubmatch(token)[1]
		if value, ok := vars[key]; ok {
			return value
		}
		missing[key] = struct{}{}
		return token
	})

	if len(missing) > 0 {
		keys := make([]string, 0, len(missing))
		for key := range missing {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		return "", fmt.Errorf("unknown placeholders (not defined in company.*.json): %s", strings.Join(keys, ", "))
	}

	return rendered, nil
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

func defaultBool(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func normalizeRelationshipType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "subprocessor":
		return "subprocessor"
	default:
		return "processor"
	}
}

func normalizeDPAStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "pending", "signed", "not_required":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "unknown"
	}
}

func normalizeDPIAStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "in_review", "approved", "retired":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "draft"
	}
}

func normalizeRiskLevel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low", "high", "very_high":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "medium"
	}
}

func sha256Hex(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}
