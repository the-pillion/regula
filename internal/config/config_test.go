package config

import (
	"os"
	"testing"
	"time"
)

func TestSplitCSV(t *testing.T) {
	got := splitCSV(" one, two ,,three ")
	if len(got) != 3 || got[0] != "one" || got[1] != "two" || got[2] != "three" {
		t.Fatalf("unexpected tokens: %#v", got)
	}
}

func TestLoadIncludesCacheSettings(t *testing.T) {
	t.Setenv("REGULA_DATABASE_URL", "postgresql://example")
	t.Setenv("ZITADEL_ISSUER", "https://ztdl.example.com")
	t.Setenv("ZITADEL_PROJECT_ID", "pillion-project")
	t.Setenv("REGULA_ALLOWED_SERVICE_IDS", "pillion-svc")
	t.Setenv("REGULA_DOCUMENT_CACHE_TTL_SECONDS", "45")
	t.Setenv("REGULA_DOCUMENT_CACHE_MAX_ITEMS", "16")
	t.Setenv("REGULA_SERVICE_NAME", "regula-test")
	t.Setenv("REGULA_HTTP_PORT", "8090")
	t.Setenv("REGULA_AUTO_MIGRATE", "false")
	t.Setenv("REGULA_LOG_LEVEL", "debug")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected config to load, got error: %v", err)
	}

	if cfg.ServiceName != "regula-test" {
		t.Fatalf("unexpected service name: %s", cfg.ServiceName)
	}
	if cfg.HTTPPort != 8090 {
		t.Fatalf("unexpected port: %d", cfg.HTTPPort)
	}
	if cfg.AutoMigrate {
		t.Fatalf("expected auto-migrate to be false")
	}
	if cfg.DocumentCacheTTL != 45*time.Second {
		t.Fatalf("unexpected cache ttl: %s", cfg.DocumentCacheTTL)
	}
	if cfg.DocumentCacheMaxItems != 16 {
		t.Fatalf("unexpected cache max items: %d", cfg.DocumentCacheMaxItems)
	}
	if cfg.ZitadelJWKSURI != "https://ztdl.example.com/oauth/v2/keys" {
		t.Fatalf("unexpected jwks uri: %s", cfg.ZitadelJWKSURI)
	}
}

func TestLoadRejectsNegativeCacheSize(t *testing.T) {
	t.Setenv("REGULA_DATABASE_URL", "postgresql://example")
	t.Setenv("ZITADEL_ISSUER", "https://ztdl.example.com")
	t.Setenv("ZITADEL_PROJECT_ID", "pillion-project")
	t.Setenv("REGULA_ALLOWED_SERVICE_IDS", "pillion-svc")
	t.Setenv("REGULA_DOCUMENT_CACHE_MAX_ITEMS", "-1")

	_, err := Load()
	if err == nil {
		t.Fatal("expected negative cache size to fail")
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}
