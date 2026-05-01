package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pillion/regula/internal/auth"
	"github.com/pillion/regula/internal/cache"
	"github.com/pillion/regula/internal/config"
	"github.com/pillion/regula/internal/dashboard"
	"github.com/pillion/regula/internal/store"
)

type API struct {
	cfg      *config.Config
	log      *slog.Logger
	queries  *store.Queries
	pool     *pgxpool.Pool
	docCache *cache.LatestDocumentCache
}

func NewRouter(cfg *config.Config, log *slog.Logger, queries *store.Queries, pool *pgxpool.Pool) (http.Handler, error) {
	api := &API{
		cfg:      cfg,
		log:      log,
		queries:  queries,
		pool:     pool,
		docCache: cache.NewLatestDocumentCache(cfg.DocumentCacheTTL, cfg.DocumentCacheMaxItems),
	}

	validator, err := auth.NewZitadelIntrospectionValidator(auth.ZitadelIntrospectionConfig{
		Issuer:            cfg.ZitadelIssuer,
		IntrospectionURI:  cfg.ZitadelIntrospectionURI,
		CacheTTL:          cfg.ZitadelIntrospectionTTL,
		AllowedAudiences:  cfg.ZitadelAllowedAudiences,
		AllowedServiceIDs: cfg.ZitadelAllowedServiceIDs,
		APIClientID:       cfg.ZitadelAPIClientID,
		APIClientSecret:   cfg.ZitadelAPIClientSecret,
	})
	if err != nil {
		return nil, err
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", api.health)
	r.Get("/readyz", api.ready)

	r.Group(func(r chi.Router) {
		r.Use(api.publicCacheHeaders)
		r.Use(api.publicCORS)

		r.Get("/public/legal/{keyExt}", api.getPublicLegalDocument)
		r.Get("/public/legal/{key}/versions.json", api.listPublicLegalVersions)
		r.Get("/public/legal/{key}/versions/{versionExt}", api.getPublicLegalDocumentPinned)
		r.Get("/public/subprocessors", api.getPublicSubprocessors)
		r.Get("/public/subprocessors.json", api.getPublicSubprocessors)
		r.Get("/public/subprocessors.html", api.getPublicSubprocessors)
	})

	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(validator, auth.LogFailureWith(log)))
		r.Use(api.auditAccess)

		r.Post("/v1/documents", api.createDocument)
		r.Post("/v1/documents/{key}/versions", api.createDocumentVersion)
		r.Get("/v1/documents/{key}/versions", api.listDocumentVersions)
		r.Get("/v1/documents/{key}/versions/latest", api.getLatestDocumentVersion)

		r.Post("/v1/consent-purposes", api.upsertConsentPurpose)
		r.Post("/v1/acceptances", api.createAcceptance)
		r.Post("/v1/consents", api.createConsent)
		r.Get("/v1/processors", api.listProcessors)
		r.Post("/v1/processors", api.upsertProcessor)
		r.Get("/v1/retention-policies", api.listRetentionPolicies)
		r.Post("/v1/retention-policies", api.upsertRetentionPolicy)
		r.Get("/v1/processing-activities", api.listProcessingActivities)
		r.Post("/v1/processing-activities", api.upsertProcessingActivity)
		r.Get("/v1/dpia-records", api.listDPIARecords)
		r.Post("/v1/dpia-records", api.upsertDPIARecord)

		r.Get("/v1/subjects/{subjectRef}/acceptances", api.listAcceptances)
		r.Get("/v1/subjects/{subjectRef}/consents/history", api.listConsentHistory)
		r.Get("/v1/subjects/{subjectRef}/consents/current", api.listCurrentConsents)
		r.Get("/v1/subjects/{subjectRef}/bundle", api.getSubjectBundle)
	})

	if cfg.Dashboard.Enabled {
		dash, err := dashboard.NewServer(cfg.Dashboard, log, queries, pool, r)
		if err != nil {
			return nil, fmt.Errorf("dashboard init: %w", err)
		}
		r.Route("/admin", func(r chi.Router) {
			r.Use(dashboard.BasicAuth(cfg.Dashboard))
			r.Use(dashboard.SameOriginUnsafeMethods)
			dash.Mount(r)
		})
	}

	return r, nil
}

func (api *API) auditAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()

		next.ServeHTTP(ww, r)

		identity, _ := auth.IdentityFromContext(r.Context())
		subjectRef := chi.URLParam(r, "subjectRef")
		documentKey := normalizeKey(chi.URLParam(r, "key"))

		fields := []any{
			"event", "internal_api_access",
			"method", r.Method,
			"path", r.URL.Path,
			"route_pattern", safeRoutePattern(r),
			"status", ww.Status(),
			"bytes", ww.BytesWritten(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", middleware.GetReqID(r.Context()),
			"remote_ip", r.RemoteAddr,
			"user_agent", r.UserAgent(),
		}

		if identity != nil {
			fields = append(fields,
				"principal", identity.Principal,
				"client_id", identity.ClientID,
				"subject", identity.Subject,
				"authorized_party", identity.AuthorizedParty,
			)
		}
		if subjectRef != "" {
			fields = append(fields, "subject_ref", subjectRef)
		}
		if documentKey != "" {
			fields = append(fields, "document_key", documentKey)
		}

		api.log.Info("regula internal api access", fields...)
	})
}

func safeRoutePattern(r *http.Request) string {
	if routeContext := chi.RouteContext(r.Context()); routeContext != nil {
		return routeContext.RoutePattern()
	}

	return ""
}

type createDocumentRequest struct {
	Key         string `json:"key"`
	DisplayName string `json:"display_name"`
	Category    string `json:"category"`
}

type createDocumentVersionRequest struct {
	Version       string    `json:"version"`
	Locale        string    `json:"locale"`
	Audience      string    `json:"audience"`
	ContentType   string    `json:"content_type"`
	Content       string    `json:"content"`
	IsPublished   bool      `json:"is_published"`
	EffectiveFrom time.Time `json:"effective_from"`
	CreatedBy     string    `json:"created_by"`
}

type upsertConsentPurposeRequest struct {
	Key         string `json:"key"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
}

type createAcceptanceRequest struct {
	SubjectRef      string          `json:"subject_ref"`
	DocumentKey     string          `json:"document_key"`
	DocumentVersion string          `json:"document_version"`
	Locale          string          `json:"locale"`
	Audience        string          `json:"audience"`
	AcceptedAt      time.Time       `json:"accepted_at"`
	SourceService   string          `json:"source_service"`
	SourceApp       string          `json:"source_app"`
	IPAddress       string          `json:"ip_address"`
	UserAgent       string          `json:"user_agent"`
	Metadata        json.RawMessage `json:"metadata"`
}

type createConsentRequest struct {
	SubjectRef      string          `json:"subject_ref"`
	PurposeKey      string          `json:"purpose_key"`
	Status          string          `json:"status"`
	LegalBasis      string          `json:"legal_basis"`
	DocumentKey     string          `json:"document_key"`
	DocumentVersion string          `json:"document_version"`
	Locale          string          `json:"locale"`
	Audience        string          `json:"audience"`
	ChangedAt       time.Time       `json:"changed_at"`
	SourceService   string          `json:"source_service"`
	SourceApp       string          `json:"source_app"`
	Metadata        json.RawMessage `json:"metadata"`
}

type upsertProcessorRequest struct {
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

type upsertRetentionPolicyRequest struct {
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

type upsertProcessingActivityRequest struct {
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

type upsertDPIARecordRequest struct {
	Key                string     `json:"key"`
	DisplayName        string     `json:"display_name"`
	Status             string     `json:"status"`
	Summary            string     `json:"summary"`
	Scope              string     `json:"scope"`
	RiskLevel          string     `json:"risk_level"`
	MitigatingMeasures string     `json:"mitigating_measures"`
	Owner              string     `json:"owner"`
	ReviewDueAt        *time.Time `json:"review_due_at"`
	IsActive           *bool      `json:"is_active"`
}

func (api *API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": api.cfg.ServiceName})
}

func (api *API) ready(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
}

func (api *API) createDocument(w http.ResponseWriter, r *http.Request) {
	var req createDocumentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	req.Key = normalizeKey(req.Key)
	if req.Key == "" || strings.TrimSpace(req.DisplayName) == "" {
		writeError(w, http.StatusBadRequest, errors.New("key and display_name are required"))
		return
	}
	if strings.TrimSpace(req.Category) == "" {
		req.Category = "legal"
	}

	doc, err := api.queries.CreateDocument(r.Context(), store.CreateDocumentParams{
		Key:         req.Key,
		DisplayName: strings.TrimSpace(req.DisplayName),
		Category:    strings.TrimSpace(req.Category),
	})
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}

	writeJSON(w, http.StatusCreated, doc)
}

func (api *API) createDocumentVersion(w http.ResponseWriter, r *http.Request) {
	key := normalizeKey(chi.URLParam(r, "key"))
	if key == "" {
		writeError(w, http.StatusBadRequest, errors.New("document key is required"))
		return
	}

	var req createDocumentVersionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	doc, err := api.queries.GetDocumentByKey(r.Context(), key)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	if req.Version == "" || req.Locale == "" || req.Content == "" {
		writeError(w, http.StatusBadRequest, errors.New("version, locale, and content are required"))
		return
	}
	if req.Audience == "" {
		req.Audience = "all"
	}

	contentType, err := normalizeContentType(req.ContentType)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if req.CreatedBy == "" {
		req.CreatedBy = api.cfg.ServiceName
	}
	if req.EffectiveFrom.IsZero() {
		req.EffectiveFrom = time.Now().UTC()
	}

	version, err := api.queries.CreateDocumentVersion(r.Context(), store.CreateDocumentVersionParams{
		DocumentID:    doc.ID,
		Version:       strings.TrimSpace(req.Version),
		Locale:        strings.ToLower(strings.TrimSpace(req.Locale)),
		Audience:      strings.ToLower(strings.TrimSpace(req.Audience)),
		ContentType:   contentType,
		ContentText:   req.Content,
		ContentSha256: sha256Hex(req.Content),
		IsPublished:   req.IsPublished,
		EffectiveFrom: asTimestamptz(req.EffectiveFrom),
		CreatedBy:     strings.TrimSpace(req.CreatedBy),
	})
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}

	api.invalidateDocumentCache(key)
	writeJSON(w, http.StatusCreated, version)
}

func (api *API) listDocumentVersions(w http.ResponseWriter, r *http.Request) {
	key := normalizeKey(chi.URLParam(r, "key"))
	if key == "" {
		writeError(w, http.StatusBadRequest, errors.New("document key is required"))
		return
	}

	rows, err := api.queries.ListDocumentVersionsByKey(r.Context(), key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if rows == nil {
		rows = []store.ListDocumentVersionsByKeyRow{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (api *API) getLatestDocumentVersion(w http.ResponseWriter, r *http.Request) {
	key := normalizeKey(chi.URLParam(r, "key"))
	locale := strings.ToLower(defaultString(r.URL.Query().Get("locale"), "en"))
	audience := strings.ToLower(defaultString(r.URL.Query().Get("audience"), "all"))
	cacheKey := buildLatestVersionCacheKey(key, locale, audience)

	if cached, ok := api.docCache.Get(cacheKey); ok {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	version, err := api.queries.GetLatestPublishedDocumentVersion(r.Context(), store.GetLatestPublishedDocumentVersionParams{
		Key:      key,
		Locale:   locale,
		Audience: audience,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	api.docCache.Set(cacheKey, version)
	writeJSON(w, http.StatusOK, version)
}

func (api *API) upsertConsentPurpose(w http.ResponseWriter, r *http.Request) {
	var req upsertConsentPurposeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	req.Key = normalizeKey(req.Key)
	if req.Key == "" || strings.TrimSpace(req.DisplayName) == "" {
		writeError(w, http.StatusBadRequest, errors.New("key and display_name are required"))
		return
	}

	purpose, err := api.queries.UpsertConsentPurpose(r.Context(), store.UpsertConsentPurposeParams{
		Key:         req.Key,
		DisplayName: strings.TrimSpace(req.DisplayName),
		Description: strings.TrimSpace(req.Description),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, purpose)
}

func (api *API) createAcceptance(w http.ResponseWriter, r *http.Request) {
	var req createAcceptanceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	version, err := api.lookupDocumentVersion(r, req.DocumentKey, req.DocumentVersion, req.Locale, req.Audience)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	if req.SubjectRef == "" || req.SourceService == "" {
		writeError(w, http.StatusBadRequest, errors.New("subject_ref and source_service are required"))
		return
	}
	if req.AcceptedAt.IsZero() {
		req.AcceptedAt = time.Now().UTC()
	}
	metadata := ensureJSON(req.Metadata)

	event, err := api.queries.CreateAcceptanceEvent(r.Context(), store.CreateAcceptanceEventParams{
		SubjectRef:        strings.TrimSpace(req.SubjectRef),
		DocumentVersionID: version.ID,
		AcceptedAt:        asTimestamptz(req.AcceptedAt),
		SourceService:     strings.TrimSpace(req.SourceService),
		SourceApp:         strings.TrimSpace(req.SourceApp),
		IpAddress:         strings.TrimSpace(req.IPAddress),
		UserAgent:         strings.TrimSpace(req.UserAgent),
		EvidenceSha256:    sha256Hex(fmt.Sprintf("%s|%s|%s|%s|%s", req.SubjectRef, req.DocumentKey, req.DocumentVersion, req.SourceService, req.AcceptedAt.UTC().Format(time.RFC3339Nano))),
		Metadata:          metadata,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusCreated, event)
}

func (api *API) createConsent(w http.ResponseWriter, r *http.Request) {
	var req createConsentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if req.SubjectRef == "" || req.SourceService == "" || req.PurposeKey == "" {
		writeError(w, http.StatusBadRequest, errors.New("subject_ref, source_service, and purpose_key are required"))
		return
	}
	if req.Status != "granted" && req.Status != "revoked" {
		writeError(w, http.StatusBadRequest, errors.New("status must be granted or revoked"))
		return
	}
	if req.LegalBasis == "" {
		req.LegalBasis = "consent"
	}
	if req.ChangedAt.IsZero() {
		req.ChangedAt = time.Now().UTC()
	}

	purpose, err := api.queries.GetConsentPurposeByKey(r.Context(), normalizeKey(req.PurposeKey))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, fmt.Errorf("unknown consent purpose %q — register it via POST /v1/consent-purposes first", req.PurposeKey))
		return
	}

	version, err := api.lookupDocumentVersion(r, req.DocumentKey, req.DocumentVersion, req.Locale, req.Audience)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	event, err := api.queries.CreateConsentEvent(r.Context(), store.CreateConsentEventParams{
		SubjectRef:        strings.TrimSpace(req.SubjectRef),
		ConsentPurposeID:  purpose.ID,
		DocumentVersionID: version.ID,
		Status:            req.Status,
		LegalBasis:        strings.TrimSpace(req.LegalBasis),
		ChangedAt:         asTimestamptz(req.ChangedAt),
		SourceService:     strings.TrimSpace(req.SourceService),
		SourceApp:         strings.TrimSpace(req.SourceApp),
		EvidenceSha256:    sha256Hex(fmt.Sprintf("%s|%s|%s|%s|%s", req.SubjectRef, req.PurposeKey, req.Status, req.SourceService, req.ChangedAt.UTC().Format(time.RFC3339Nano))),
		Metadata:          ensureJSON(req.Metadata),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusCreated, event)
}

func (api *API) listAcceptances(w http.ResponseWriter, r *http.Request) {
	subjectRef := chi.URLParam(r, "subjectRef")
	rows, err := api.queries.ListAcceptanceEventsBySubject(r.Context(), subjectRef)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (api *API) listConsentHistory(w http.ResponseWriter, r *http.Request) {
	subjectRef := chi.URLParam(r, "subjectRef")
	rows, err := api.queries.ListConsentEventsBySubject(r.Context(), subjectRef)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (api *API) listCurrentConsents(w http.ResponseWriter, r *http.Request) {
	subjectRef := chi.URLParam(r, "subjectRef")
	rows, err := api.queries.GetCurrentConsentsBySubject(r.Context(), subjectRef)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (api *API) getSubjectBundle(w http.ResponseWriter, r *http.Request) {
	subjectRef := chi.URLParam(r, "subjectRef")
	acceptances, err := api.queries.ListAcceptanceEventsBySubject(r.Context(), subjectRef)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	consentHistory, err := api.queries.ListConsentEventsBySubject(r.Context(), subjectRef)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	currentConsents, err := api.queries.GetCurrentConsentsBySubject(r.Context(), subjectRef)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"subject_ref":      subjectRef,
		"acceptances":      acceptances,
		"consent_history":  consentHistory,
		"current_consents": currentConsents,
	})
}

func (api *API) listProcessors(w http.ResponseWriter, r *http.Request) {
	rows, err := api.queries.ListProcessors(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (api *API) upsertProcessor(w http.ResponseWriter, r *http.Request) {
	var req upsertProcessorRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	req.Key = normalizeKey(req.Key)
	if req.Key == "" || strings.TrimSpace(req.DisplayName) == "" {
		writeError(w, http.StatusBadRequest, errors.New("key and display_name are required"))
		return
	}

	row, err := api.queries.UpsertProcessor(r.Context(), store.UpsertProcessorParams{
		Key:               req.Key,
		DisplayName:       strings.TrimSpace(req.DisplayName),
		RelationshipType:  normalizeRelationshipType(req.RelationshipType),
		ServiceArea:       strings.TrimSpace(req.ServiceArea),
		WebsiteUrl:        strings.TrimSpace(req.WebsiteURL),
		PrimaryCountry:    strings.TrimSpace(req.PrimaryCountry),
		DataLocation:      strings.TrimSpace(req.DataLocation),
		TransferMechanism: strings.TrimSpace(req.TransferMechanism),
		DpaStatus:         normalizeDPAStatus(req.DPAStatus),
		Notes:             strings.TrimSpace(req.Notes),
		IsActive:          defaultBool(req.IsActive, true),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, row)
}

func (api *API) listRetentionPolicies(w http.ResponseWriter, r *http.Request) {
	rows, err := api.queries.ListRetentionPolicies(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (api *API) upsertRetentionPolicy(w http.ResponseWriter, r *http.Request) {
	var req upsertRetentionPolicyRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	req.Key = normalizeKey(req.Key)
	if req.Key == "" || strings.TrimSpace(req.DisplayName) == "" {
		writeError(w, http.StatusBadRequest, errors.New("key and display_name are required"))
		return
	}

	retentionDays := pgtype.Int4{}
	if req.RetentionDays != nil {
		if *req.RetentionDays < 0 {
			writeError(w, http.StatusBadRequest, errors.New("retention_days must be greater than or equal to 0"))
			return
		}

		retentionDays = pgtype.Int4{Int32: *req.RetentionDays, Valid: true}
	}

	row, err := api.queries.UpsertRetentionPolicy(r.Context(), store.UpsertRetentionPolicyParams{
		Key:            req.Key,
		DisplayName:    strings.TrimSpace(req.DisplayName),
		DataCategory:   strings.TrimSpace(defaultString(req.DataCategory, "general")),
		Description:    strings.TrimSpace(req.Description),
		RetentionDays:  retentionDays,
		TriggerEvent:   strings.TrimSpace(req.TriggerEvent),
		StorageScope:   strings.TrimSpace(req.StorageScope),
		DeletionMethod: strings.TrimSpace(req.DeletionMethod),
		LegalBasis:     strings.TrimSpace(req.LegalBasis),
		Notes:          strings.TrimSpace(req.Notes),
		IsActive:       defaultBool(req.IsActive, true),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, row)
}

func (api *API) listProcessingActivities(w http.ResponseWriter, r *http.Request) {
	rows, err := api.queries.ListProcessingActivities(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (api *API) upsertProcessingActivity(w http.ResponseWriter, r *http.Request) {
	var req upsertProcessingActivityRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	req.Key = normalizeKey(req.Key)
	if req.Key == "" || strings.TrimSpace(req.DisplayName) == "" {
		writeError(w, http.StatusBadRequest, errors.New("key and display_name are required"))
		return
	}

	row, err := api.queries.UpsertProcessingActivity(r.Context(), store.UpsertProcessingActivityParams{
		Key:                    req.Key,
		DisplayName:            strings.TrimSpace(req.DisplayName),
		Purpose:                strings.TrimSpace(req.Purpose),
		LegalBasis:             strings.TrimSpace(req.LegalBasis),
		DataSubjectCategories:  strings.TrimSpace(req.DataSubjectCategories),
		PersonalDataCategories: strings.TrimSpace(req.PersonalDataCategories),
		RecipientCategories:    strings.TrimSpace(req.RecipientCategories),
		TransferNotes:          strings.TrimSpace(req.TransferNotes),
		RetentionSummary:       strings.TrimSpace(req.RetentionSummary),
		SecurityMeasures:       strings.TrimSpace(req.SecurityMeasures),
		Owner:                  strings.TrimSpace(req.Owner),
		IsActive:               defaultBool(req.IsActive, true),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, row)
}

func (api *API) listDPIARecords(w http.ResponseWriter, r *http.Request) {
	rows, err := api.queries.ListDPIARecords(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (api *API) upsertDPIARecord(w http.ResponseWriter, r *http.Request) {
	var req upsertDPIARecordRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	req.Key = normalizeKey(req.Key)
	if req.Key == "" || strings.TrimSpace(req.DisplayName) == "" {
		writeError(w, http.StatusBadRequest, errors.New("key and display_name are required"))
		return
	}

	reviewDueAt := pgtype.Timestamptz{}
	if req.ReviewDueAt != nil {
		reviewDueAt = pgtype.Timestamptz{Time: req.ReviewDueAt.UTC(), Valid: true}
	}

	row, err := api.queries.UpsertDPIARecord(r.Context(), store.UpsertDPIARecordParams{
		Key:                req.Key,
		DisplayName:        strings.TrimSpace(req.DisplayName),
		Status:             normalizeDPIAStatus(req.Status),
		Summary:            strings.TrimSpace(req.Summary),
		Scope:              strings.TrimSpace(req.Scope),
		RiskLevel:          normalizeRiskLevel(req.RiskLevel),
		MitigatingMeasures: strings.TrimSpace(req.MitigatingMeasures),
		Owner:              strings.TrimSpace(req.Owner),
		ReviewDueAt:        reviewDueAt,
		IsActive:           defaultBool(req.IsActive, true),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, row)
}

func (api *API) lookupDocumentVersion(r *http.Request, documentKey, documentVersion, locale, audience string) (store.DocumentVersion, error) {
	if normalizeKey(documentKey) == "" || strings.TrimSpace(documentVersion) == "" || strings.TrimSpace(locale) == "" {
		return store.DocumentVersion{}, errors.New("document_key, document_version, and locale are required")
	}

	if strings.TrimSpace(audience) == "" {
		audience = "all"
	}

	return api.queries.GetDocumentVersionByNaturalKey(r.Context(), store.GetDocumentVersionByNaturalKeyParams{
		Key:      normalizeKey(documentKey),
		Version:  strings.TrimSpace(documentVersion),
		Locale:   strings.ToLower(strings.TrimSpace(locale)),
		Audience: strings.ToLower(strings.TrimSpace(audience)),
	})
}

func (api *API) invalidateDocumentCache(documentKey string) {
	api.docCache.InvalidatePrefix(normalizeKey(documentKey) + "|")
}

func decodeJSON(r *http.Request, dst any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if decoder.More() {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

func ensureJSON(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return []byte(`{}`)
	}
	return raw
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{
		"error":   http.StatusText(status),
		"message": err.Error(),
	})
}

func normalizeKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
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
	switch normalizeKey(value) {
	case "subprocessor":
		return "subprocessor"
	default:
		return "processor"
	}
}

func normalizeDPAStatus(value string) string {
	switch normalizeKey(value) {
	case "pending", "signed", "not_required":
		return normalizeKey(value)
	default:
		return "unknown"
	}
}

func normalizeDPIAStatus(value string) string {
	switch normalizeKey(value) {
	case "in_review", "approved", "retired":
		return normalizeKey(value)
	default:
		return "draft"
	}
}

func normalizeRiskLevel(value string) string {
	switch normalizeKey(value) {
	case "low", "high", "very_high":
		return normalizeKey(value)
	default:
		return "medium"
	}
}

func normalizeContentType(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "html", nil
	}

	switch normalized {
	case "html", "text/html":
		return "html", nil
	case "markdown", "md", "text/markdown":
		return "markdown", nil
	default:
		return "", fmt.Errorf("unsupported content_type %q: use html or markdown", value)
	}
}

func buildLatestVersionCacheKey(documentKey, locale, audience string) string {
	return normalizeKey(documentKey) + "|" + strings.ToLower(strings.TrimSpace(locale)) + "|" + strings.ToLower(strings.TrimSpace(audience))
}

func asTimestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func sha256Hex(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
