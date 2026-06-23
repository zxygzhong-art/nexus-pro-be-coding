package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
)

// OIDCStateCodec signs stateless callback state values.
type OIDCStateCodec struct {
	signingKey []byte
	ttl        time.Duration
}

// NewOIDCStateCodec creates a signed state codec for external login callbacks.
func NewOIDCStateCodec(signingKey string, ttl time.Duration) *OIDCStateCodec {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &OIDCStateCodec{signingKey: []byte(strings.TrimSpace(signingKey)), ttl: ttl}
}

// EncodeOIDCState signs provider, tenant, and return URL callback context.
func (c *OIDCStateCodec) EncodeOIDCState(provider, tenantID, returnURL string) (string, error) {
	if c == nil || len(c.signingKey) == 0 {
		return "", errors.New("OIDC state signing key is required")
	}
	state := domain.OIDCState{
		Provider:  strings.TrimSpace(provider),
		TenantID:  strings.TrimSpace(tenantID),
		ReturnURL: strings.TrimSpace(returnURL),
		Nonce:     randomNonce(),
		ExpiresAt: time.Now().UTC().Add(c.ttl),
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return encoded + "." + c.signature(encoded), nil
}

// DecodeOIDCState verifies and decodes callback state.
func (c *OIDCStateCodec) DecodeOIDCState(raw string) (domain.OIDCState, error) {
	if c == nil || len(c.signingKey) == 0 {
		return domain.OIDCState{}, errors.New("OIDC state signing key is required")
	}
	parts := strings.Split(strings.TrimSpace(raw), ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return domain.OIDCState{}, domain.Unauthorized("invalid OIDC state")
	}
	if !hmac.Equal([]byte(parts[1]), []byte(c.signature(parts[0]))) {
		return domain.OIDCState{}, domain.Unauthorized("invalid OIDC state signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return domain.OIDCState{}, domain.Unauthorized("invalid OIDC state")
	}
	var state domain.OIDCState
	if err := json.Unmarshal(payload, &state); err != nil {
		return domain.OIDCState{}, domain.Unauthorized("invalid OIDC state")
	}
	if state.Provider == "" || state.TenantID == "" || state.ExpiresAt.IsZero() {
		return domain.OIDCState{}, domain.Unauthorized("invalid OIDC state")
	}
	if !time.Now().UTC().Before(state.ExpiresAt) {
		return domain.OIDCState{}, domain.Unauthorized("OIDC state expired")
	}
	return state, nil
}

func (c *OIDCStateCodec) signature(payload string) string {
	mac := hmac.New(sha256.New, c.signingKey)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func randomNonce() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return hex.EncodeToString(raw[:])
	}
	return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
}
