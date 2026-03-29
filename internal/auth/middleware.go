package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

type contextKey string

const identityContextKey contextKey = "regulaAuthIdentity"

type Identity struct {
	Token           string
	Principal       string
	Subject         string
	ClientID        string
	AuthorizedParty string
	Issuer          string
	Audience        []string
	Scopes          []string
}

type Validator interface {
	ValidateToken(ctx context.Context, rawToken string) (*Identity, error)
}

func Middleware(validator Validator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
			token, ok := strings.CutPrefix(authHeader, "Bearer ")
			if !ok || strings.TrimSpace(token) == "" {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}

			identity, err := validator.ValidateToken(r.Context(), strings.TrimSpace(token))
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid bearer token: %v", err), http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), identityContextKey, identity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func IdentityFromContext(ctx context.Context) (*Identity, bool) {
	identity, ok := ctx.Value(identityContextKey).(*Identity)
	return identity, ok
}
