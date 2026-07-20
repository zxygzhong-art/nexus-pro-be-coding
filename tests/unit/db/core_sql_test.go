package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	sqlc "nexus-pro-api/internal/platform/postgres/db"
)

type captureDBTX struct {
	queryRowSQL  string
	queryRowArgs []interface{}
	row          pgx.Row
}

// Exec 驗證 exec。
func (db *captureDBTX) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
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
