package openfga

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
)

const maxOpenFGAErrorBodyLength = 500

// Checker 定義 checker 的資料結構。
type Checker struct {
	apiURL    string
	storeID   string
	modelID   string
	authToken string
	client    *http.Client
}

// NewChecker 建立 checker。
func NewChecker(apiURL, storeID string, client *http.Client) *Checker {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &Checker{
		apiURL:  strings.TrimRight(strings.TrimSpace(apiURL), "/"),
		storeID: strings.TrimSpace(storeID),
		client:  client,
	}
}

// WithAuthorizationModelID 附加授權 model ID。
func (c *Checker) WithAuthorizationModelID(modelID string) *Checker {
	if c == nil {
		return c
	}
	c.modelID = strings.TrimSpace(modelID)
	return c
}

// WithAuthToken attaches the preshared bearer token sent on every OpenFGA request.
func (c *Checker) WithAuthToken(token string) *Checker {
	if c == nil {
		return c
	}
	c.authToken = strings.TrimSpace(token)
	return c
}

// authorize 在請求上附加 preshared 認證標頭（僅當 token 非空）。
func (c *Checker) authorize(req *http.Request) {
	if c.authToken == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.authToken)
}

// Ping 檢查外部服務連線狀態。
func (c *Checker) Ping(ctx context.Context) error {
	if c == nil || c.apiURL == "" || c.storeID == "" {
		return errors.New("openfga checker not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL+"/healthz", nil)
	if err != nil {
		return err
	}
	c.authorize(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := openFGAStatusError(resp, "healthz"); err != nil {
		return err
	}
	return c.verifyAuthorizationModel(ctx)
}

// CheckRelationship 檢查關係。
func (c *Checker) CheckRelationship(ctx context.Context, check domain.RelationshipCheck) (bool, error) {
	if c == nil || c.apiURL == "" || c.storeID == "" {
		return false, nil
	}
	body := map[string]any{
		"tuple_key": map[string]any{
			"user":     check.Subject,
			"relation": check.Relation,
			"object":   check.Object,
		},
		"context": map[string]any{
			"tenant_id": check.TenantID,
		},
	}
	if c.modelID != "" {
		body["authorization_model_id"] = c.modelID
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+"/stores/"+c.storeID+"/check", bytes.NewReader(raw))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.authorize(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if err := openFGAStatusError(resp, "check"); err != nil {
		return false, err
	}
	var payload struct {
		Allowed bool `json:"allowed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return false, err
	}
	return payload.Allowed, nil
}

// WriteRelationshipTuples 寫入關係 tuple。
func (c *Checker) WriteRelationshipTuples(ctx context.Context, changes []domain.AuthzRelationshipTupleChange) error {
	if c == nil || c.apiURL == "" || c.storeID == "" || len(changes) == 0 {
		return nil
	}
	writes := make([]map[string]string, 0, len(changes))
	deletes := make([]map[string]string, 0, len(changes))
	for _, change := range changes {
		tuple := change.Tuple
		key := map[string]string{
			"user":     openFGASubject(tuple.SubjectType, tuple.SubjectID),
			"relation": tuple.Relation,
			"object":   tuple.ObjectType + ":" + tuple.ObjectID,
		}
		switch change.Operation {
		case domain.AuthzRelationshipTupleDelete:
			deletes = append(deletes, key)
		default:
			writes = append(writes, key)
		}
	}
	body := map[string]any{}
	if len(writes) > 0 {
		body["writes"] = map[string]any{"tuple_keys": writes}
	}
	if len(deletes) > 0 {
		body["deletes"] = map[string]any{"tuple_keys": deletes}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+"/stores/"+c.storeID+"/write", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.authorize(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := openFGAStatusError(resp, "write"); err != nil {
		if isOpenFGAIdempotentTupleWriteError(err) {
			return nil
		}
		return err
	}
	return nil
}

func openFGASubject(subjectType, subjectID string) string {
	subjectType = strings.TrimSpace(subjectType)
	subjectID = strings.TrimSpace(subjectID)
	if subjectType == "" || subjectID == "" {
		return subjectType + ":" + subjectID
	}
	typ, relation, ok := strings.Cut(subjectType, "#")
	if !ok || strings.TrimSpace(relation) == "" {
		return subjectType + ":" + subjectID
	}
	return strings.TrimSpace(typ) + ":" + subjectID + "#" + strings.TrimSpace(relation)
}

// verifyAuthorizationModel 處理 verify 授權 model。
func (c *Checker) verifyAuthorizationModel(ctx context.Context) error {
	if c.modelID == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL+"/stores/"+c.storeID+"/authorization-models/"+c.modelID, nil)
	if err != nil {
		return err
	}
	c.authorize(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := openFGAStatusError(resp, "authorization model"); err != nil {
		return fmt.Errorf("openfga authorization model %q is not ready: %w", c.modelID, err)
	}
	return nil
}

// openFGAStatusError 處理 OpenFGA 狀態錯誤。
func openFGAStatusError(resp *http.Response, operation string) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("openfga %s failed: status=%d body=%q", operation, resp.StatusCode, readOpenFGAErrorBody(resp))
}

// isOpenFGAIdempotentTupleWriteError treats tuple replay conflicts as success.
func isOpenFGAIdempotentTupleWriteError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "tuple") {
		return false
	}
	return strings.Contains(message, "already exists") ||
		strings.Contains(message, "duplicate") ||
		strings.Contains(message, "does not exist") ||
		strings.Contains(message, "not found")
}

// readOpenFGAErrorBody 讀取 OpenFGA 錯誤 body。
func readOpenFGAErrorBody(resp *http.Response) string {
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxOpenFGAErrorBodyLength+1))
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(string(raw))
	if len(value) > maxOpenFGAErrorBodyLength {
		return value[:maxOpenFGAErrorBodyLength] + "..."
	}
	return value
}
