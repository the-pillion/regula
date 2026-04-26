package dashboard

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"

	"github.com/pillion/regula/internal/config"
)

// BasicAuth returns an HTTP middleware that gates the dashboard
// behind a single shared username/password from env. Constant-time
// compare via SHA-256 digest to avoid leaking credential length.
//
// One user, no sessions, no role model — by design. The dashboard is
// a standalone admin app; Zitadel intentionally stays scoped to /v1/*
// M2M only (see AGENT_MEMORY Decision 8).
func BasicAuth(cfg config.DashboardConfig) func(http.Handler) http.Handler {
	wantUser := sha256.Sum256([]byte(cfg.Username))
	wantPass := sha256.Sum256([]byte(cfg.Password))
	realm := cfg.Realm
	if realm == "" {
		realm = "Regula Admin"
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, pass, ok := r.BasicAuth()
			if !ok {
				challenge(w, realm)
				return
			}
			gotUser := sha256.Sum256([]byte(user))
			gotPass := sha256.Sum256([]byte(pass))
			if subtle.ConstantTimeCompare(gotUser[:], wantUser[:]) != 1 ||
				subtle.ConstantTimeCompare(gotPass[:], wantPass[:]) != 1 {
				challenge(w, realm)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func challenge(w http.ResponseWriter, realm string) {
	w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`", charset="UTF-8"`)
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}
