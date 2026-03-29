package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddlewareRejectsMissingToken(t *testing.T) {
	handler := Middleware(stubValidator{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	handler := Middleware(stubValidator{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

type stubValidator struct{}

func (stubValidator) ValidateToken(_ context.Context, rawToken string) (*Identity, error) {
	if rawToken != "abc" {
		return nil, fmt.Errorf("token not allowlisted")
	}

	return &Identity{Principal: "abc", Token: rawToken}, nil
}
