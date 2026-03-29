package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type ZitadelIntrospectionConfig struct {
	Issuer            string
	IntrospectionURI  string
	CacheTTL          time.Duration
	AllowedAudiences  []string
	AllowedServiceIDs []string
	APIClientID       string
	APIClientSecret   string
	HTTPClient        *http.Client
}

type ZitadelIntrospectionValidator struct {
	issuer            string
	introspectionURI  string
	httpClient        *http.Client
	cacheTTL          time.Duration
	allowedAudiences  map[string]struct{}
	allowedServiceIDs map[string]struct{}
	basicAuthHeader   string

	mu    sync.RWMutex
	cache map[string]cachedIntrospection
}

type cachedIntrospection struct {
	identity *Identity
	expires  time.Time
}

type introspectionResponse struct {
	Active    bool        `json:"active"`
	Audience  []string    `json:"aud"`
	ClientID  string      `json:"client_id"`
	ExpiresAt int64       `json:"exp"`
	IssuedAt  int64       `json:"iat"`
	Issuer    string      `json:"iss"`
	Scope     string      `json:"scope"`
	Subject   string      `json:"sub"`
	Username  string      `json:"username"`
	TokenType string      `json:"token_type"`
	Azp       string      `json:"azp"`
	ExtraAud  interface{} `json:"audiences"`
}

func NewZitadelIntrospectionValidator(cfg ZitadelIntrospectionConfig) (*ZitadelIntrospectionValidator, error) {
	issuer := strings.TrimRight(strings.TrimSpace(cfg.Issuer), "/")
	uri := strings.TrimSpace(cfg.IntrospectionURI)
	if issuer == "" {
		return nil, fmt.Errorf("issuer is required")
	}
	if uri == "" {
		return nil, fmt.Errorf("introspection uri is required")
	}
	if len(cfg.AllowedAudiences) == 0 {
		return nil, fmt.Errorf("at least one allowed audience is required")
	}
	if len(cfg.AllowedServiceIDs) == 0 {
		return nil, fmt.Errorf("at least one allowed service id is required")
	}
	if strings.TrimSpace(cfg.APIClientID) == "" {
		return nil, fmt.Errorf("api client id is required")
	}
	if strings.TrimSpace(cfg.APIClientSecret) == "" {
		return nil, fmt.Errorf("api client secret is required")
	}

	ttl := cfg.CacheTTL
	if ttl <= 0 {
		ttl = 15 * time.Second
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}

	basic := base64.StdEncoding.EncodeToString([]byte(url.QueryEscape(cfg.APIClientID) + ":" + url.QueryEscape(cfg.APIClientSecret)))

	return &ZitadelIntrospectionValidator{
		issuer:            issuer,
		introspectionURI:  uri,
		httpClient:        httpClient,
		cacheTTL:          ttl,
		allowedAudiences:  toSet(cfg.AllowedAudiences),
		allowedServiceIDs: toSet(cfg.AllowedServiceIDs),
		basicAuthHeader:   "Basic " + basic,
		cache:             make(map[string]cachedIntrospection),
	}, nil
}

func (v *ZitadelIntrospectionValidator) ValidateToken(ctx context.Context, rawToken string) (*Identity, error) {
	if identity, ok := v.cached(rawToken); ok {
		return identity, nil
	}

	identity, expiresAt, err := v.introspect(ctx, rawToken)
	if err != nil {
		return nil, err
	}

	v.store(rawToken, identity, expiresAt)
	return identity, nil
}

func (v *ZitadelIntrospectionValidator) cached(rawToken string) (*Identity, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	entry, ok := v.cache[rawToken]
	if !ok || time.Now().After(entry.expires) {
		return nil, false
	}

	return entry.identity, true
}

func (v *ZitadelIntrospectionValidator) store(rawToken string, identity *Identity, expiresAt time.Time) {
	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(v.cacheTTL)
	}
	if ttlExpiry := time.Now().Add(v.cacheTTL); ttlExpiry.Before(expiresAt) {
		expiresAt = ttlExpiry
	}

	v.mu.Lock()
	v.cache[rawToken] = cachedIntrospection{identity: identity, expires: expiresAt}
	v.mu.Unlock()
}

func (v *ZitadelIntrospectionValidator) introspect(ctx context.Context, rawToken string) (*Identity, time.Time, error) {
	form := url.Values{}
	form.Set("token", rawToken)
	form.Set("token_type_hint", "access_token")
	form.Set("scope", "openid")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.introspectionURI, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, time.Time{}, err
	}
	req.Header.Set("Authorization", v.basicAuthHeader)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, time.Time{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, time.Time{}, fmt.Errorf("introspection failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var data introspectionResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, time.Time{}, err
	}
	if !data.Active {
		return nil, time.Time{}, fmt.Errorf("token is inactive")
	}
	if strings.TrimSpace(data.Issuer) != v.issuer {
		return nil, time.Time{}, fmt.Errorf("token issuer is not allowed")
	}

	audiences := data.Audience
	if len(audiences) == 0 {
		audiences = extractStringSlice(data.ExtraAud)
	}
	if !intersectsSet(audiences, v.allowedAudiences) {
		return nil, time.Time{}, fmt.Errorf("token audience is not allowed")
	}

	principal := firstAllowed([]string{data.ClientID, data.Azp, data.Subject}, v.allowedServiceIDs)
	if principal == "" {
		return nil, time.Time{}, fmt.Errorf("token principal is not allowlisted")
	}

	identity := &Identity{
		Token:           rawToken,
		Principal:       principal,
		Subject:         strings.TrimSpace(data.Subject),
		ClientID:        strings.TrimSpace(data.ClientID),
		AuthorizedParty: strings.TrimSpace(data.Azp),
		Issuer:          strings.TrimSpace(data.Issuer),
		Audience:        audiences,
		Scopes:          strings.Fields(strings.TrimSpace(data.Scope)),
	}

	var expiresAt time.Time
	if data.ExpiresAt > 0 {
		expiresAt = time.Unix(data.ExpiresAt, 0)
	}

	return identity, expiresAt, nil
}

func extractStringSlice(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{typed}
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if strings.TrimSpace(item) != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if str, ok := item.(string); ok && strings.TrimSpace(str) != "" {
				out = append(out, str)
			}
		}
		return out
	default:
		return nil
	}
}

func intersectsSet(values []string, allowed map[string]struct{}) bool {
	for _, value := range values {
		if _, ok := allowed[strings.TrimSpace(value)]; ok {
			return true
		}
	}
	return false
}

func firstAllowed(values []string, allowed map[string]struct{}) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := allowed[value]; ok {
			return value
		}
	}
	return ""
}

func toSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}
