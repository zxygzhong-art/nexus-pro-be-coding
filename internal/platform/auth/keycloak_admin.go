package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"nexus-pro-be/internal/domain"
)

// KeycloakAdminConfig 定義 Keycloak 管理員組態的資料結構。
type KeycloakAdminConfig struct {
	IssuerURL         string
	ClientID          string
	ClientSecret      string
	SendInviteEmail   bool
	InviteClientID    string
	InviteRedirectURL string
}

// KeycloakAdminClient 定義 Keycloak 管理員 client 的資料結構。
type KeycloakAdminClient struct {
	tokenURL          string
	usersURL          string
	clientID          string
	clientSecret      string
	sendInviteEmail   bool
	inviteClientID    string
	inviteRedirectURL string
	client            *http.Client

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

const keycloakUpdatePasswordAction = "UPDATE_PASSWORD"

// NewKeycloakAdminClient 建立 Keycloak 管理員 client。
func NewKeycloakAdminClient(cfg KeycloakAdminConfig, client *http.Client) (*KeycloakAdminClient, error) {
	tokenURL, usersURL, err := keycloakAdminURLs(cfg.IssuerURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.ClientID) == "" {
		return nil, errors.New("keycloak admin client id is required")
	}
	if strings.TrimSpace(cfg.ClientSecret) == "" {
		return nil, errors.New("keycloak admin client secret is required")
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &KeycloakAdminClient{
		tokenURL:          tokenURL,
		usersURL:          usersURL,
		clientID:          strings.TrimSpace(cfg.ClientID),
		clientSecret:      cfg.ClientSecret,
		sendInviteEmail:   cfg.SendInviteEmail,
		inviteClientID:    strings.TrimSpace(cfg.InviteClientID),
		inviteRedirectURL: strings.TrimSpace(cfg.InviteRedirectURL),
		client:            client,
	}, nil
}

// Ping 檢查外部服務連線狀態。
func (c *KeycloakAdminClient) Ping(ctx context.Context) error {
	if c == nil {
		return errors.New("keycloak admin client is not configured")
	}
	_, err := c.adminToken(ctx)
	return err
}

// EnsureUser 確保使用者。
func (c *KeycloakAdminClient) EnsureUser(ctx context.Context, input domain.IdentityProvisioningInput) (domain.ProvisionedIdentity, error) {
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if email == "" {
		return domain.ProvisionedIdentity{}, errors.New("keycloak user email is required")
	}
	token, err := c.adminToken(ctx)
	if err != nil {
		return domain.ProvisionedIdentity{}, err
	}
	user, ok, err := c.findUserByEmail(ctx, token, email)
	if err != nil {
		return domain.ProvisionedIdentity{}, err
	}
	payload := keycloakUserRepresentation{
		Username:      email,
		Email:         email,
		EmailVerified: true,
		Enabled:       input.Enabled,
		FirstName:     strings.TrimSpace(input.DisplayName),
		Attributes:    keycloakProvisioningAttributes(input, user.Attributes),
	}
	if input.SendInvite {
		payload.RequiredActions = appendKeycloakRequiredAction(user.RequiredActions, keycloakUpdatePasswordAction)
	}
	if ok {
		if err := c.updateUser(ctx, token, user.ID, payload); err != nil {
			return domain.ProvisionedIdentity{}, err
		}
		if input.SendInvite && c.sendInviteEmail {
			if err := c.sendRequiredActionsEmail(ctx, token, user.ID); err != nil {
				return domain.ProvisionedIdentity{}, err
			}
		}
		return domain.ProvisionedIdentity{Provider: domain.IdentityProviderKeycloak, Subject: user.ID, Email: email}, nil
	}
	createdID, err := c.createUser(ctx, token, payload)
	if err != nil {
		return domain.ProvisionedIdentity{}, err
	}
	if input.SendInvite && c.sendInviteEmail {
		if err := c.sendRequiredActionsEmail(ctx, token, createdID); err != nil {
			return domain.ProvisionedIdentity{}, err
		}
	}
	return domain.ProvisionedIdentity{Provider: domain.IdentityProviderKeycloak, Subject: createdID, Email: email}, nil
}

type keycloakUserRepresentation struct {
	ID              string              `json:"id,omitempty"`
	Username        string              `json:"username,omitempty"`
	Email           string              `json:"email,omitempty"`
	EmailVerified   bool                `json:"emailVerified"`
	Enabled         bool                `json:"enabled"`
	FirstName       string              `json:"firstName,omitempty"`
	Attributes      map[string][]string `json:"attributes,omitempty"`
	RequiredActions []string            `json:"requiredActions,omitempty"`
}

// keycloakAdminURLs 處理 Keycloak 管理員 URLs。
func keycloakAdminURLs(issuerURL string) (string, string, error) {
	raw := strings.TrimRight(strings.TrimSpace(issuerURL), "/")
	if raw == "" {
		return "", "", errors.New("keycloak issuer url is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "", errors.New("keycloak issuer url must be a valid URL")
	}
	realmPath := parsed.EscapedPath()
	idx := strings.Index(realmPath, "/realms/")
	if idx < 0 {
		return "", "", errors.New("keycloak issuer url must include /realms/{realm}")
	}
	prefix := strings.TrimSuffix(realmPath[:idx], "/")
	realm := strings.Trim(realmPath[idx+len("/realms/"):], "/")
	if realm == "" || strings.Contains(realm, "/") {
		return "", "", errors.New("keycloak issuer url must include exactly one realm segment")
	}
	tokenURL := raw + "/protocol/openid-connect/token"
	admin := *parsed
	admin.RawQuery = ""
	admin.Fragment = ""
	admin.Path = path.Join(prefix, "admin", "realms", realm, "users")
	if strings.HasPrefix(prefix, "/") {
		admin.Path = "/" + strings.TrimPrefix(admin.Path, "/")
	}
	return tokenURL, admin.String(), nil
}

// keycloakProvisioningAttributes 處理 Keycloak 開通 attributes。
func keycloakProvisioningAttributes(input domain.IdentityProvisioningInput, existing map[string][]string) map[string][]string {
	attrs := copyKeycloakAttributes(existing)
	set := func(key, value string) {
		if strings.TrimSpace(value) != "" {
			attrs[key] = []string{strings.TrimSpace(value)}
		}
	}
	set("tenant_id", input.TenantID)
	set("account_id", input.AccountID)
	set("employee_id", input.EmployeeID)
	set("employee_no", input.EmployeeNo)
	return attrs
}

// copyKeycloakAttributes 複製 Keycloak attributes。
func copyKeycloakAttributes(src map[string][]string) map[string][]string {
	dst := make(map[string][]string, len(src)+4)
	for key, values := range src {
		dst[key] = append([]string(nil), values...)
	}
	return dst
}

// appendKeycloakRequiredAction 附加 Keycloak required action。
func appendKeycloakRequiredAction(actions []string, action string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(actions)+1)
	for _, item := range actions {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	if _, ok := seen[action]; !ok {
		out = append(out, action)
	}
	return out
}

// adminToken 處理管理員 token。
func (c *KeycloakAdminClient) adminToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry) {
		token := c.accessToken
		c.mu.Unlock()
		return token, nil
	}
	c.mu.Unlock()

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", c.clientID)
	form.Set("client_secret", c.clientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", keycloakHTTPError("admin token", resp)
	}
	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	if strings.TrimSpace(body.AccessToken) == "" {
		return "", errors.New("keycloak admin token response missing access_token")
	}
	expiresIn := time.Duration(body.ExpiresIn) * time.Second
	if expiresIn <= 0 {
		expiresIn = time.Minute
	}
	c.mu.Lock()
	c.accessToken = body.AccessToken
	c.tokenExpiry = time.Now().Add(expiresIn - 30*time.Second)
	if c.tokenExpiry.Before(time.Now()) {
		c.tokenExpiry = time.Now().Add(expiresIn / 2)
	}
	c.mu.Unlock()
	return body.AccessToken, nil
}

// findUserByEmail 處理 find 使用者 by email。
func (c *KeycloakAdminClient) findUserByEmail(ctx context.Context, token string, email string) (keycloakUserRepresentation, bool, error) {
	endpoint, err := url.Parse(c.usersURL)
	if err != nil {
		return keycloakUserRepresentation{}, false, err
	}
	query := endpoint.Query()
	query.Set("email", email)
	query.Set("exact", "true")
	endpoint.RawQuery = query.Encode()
	var users []keycloakUserRepresentation
	if err := c.doJSON(ctx, http.MethodGet, endpoint.String(), token, nil, &users); err != nil {
		return keycloakUserRepresentation{}, false, err
	}
	for _, user := range users {
		if strings.EqualFold(strings.TrimSpace(user.Email), email) || strings.EqualFold(strings.TrimSpace(user.Username), email) {
			if strings.TrimSpace(user.ID) == "" {
				return keycloakUserRepresentation{}, false, errors.New("keycloak user response missing id")
			}
			return user, true, nil
		}
	}
	return keycloakUserRepresentation{}, false, nil
}

// createUser 建立使用者。
func (c *KeycloakAdminClient) createUser(ctx context.Context, token string, payload keycloakUserRepresentation) (string, error) {
	status, headers, err := c.doJSONWithStatus(ctx, http.MethodPost, c.usersURL, token, payload, nil)
	if err != nil {
		if status == http.StatusConflict {
			user, ok, findErr := c.findUserByEmail(ctx, token, payload.Email)
			if findErr != nil {
				return "", findErr
			}
			if ok {
				return user.ID, nil
			}
		}
		return "", err
	}
	if status != http.StatusCreated && status != http.StatusNoContent {
		return "", fmt.Errorf("keycloak create user returned unexpected status %d", status)
	}
	if location := headers.Get("Location"); location != "" {
		parts := strings.Split(strings.TrimRight(location, "/"), "/")
		if len(parts) > 0 && strings.TrimSpace(parts[len(parts)-1]) != "" {
			return strings.TrimSpace(parts[len(parts)-1]), nil
		}
	}
	user, ok, err := c.findUserByEmail(ctx, token, payload.Email)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errors.New("keycloak user was created but could not be found by email")
	}
	return user.ID, nil
}

// updateUser 更新使用者。
func (c *KeycloakAdminClient) updateUser(ctx context.Context, token string, userID string, payload keycloakUserRepresentation) error {
	endpoint := strings.TrimRight(c.usersURL, "/") + "/" + url.PathEscape(userID)
	return c.doJSON(ctx, http.MethodPut, endpoint, token, payload, nil)
}

// sendRequiredActionsEmail 處理 send required actions email。
func (c *KeycloakAdminClient) sendRequiredActionsEmail(ctx context.Context, token string, userID string) error {
	endpoint, err := url.Parse(strings.TrimRight(c.usersURL, "/") + "/" + url.PathEscape(userID) + "/execute-actions-email")
	if err != nil {
		return err
	}
	query := endpoint.Query()
	if c.inviteClientID != "" {
		query.Set("client_id", c.inviteClientID)
	}
	if c.inviteRedirectURL != "" {
		query.Set("redirect_uri", c.inviteRedirectURL)
	}
	query.Set("lifespan", "86400")
	endpoint.RawQuery = query.Encode()
	return c.doJSON(ctx, http.MethodPut, endpoint.String(), token, []string{keycloakUpdatePasswordAction}, nil)
}

// doJSON 處理 do JSON。
func (c *KeycloakAdminClient) doJSON(ctx context.Context, method, endpoint, token string, payload any, out any) error {
	_, _, err := c.doJSONWithStatus(ctx, method, endpoint, token, payload, out)
	return err
}

// doJSONWithStatus 處理 do JSON with 狀態。
func (c *KeycloakAdminClient) doJSONWithStatus(ctx context.Context, method, endpoint, token string, payload any, out any) (int, http.Header, error) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, resp.Header, keycloakHTTPError(method+" "+endpoint, resp)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, resp.Header, err
		}
	} else {
		_, _ = io.Copy(io.Discard, resp.Body)
	}
	return resp.StatusCode, resp.Header, nil
}

// keycloakHTTPError 處理 Keycloak HTTP 錯誤。
func keycloakHTTPError(operation string, resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	detail := strings.TrimSpace(string(raw))
	if detail == "" {
		return fmt.Errorf("keycloak %s failed with status %d", operation, resp.StatusCode)
	}
	return fmt.Errorf("keycloak %s failed with status %d: %s", operation, resp.StatusCode, detail)
}
