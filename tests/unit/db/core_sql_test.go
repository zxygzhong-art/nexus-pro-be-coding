package db_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	sqlc "nexus-pro-api/internal/platform/postgres/db"
)

type captureDBTX struct {
	queryRowSQL  string
	queryRowArgs []interface{}
	execSQL      string
	execArgs     []interface{}
	row          pgx.Row
}

// Exec 驗證 exec。
func (db *captureDBTX) Exec(_ context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	db.execSQL = sql
	db.execArgs = append([]interface{}{}, args...)
	return pgconn.CommandTag{}, nil
}

// Query 驗證查詢。
func (db *captureDBTX) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	return nil, nil
}

// QueryRow 驗證查詢列。
func (db *captureDBTX) QueryRow(_ context.Context, sql string, args ...interface{}) pgx.Row {
	db.queryRowSQL = sql
	db.queryRowArgs = append([]interface{}{}, args...)
	if db.row != nil {
		return db.row
	}
	return singleIntRow{value: 0}
}

type singleIntRow struct {
	value int32
}

// Scan 驗證 scan。
func (r singleIntRow) Scan(dest ...interface{}) error {
	*(dest[0].(*int32)) = r.value
	return nil
}

// TestNextEmployeeNoSequenceUsesSequenceTable 驗證 next 員工 no sequence uses sequence table。
func TestNextEmployeeNoSequenceUsesSequenceTable(t *testing.T) {
	dbtx := &captureDBTX{row: singleIntRow{value: 7}}
	got, err := sqlc.New(dbtx).NextEmployeeNoSequence(context.Background(), sqlc.NextEmployeeNoSequenceParams{
		TenantID:    "tenant-1",
		Prefix:      "IKL",
		InitialNext: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != 7 {
		t.Fatalf("unexpected next sequence: %d", got)
	}
	if len(dbtx.queryRowArgs) != 3 {
		t.Fatalf("expected 3 query args, got %d: %#v", len(dbtx.queryRowArgs), dbtx.queryRowArgs)
	}
	if dbtx.queryRowArgs[0] != "tenant-1" || dbtx.queryRowArgs[1] != "IKL" || dbtx.queryRowArgs[2] != int32(1) {
		t.Fatalf("expected tenantID, prefix, initial sequence query args, got %#v", dbtx.queryRowArgs)
	}
	if strings.Contains(dbtx.queryRowSQL, "FROM employees") {
		t.Fatalf("employee number sequence query should not scan employees: %s", dbtx.queryRowSQL)
	}
}

// TestInsertFormInstanceFieldValueBooleanBranchCasts 驗證 boolean 投影欄位帶 ::boolean cast。
// 2026-07-17 缺陷:缺少 cast 時 Postgres PREPARE 報 42804,CASE 分支無法確定型別,
// 導致所有含 analytics.reportable 布林欄位的表單提交 500。
func TestInsertFormInstanceFieldValueBooleanBranchCasts(t *testing.T) {
	dbtx := &captureDBTX{}
	err := sqlc.New(dbtx).InsertFormInstanceFieldValue(context.Background(), sqlc.InsertFormInstanceFieldValueParams{
		TenantID:       "tenant-1",
		FormInstanceID: "fi-1",
		TemplateID:     "ft-1",
		FieldID:        "overtime_confirmed",
		ValueType:      "boolean",
		ValueBoolean:   pgtype.Bool{Bool: true, Valid: true},
		CreatedAt:      pgtype.Timestamptz{Time: time.Unix(0, 0).UTC(), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dbtx.execSQL, "'boolean' THEN $9::boolean") {
		t.Fatalf("boolean CASE branch must cast the parameter with ::boolean, got SQL: %s", dbtx.execSQL)
	}
	if got, ok := dbtx.execArgs[8].(pgtype.Bool); !ok || !got.Valid || !got.Bool {
		t.Fatalf("expected boolean arg at position 9, got %#v", dbtx.execArgs[8])
	}
}
