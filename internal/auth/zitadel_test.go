package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestZitadelIntrospectionValidatorAcceptsConfiguredServiceToken(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") == "" {
			t.Fatal("expected authorization header")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"active":    true,
			"iss":       "https://ztdl.example.com",
			"sub":       "366-service-user",
			"aud":       []string{"pillion-project"},
			"client_id": "pillion-svc",
			"scope":     "openid profile email",
			"exp":       time.Now().Add(time.Hour).Unix(),
		})
	}))
	defer server.Close()

	validator, err := NewZitadelIntrospectionValidator(ZitadelIntrospectionConfig{
		Issuer:            "https://ztdl.example.com",
		IntrospectionURI:  server.URL,
		AllowedAudiences:  []string{"pillion-project"},
		AllowedServiceIDs: []string{"pillion-svc"},
		APIClientID:       "regula-api",
		APIClientSecret:   "secret",
		CacheTTL:          time.Minute,
	})
	if err != nil {
		t.Fatalf("create validator: %v", err)
	}

	identity, err := validator.ValidateToken(t.Context(), "opaque-access-token")
	if err != nil {
		t.Fatalf("expected token to validate, got error: %v", err)
	}
	if identity.Principal != "pillion-svc" {
		t.Fatalf("unexpected principal: %s", identity.Principal)
	}
	if calls != 1 {
		t.Fatalf("expected one introspection call, got %d", calls)
	}

	_, err = validator.ValidateToken(t.Context(), "opaque-access-token")
	if err != nil {
		t.Fatalf("expected cached token to validate, got error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected cached introspection result, got %d calls", calls)
	}
}

func TestZitadelIntrospectionValidatorRejectsDisallowedPrincipal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"active":    true,
			"iss":       "https://ztdl.example.com",
			"sub":       "366-service-user",
			"aud":       []string{"pillion-project"},
			"client_id": "pillion-svc",
			"exp":       time.Now().Add(time.Hour).Unix(),
		})
	}))
	defer server.Close()

	validator, err := NewZitadelIntrospectionValidator(ZitadelIntrospectionConfig{
		Issuer:            "https://ztdl.example.com",
		IntrospectionURI:  server.URL,
		AllowedAudiences:  []string{"pillion-project"},
		AllowedServiceIDs: []string{"other-service"},
		APIClientID:       "regula-api",
		APIClientSecret:   "secret",
	})
	if err != nil {
		t.Fatalf("create validator: %v", err)
	}

	if _, err := validator.ValidateToken(t.Context(), "opaque-access-token"); err == nil {
		t.Fatal("expected validator to reject token principal")
	}
}
