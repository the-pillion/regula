package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type ZitadelJWTConfig struct {
	Issuer           string
	JWKSURI          string
	JWKSCacheTTL     time.Duration
	AllowedAudiences []string
	AllowedClientIDs []string
	HTTPClient       *http.Client
}

type ZitadelJWTValidator struct {
	issuer           string
	jwks             *jwksCache
	allowedAudiences map[string]struct{}
	allowedClientIDs map[string]struct{}
}

func NewZitadelJWTValidator(cfg ZitadelJWTConfig) (*ZitadelJWTValidator, error) {
	issuer := strings.TrimRight(strings.TrimSpace(cfg.Issuer), "/")
	if issuer == "" {
		return nil, errors.New("jwt: issuer required")
	}
	jwksURI := strings.TrimSpace(cfg.JWKSURI)
	if jwksURI == "" {
		jwksURI = issuer + "/oauth/v2/keys"
	}
	if len(cfg.AllowedAudiences) == 0 {
		return nil, errors.New("jwt: at least one allowed audience required")
	}

	ttl := cfg.JWKSCacheTTL
	if ttl <= 0 {
		ttl = time.Hour
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}

	return &ZitadelJWTValidator{
		issuer:           issuer,
		jwks:             newJWKSCache(jwksURI, ttl, httpClient),
		allowedAudiences: toSet(cfg.AllowedAudiences),
		allowedClientIDs: toSet(cfg.AllowedClientIDs),
	}, nil
}

func (v *ZitadelJWTValidator) ValidateToken(ctx context.Context, rawToken string) (*Identity, error) {
	parser := jwt.NewParser(
		jwt.WithIssuer(v.issuer),
		jwt.WithExpirationRequired(),
		jwt.WithValidMethods([]string{"RS256"}),
	)

	claims := jwt.MapClaims{}
	_, err := parser.ParseWithClaims(rawToken, &claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		return v.jwks.key(ctx, kid)
	})
	if err != nil {
		return nil, fmt.Errorf("jwt parse: %w", err)
	}

	audiences := extractAudienceClaim(claims["aud"])
	if !intersectsSet(audiences, v.allowedAudiences) {
		return nil, errors.New("jwt: audience not allowed")
	}

	sub, _ := claims["sub"].(string)
	clientID, _ := claims["client_id"].(string)
	azp, _ := claims["azp"].(string)
	caller := firstNonEmpty(clientID, azp)

	identity := &Identity{
		Token:           rawToken,
		Subject:         strings.TrimSpace(sub),
		ClientID:        strings.TrimSpace(clientID),
		AuthorizedParty: strings.TrimSpace(azp),
		Issuer:          v.issuer,
		Audience:        audiences,
		Source:          "zitadel",
	}

	if len(v.allowedClientIDs) > 0 {
		if caller == "" {
			return nil, errors.New("jwt: client_id missing")
		}
		if _, ok := v.allowedClientIDs[caller]; !ok {
			return nil, errors.New("jwt: client_id not allowlisted")
		}
	}

	// regula's allowlist (REGULA_ALLOWED_SERVICE_IDS) is the set of service
	// users; the public website uses a separate unauthenticated path. So any
	// caller that passed the allowlist is a service. (The old sub==caller
	// heuristic never matched Zitadel service tokens, where sub is a numeric id
	// and client_id is the loginname.)
	if caller != "" {
		identity.Service = true
		identity.Principal = caller
		return identity, nil
	}

	identity.Principal = sub
	return identity, nil
}

type jwksCache struct {
	uri        string
	ttl        time.Duration
	httpClient *http.Client

	mu      sync.RWMutex
	keys    map[string]*rsa.PublicKey
	expires time.Time
}

func newJWKSCache(uri string, ttl time.Duration, httpClient *http.Client) *jwksCache {
	return &jwksCache{uri: uri, ttl: ttl, httpClient: httpClient, keys: map[string]*rsa.PublicKey{}}
}

func (c *jwksCache) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	c.mu.RLock()
	if time.Now().Before(c.expires) {
		if k, ok := c.keys[kid]; ok {
			c.mu.RUnlock()
			return k, nil
		}
	}
	c.mu.RUnlock()

	if err := c.refresh(ctx); err != nil {
		return nil, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	k, ok := c.keys[kid]
	if !ok {
		return nil, fmt.Errorf("jwks: kid %q not found", kid)
	}
	return k, nil
}

func (c *jwksCache) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.uri, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("jwks: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}

	next := make(map[string]*rsa.PublicKey, len(payload.Keys))
	for _, k := range payload.Keys {
		if k.Kty != "RSA" {
			continue
		}
		pub, err := parseRSAKey(k.N, k.E)
		if err != nil {
			continue
		}
		next[k.Kid] = pub
	}

	c.mu.Lock()
	c.keys = next
	c.expires = time.Now().Add(c.ttl)
	c.mu.Unlock()
	return nil
}

func parseRSAKey(modulus, exponent string) (*rsa.PublicKey, error) {
	n, err := base64.RawURLEncoding.DecodeString(modulus)
	if err != nil {
		return nil, fmt.Errorf("decode modulus: %w", err)
	}
	e, err := base64.RawURLEncoding.DecodeString(exponent)
	if err != nil {
		return nil, fmt.Errorf("decode exponent: %w", err)
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(n),
		E: int(new(big.Int).SetBytes(e).Int64()),
	}, nil
}

func extractAudienceClaim(raw any) []string {
	switch v := raw.(type) {
	case nil:
		return nil
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func toSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			out[v] = struct{}{}
		}
	}
	return out
}

func intersectsSet(values []string, allowed map[string]struct{}) bool {
	for _, v := range values {
		if _, ok := allowed[strings.TrimSpace(v)]; ok {
			return true
		}
	}
	return false
}
