package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ServiceName              string
	HTTPPort                 int
	DatabaseURL              string
	AutoMigrate              bool
	LogLevel                 string
	DocumentCacheTTL         time.Duration
	DocumentCacheMaxItems    int
	ZitadelIssuer            string
	ZitadelProjectID         string
	ZitadelIntrospectionURI  string
	ZitadelIntrospectionTTL  time.Duration
	ZitadelAllowedAudiences  []string
	ZitadelAllowedServiceIDs []string
	ZitadelAPIClientID       string
	ZitadelAPIClientSecret   string

	PublicCacheMaxAge          time.Duration
	PublicCacheSharedMaxAge    time.Duration
	PublicCacheStaleRevalidate time.Duration
	PublicCORSAllowedOrigins   []string

	Dashboard DashboardConfig
}

// DashboardConfig is the basic-auth-protected admin dashboard config.
// Zitadel OAuth integration is deferred; for now the dashboard uses
// a single shared username + password from env. Keep a single user only.
type DashboardConfig struct {
	Enabled  bool
	Username string
	Password string
	Realm    string
}

func Load() (*Config, error) {
	issuer := strings.TrimRight(strings.TrimSpace(os.Getenv("ZITADEL_ISSUER")), "/")
	introspectionURI := strings.TrimSpace(os.Getenv("ZITADEL_INTROSPECTION_URI"))
	if introspectionURI == "" && issuer != "" {
		introspectionURI = issuer + "/oauth/v2/introspect"
	}

	cfg := &Config{
		ServiceName:              getEnv("REGULA_SERVICE_NAME", "regula"),
		HTTPPort:                 getInt("REGULA_HTTP_PORT", 8085),
		DatabaseURL:              strings.TrimSpace(os.Getenv("REGULA_DATABASE_URL")),
		AutoMigrate:              getBool("REGULA_AUTO_MIGRATE", true),
		LogLevel:                 getEnv("REGULA_LOG_LEVEL", "info"),
		DocumentCacheTTL:         time.Duration(getInt("REGULA_DOCUMENT_CACHE_TTL_SECONDS", 120)) * time.Second,
		DocumentCacheMaxItems:    getInt("REGULA_DOCUMENT_CACHE_MAX_ITEMS", 64),
		ZitadelIssuer:            issuer,
		ZitadelProjectID:         strings.TrimSpace(os.Getenv("ZITADEL_PROJECT_ID")),
		ZitadelIntrospectionURI:  introspectionURI,
		ZitadelIntrospectionTTL:  time.Duration(getInt("ZITADEL_INTROSPECTION_CACHE_TTL_SECONDS", 15)) * time.Second,
		ZitadelAllowedAudiences:  audienceConfig(),
		ZitadelAllowedServiceIDs: splitCSV(os.Getenv("REGULA_ALLOWED_SERVICE_IDS")),
		ZitadelAPIClientID:       strings.TrimSpace(os.Getenv("REGULA_ZITADEL_API_CLIENT_ID")),
		ZitadelAPIClientSecret:   strings.TrimSpace(os.Getenv("REGULA_ZITADEL_API_CLIENT_SECRET")),

		PublicCacheMaxAge:          time.Duration(getInt("REGULA_PUBLIC_CACHE_MAX_AGE_SECONDS", 300)) * time.Second,
		PublicCacheSharedMaxAge:    time.Duration(getInt("REGULA_PUBLIC_CACHE_SHARED_MAX_AGE_SECONDS", 3600)) * time.Second,
		PublicCacheStaleRevalidate: time.Duration(getInt("REGULA_PUBLIC_CACHE_STALE_REVALIDATE_SECONDS", 86400)) * time.Second,
		PublicCORSAllowedOrigins:   splitCSV(os.Getenv("REGULA_PUBLIC_CORS_ALLOWED_ORIGINS")),
		Dashboard:                  loadDashboardConfig(),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("REGULA_DATABASE_URL is required")
	}
	if cfg.DocumentCacheMaxItems < 0 {
		return nil, fmt.Errorf("REGULA_DOCUMENT_CACHE_MAX_ITEMS cannot be negative")
	}
	if cfg.ZitadelIssuer == "" {
		return nil, fmt.Errorf("ZITADEL_ISSUER is required")
	}
	if cfg.ZitadelProjectID == "" {
		return nil, fmt.Errorf("ZITADEL_PROJECT_ID is required")
	}
	if cfg.ZitadelIntrospectionURI == "" {
		return nil, fmt.Errorf("ZITADEL_INTROSPECTION_URI is required when ZITADEL_ISSUER is set")
	}
	if cfg.ZitadelAPIClientID == "" {
		return nil, fmt.Errorf("REGULA_ZITADEL_API_CLIENT_ID is required")
	}
	if cfg.ZitadelAPIClientSecret == "" {
		return nil, fmt.Errorf("REGULA_ZITADEL_API_CLIENT_SECRET is required")
	}
	if len(cfg.ZitadelAllowedAudiences) == 0 {
		return nil, fmt.Errorf("REGULA_ALLOWED_AUDIENCES or ZITADEL_PROJECT_ID must contain at least one value")
	}
	if len(cfg.ZitadelAllowedServiceIDs) == 0 {
		return nil, fmt.Errorf("REGULA_ALLOWED_SERVICE_IDS must contain at least one value")
	}
	if err := cfg.Dashboard.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func loadDashboardConfig() DashboardConfig {
	enabled := getBool("REGULA_DASHBOARD_ENABLED", false)
	return DashboardConfig{
		Enabled:  enabled,
		Username: strings.TrimSpace(os.Getenv("REGULA_DASHBOARD_USER")),
		Password: os.Getenv("REGULA_DASHBOARD_PASSWORD"),
		Realm:    getEnv("REGULA_DASHBOARD_REALM", "Regula Admin"),
	}
}

// Validate returns nil if dashboard is disabled or properly configured.
func (d DashboardConfig) Validate() error {
	if !d.Enabled {
		return nil
	}
	if d.Username == "" {
		return fmt.Errorf("REGULA_DASHBOARD_USER is required when REGULA_DASHBOARD_ENABLED=true")
	}
	if d.Password == "" {
		return fmt.Errorf("REGULA_DASHBOARD_PASSWORD is required when REGULA_DASHBOARD_ENABLED=true")
	}
	return nil
}

func audienceConfig() []string {
	raw := strings.TrimSpace(os.Getenv("REGULA_ALLOWED_AUDIENCES"))
	if raw != "" {
		return splitCSV(raw)
	}

	projectID := strings.TrimSpace(os.Getenv("ZITADEL_PROJECT_ID"))
	if projectID == "" {
		return nil
	}

	return []string{projectID}
}

func getEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}

	return fallback
}

func getInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}

	return value
}

func getBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}

	return value
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}

	return out
}
