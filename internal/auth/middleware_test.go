package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddlewareRejectsMissingToken(t *testing.T) {
	handler := Middleware(stubValidator{}, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestMiddlewareAcceptsConfiguredToken(t *testing.T) {
	handler := Middleware(stubValidator{}, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, ok := IdentityFromContext(r.Context())
		if !ok || identity.Principal != "abc" {
			t.Fatal("expected identity in request context")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer abc")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
}

func TestMiddlewareLogsAuthFailures(t *testing.T) {
	var sink strings.Builder
	logger := slog.New(slog.NewTextHandler(&sink, nil))

	handler := Middleware(stubValidator{}, LogFailureWith(logger))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/subjects/demo/bundle", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("User-Agent", "regula-test")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	if !strings.Contains(sink.String(), "auth_failure") || !strings.Contains(sink.String(), "missing_bearer_token") {
		t.Fatalf("expected auth failure log, got %q", sink.String())
	}
}

type stubValidator struct{}

func (stubValidator) ValidateToken(_ context.Context, rawToken string) (*Identity, error) {
	if rawToken != "abc" {
		return nil, fmt.Errorf("token not allowlisted")
	}

	return &Identity{Principal: "abc", Token: rawToken}, nil
}
