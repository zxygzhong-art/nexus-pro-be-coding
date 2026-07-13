package domain

import "time"

const (
	EHRMSSyncRunStatusRunning   = "running"
	EHRMSSyncRunStatusSucceeded = "succeeded"
	EHRMSSyncRunStatusPartial   = "partial"
	EHRMSSyncRunStatusFailed    = "failed"
	EHRMSSyncRunStatusSkipped   = "skipped"
)

// EHRMSSyncRun 保存一次 eHRMS 同步的可運維狀態，不保存上游原始人員資料。
type EHRMSSyncRun struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenant_id"`
	AccountID    string         `json:"account_id"`
	SyncType     string         `json:"sync_type"`
	TriggerType  string         `json:"trigger_type"`
	Status       string         `json:"status"`
	CurrentStep  string         `json:"current_step,omitempty"`
	Mode         string         `json:"mode"`
	Since        string         `json:"since,omitempty"`
	Attempt      int            `json:"attempt"`
	MaxAttempts  int            `json:"max_attempts"`
	RetryOfRunID string         `json:"retry_of_run_id,omitempty"`
	RequestID    string         `json:"request_id,omitempty"`
	TraceID      string         `json:"trace_id,omitempty"`
	ErrorCode    string         `json:"error_code,omitempty"`
	ErrorMessage string         `json:"error_message,omitempty"`
	Retryable    bool           `json:"retryable"`
	NextRetryAt  *time.Time     `json:"next_retry_at,omitempty"`
	Summary      map[string]any `json:"summary,omitempty"`
	StartedAt    time.Time      `json:"started_at"`
	FinishedAt   *time.Time     `json:"finished_at,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// EHRMSSyncRunStep 保存 pipeline 每一步的結果。
type EHRMSSyncRunStep struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenant_id"`
	RunID        string         `json:"run_id"`
	Step         string         `json:"step"`
	Sequence     int            `json:"sequence"`
	Status       string         `json:"status"`
	Attempt      int            `json:"attempt"`
	ErrorCode    string         `json:"error_code,omitempty"`
	ErrorMessage string         `json:"error_message,omitempty"`
	Summary      map[string]any `json:"summary,omitempty"`
	StartedAt    time.Time      `json:"started_at"`
	FinishedAt   *time.Time     `json:"finished_at,omitempty"`
}

// EHRMSSyncRunDetail 聚合一次運行及其步驟。
type EHRMSSyncRunDetail struct {
	Run   EHRMSSyncRun       `json:"run"`
	Steps []EHRMSSyncRunStep `json:"steps"`
}

// RetryEHRMSSyncRunInput 定義人工重試參數。
type RetryEHRMSSyncRunInput struct {
	Mode  string `json:"mode,omitempty"`
	Since string `json:"since,omitempty"`
}

// StartEHRMSSyncInput 定義可追蹤的手動 pipeline 執行參數。
type StartEHRMSSyncInput struct {
	Mode              string `json:"mode,omitempty"`
	Since             string `json:"since,omitempty"`
	IncludeAttendance bool   `json:"include_attendance,omitempty"`
}
