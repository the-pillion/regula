package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
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

type FailureLogger func(r *http.Request, reason string, err error)

func Middleware(validator Validator, onFailure FailureLogger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
			token, ok := strings.CutPrefix(authHeader, "Bearer ")
			if !ok || strings.TrimSpace(token) == "" {
				logFailure(onFailure, r, "missing_bearer_token", nil)
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}

			identity, err := validator.ValidateToken(r.Context(), strings.TrimSpace(token))
			if err != nil {
				logFailure(onFailure, r, "invalid_bearer_token", err)
				http.Error(w, fmt.Sprintf("invalid bearer token: %v", err), http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), identityContextKey, identity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func LogFailureWith(logger *slog.Logger) FailureLogger {
	if logger == nil {
		return nil
	}

	return func(r *http.Request, reason string, err error) {
		fields := []any{
			"event", "auth_failure",
			"reason", reason,
			"method", r.Method,
			"path", r.URL.Path,
			"remote_ip", r.RemoteAddr,
			"request_id", middleware.GetReqID(r.Context()),
			"user_agent", r.UserAgent(),
		}

		if err != nil {
			fields = append(fields, "error", err.Error())
		}

		logger.Warn("regula auth rejected request", fields...)
	}
}

func logFailure(logger FailureLogger, r *http.Request, reason string, err error) {
	if logger != nil {
		logger(r, reason, err)
	}
}

func IdentityFromContext(ctx context.Context) (*Identity, bool) {
	identity, ok := ctx.Value(identityContextKey).(*Identity)
	return identity, ok
}
