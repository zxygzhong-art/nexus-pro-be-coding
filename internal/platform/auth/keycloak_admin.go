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

	"nexus-pro-api/internal/domain"
)

// KeycloakAdminConfig 定義 Keycloak 管理員組態的資料結構。
type KeycloakAdminConfig struct {
	IssuerURL         string
	ClientID          string
	ClientSecret      string
	LoginClientID     string
	SendInviteEmail   bool
	InviteClientID    string
	InviteRedirectURL string
}

// KeycloakAdminClient 定義 Keycloak 管理員 client 的資料結構。
type KeycloakAdminClient struct {
	tokenURL          string
	logoutURL         string
	usersURL          string
	clientID          string
	clientSecret      string
	loginClientID     string
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
		logoutURL:         strings.TrimSuffix(tokenURL, "/token") + "/logout",
		usersURL:          usersURL,
		clientID:          strings.TrimSpace(cfg.ClientID),
		clientSecret:      cfg.ClientSecret,
		loginClientID:     strings.TrimSpace(cfg.LoginClientID),
		sendInviteEmail:   cfg.SendInviteEmail,
		inviteClientID:    strings.TrimSpace(cfg.InviteClientID),
		inviteRedirectURL: strings.TrimSpace(cfg.InviteRedirectURL),
		client:            client,
	}, nil
}

// ChangePassword verifies the existing credential and resets only the already bound Keycloak subject.
func (c *KeycloakAdminClient) ChangePassword(ctx context.Context, input domain.IdentityPasswordChangeInput) error {
	if c == nil || c.loginClientID == "" {
		return domain.ErrIdentityPasswordUnavailable
	}
	if strings.TrimSpace(input.Subject) == "" || strings.TrimSpace(input.TenantID) == "" || strings.TrimSpace(input.AccountID) == "" {
		return domain.ErrIdentityPasswordUnavailable
	}
	if input.CurrentPassword == "" || input.NewPassword == "" {
		return domain.ErrIdentityPasswordUnavailable
	}
	adminToken, err := c.adminToken(ctx)
	if err != nil {
		return err
	}
	user, err := c.getUserByID(ctx, adminToken, input.Subject)
	if err != nil {
		return err
	}
	if err := validateKeycloakUserOwnership(domain.IdentityProvisioningInput{
		TenantID:  input.TenantID,
		AccountID: input.AccountID,
	}, user.Attributes); err != nil {
		return err
	}
	username := strings.TrimSpace(user.Username)
	if username == "" {
		username = strings.TrimSpace(user.Email)
	}
	if username == "" {
		return domain.ErrIdentityPasswordUnavailable
	}
	if err := c.verifyCurrentPassword(ctx, username, input.CurrentPassword); err != nil {
		return err
	}
	return c.resetPassword(ctx, adminToken, input.Subject, input.NewPassword)
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
	if ok {
		if err := validateKeycloakUserOwnership(input, user.Attributes); err != nil {
			return domain.ProvisionedIdentity{}, err
		}
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
	createdUser, found, err := c.findUserByEmail(ctx, token, email)
	if err != nil {
		return domain.ProvisionedIdentity{}, err
	}
	if !found || createdUser.ID != createdID {
		return domain.ProvisionedIdentity{}, errors.New("keycloak user ownership could not be verified after creation")
	}
	if err := validateKeycloakUserOwnership(input, createdUser.Attributes); err != nil {
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

// validateKeycloakUserOwnership 在任何 PUT 或本地綁定前驗證 realm-global 使用者仍屬於相同 tenant/account。
func validateKeycloakUserOwnership(input domain.IdentityProvisioningInput, attributes map[string][]string) error {
	existingTenant := firstKeycloakAttribute(attributes, "tenant_id")
	if existingTenant != "" && existingTenant != strings.TrimSpace(input.TenantID) {
		return domain.IdentityProvisioningOwnershipConflict("keycloak user is already owned by another tenant")
	}
	existingAccount := firstKeycloakAttribute(attributes, "account_id")
	if existingAccount != "" && existingAccount != strings.TrimSpace(input.AccountID) && !sameKeycloakEmployee(input, attributes) {
		return domain.IdentityProvisioningOwnershipConflict("keycloak user is already owned by another account")
	}
	return nil
}

// sameKeycloakEmployee permits account-ID rotation only when an immutable-in-practice employee key still matches.
func sameKeycloakEmployee(input domain.IdentityProvisioningInput, attributes map[string][]string) bool {
	existingEmployeeID := firstKeycloakAttribute(attributes, "employee_id")
	if employeeID := strings.TrimSpace(input.EmployeeID); employeeID != "" && existingEmployeeID != "" && existingEmployeeID == employeeID {
		return true
	}
	existingEmployeeNo := firstKeycloakAttribute(attributes, "employee_no")
	employeeNo := strings.TrimSpace(input.EmployeeNo)
	return employeeNo != "" && existingEmployeeNo != "" && existingEmployeeNo == employeeNo
}

// firstKeycloakAttribute 取得 Keycloak 單值 ownership attribute。
func firstKeycloakAttribute(attributes map[string][]string, key string) string {
	for _, value := range attributes[key] {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
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
	query.Set("briefRepresentation", "false")
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

// getUserByID resolves the immutable Keycloak subject before any credential mutation.
func (c *KeycloakAdminClient) getUserByID(ctx context.Context, token, userID string) (keycloakUserRepresentation, error) {
	var user keycloakUserRepresentation
	endpoint := strings.TrimRight(c.usersURL, "/") + "/" + url.PathEscape(strings.TrimSpace(userID))
	if err := c.doJSON(ctx, http.MethodGet, endpoint, token, nil, &user); err != nil {
		return keycloakUserRepresentation{}, err
	}
	if strings.TrimSpace(user.ID) == "" || user.ID != strings.TrimSpace(userID) {
		return keycloakUserRepresentation{}, errors.New("keycloak user response subject mismatch")
	}
	return user, nil
}

// verifyCurrentPassword uses the login client and immediately closes the short-lived verification session.
func (c *KeycloakAdminClient) verifyCurrentPassword(ctx context.Context, username, password string) error {
	form := url.Values{}
	form.Set("grant_type", "password")
	form.Set("client_id", c.loginClientID)
	form.Set("username", username)
	form.Set("password", password)
	form.Set("scope", "openid")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized {
		_, _ = io.Copy(io.Discard, resp.Body)
		return domain.ErrIdentityCurrentPasswordInvalid
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return keycloakHTTPError("current password verification", resp)
	}
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return err
	}
	if strings.TrimSpace(body.RefreshToken) == "" {
		return errors.New("keycloak password verification response missing refresh_token")
	}
	return c.closeVerificationSession(ctx, body.RefreshToken)
}

// closeVerificationSession prevents password verification from leaving an extra user session behind.
func (c *KeycloakAdminClient) closeVerificationSession(ctx context.Context, refreshToken string) error {
	form := url.Values{}
	form.Set("client_id", c.loginClientID)
	form.Set("refresh_token", refreshToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.logoutURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return keycloakHTTPError("password verification logout", resp)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// resetPassword applies Keycloak's password policy through the Admin REST credential endpoint.
func (c *KeycloakAdminClient) resetPassword(ctx context.Context, token, userID, password string) error {
	endpoint := strings.TrimRight(c.usersURL, "/") + "/" + url.PathEscape(strings.TrimSpace(userID)) + "/reset-password"
	payload := struct {
		Type      string `json:"type"`
		Value     string `json:"value"`
		Temporary bool   `json:"temporary"`
	}{Type: "password", Value: password, Temporary: false}
	status, _, err := c.doJSONWithStatus(ctx, http.MethodPut, endpoint, token, payload, nil)
	if err != nil && status == http.StatusBadRequest {
		return fmt.Errorf("%w: %v", domain.ErrIdentityPasswordRejected, err)
	}
	return err
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
