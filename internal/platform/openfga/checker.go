package openfga

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"nexus-pro-be/internal/domain"
	authzpkg "nexus-pro-be/internal/domain/authz"
)

type Checker struct {
	apiURL  string
	storeID string
	client  *http.Client
}

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

func (c *Checker) CheckRelationship(ctx context.Context, check authzpkg.RelationshipCheck) (bool, error) {
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
	raw, err := json.Marshal(body)
	if err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+"/stores/"+c.storeID+"/check", bytes.NewReader(raw))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, errors.New("openfga check failed")
	}
	var payload struct {
		Allowed bool `json:"allowed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return false, err
	}
	return payload.Allowed, nil
}

func (c *Checker) WriteRelationshipTuples(ctx context.Context, changes []domain.AuthzRelationshipTupleChange) error {
	if c == nil || c.apiURL == "" || c.storeID == "" || len(changes) == 0 {
		return nil
	}
	writes := make([]map[string]string, 0, len(changes))
	deletes := make([]map[string]string, 0, len(changes))
	for _, change := range changes {
		tuple := change.Tuple
		key := map[string]string{
			"user":     tuple.SubjectType + ":" + tuple.SubjectID,
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
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("openfga write failed")
	}
	return nil
}
