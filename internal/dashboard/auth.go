package dashboard

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"

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

// SameOriginUnsafeMethods rejects browser-originated cross-site POSTs to
// the basic-auth dashboard. It is intentionally small: Regula has no browser
// session state, but browsers will resend Basic Auth credentials across
// sites, so unsafe methods must stay same-origin.
func SameOriginUnsafeMethods(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		if sameOrigin(r, r.Header.Get("Origin")) {
			next.ServeHTTP(w, r)
			return
		}
		if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
			http.Error(w, "cross-origin dashboard write rejected", http.StatusForbidden)
			return
		}
		if ref := strings.TrimSpace(r.Header.Get("Referer")); ref != "" && !sameOrigin(r, ref) {
			http.Error(w, "cross-origin dashboard write rejected", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func sameOrigin(r *http.Request, raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}
