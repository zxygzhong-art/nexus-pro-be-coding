package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"nexus-pro-api/internal/utils/tenantctx"
)

// tenantDBTX 定義租戶 dbtx 的資料結構。
type tenantDBTX struct {
	pool *pgxpool.Pool
}

// newTenantDBTX 建立租戶 dbtx。
func newTenantDBTX(pool *pgxpool.Pool) tenantDBTX {
	return tenantDBTX{pool: pool}
}

// Exec 處理 exec。
func (db tenantDBTX) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	scope := scopeFrom(ctx, args)
	if scope.isEmpty() {
		return db.pool.Exec(ctx, sql, args...)
	}
	tx, err := db.beginScoped(ctx, scope)
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	tag, err := tx.Exec(ctx, sql, args...)
	if err != nil {
		_ = tx.Rollback(ctx)
		return pgconn.CommandTag{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return pgconn.CommandTag{}, err
	}
	return tag, nil
}

// Query 處理查詢。
func (db tenantDBTX) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	scope := scopeFrom(ctx, args)
	if scope.isEmpty() {
		return db.pool.Query(ctx, sql, args...)
	}
	tx, err := db.beginScoped(ctx, scope)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	// transaction 會保持開啟，直到 caller 關閉或讀完 rows。
	return &tenantRows{Rows: rows, tx: tx, ctx: ctx}, nil
}

// QueryRow 處理查詢列。
func (db tenantDBTX) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	scope := scopeFrom(ctx, args)
	if scope.isEmpty() {
		return db.pool.QueryRow(ctx, sql, args...)
	}
	tx, err := db.beginScoped(ctx, scope)
	if err != nil {
		return errorRow{err: err}
	}
	return tenantRow{row: tx.QueryRow(ctx, sql, args...), tx: tx, ctx: ctx}
}

// tenantScope 定義租戶範圍的資料結構。
type tenantScope struct {
	tenantID   string
	companyID  string
	systemTask bool
}

// isEmpty 判斷是否為空值。
func (s tenantScope) isEmpty() bool {
	return s.tenantID == "" && !s.systemTask
}

// scopeFrom 處理範圍 from。
func scopeFrom(ctx context.Context, args []interface{}) tenantScope {
	return tenantScope{
		tenantID:   firstTenantID(ctx, args),
		companyID:  firstCompanyID(ctx, args),
		systemTask: tenantctx.SystemTaskFromContext(ctx),
	}
}

// firstTenantID 取得第一個租戶 ID。
func firstTenantID(ctx context.Context, args []interface{}) string {
	if tenantID := tenantctx.TenantIDFromContext(ctx); tenantID != "" {
		return tenantID
	}
	return tenantctx.TenantIDFromArgs(args)
}

// firstCompanyID 取得第一個公司 ID。
func firstCompanyID(ctx context.Context, args []interface{}) string {
	if companyID := tenantctx.CompanyIDFromContext(ctx); companyID != "" {
		return companyID
	}
	return tenantctx.CompanyIDFromArgs(args)
}

// beginScoped 處理 begin scoped。
func (db tenantDBTX) beginScoped(ctx context.Context, scope tenantScope) (pgx.Tx, error) {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	if scope.tenantID != "" {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", scope.tenantID); err != nil {
			_ = tx.Rollback(ctx)
			return nil, err
		}
	}
	if scope.companyID != "" {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.company_id', $1, true)", scope.companyID); err != nil {
			_ = tx.Rollback(ctx)
			return nil, err
		}
	}
	if scope.systemTask {
		// local=true 的 set_config 只在目前 transaction 內有效。
		// 這正好等於被包裹 sqlc call 的生命週期。
		if _, err := tx.Exec(ctx, "SELECT set_config('app.system_task', 'on', true)"); err != nil {
			_ = tx.Rollback(ctx)
			return nil, err
		}
	}
	return tx, nil
}

type tenantRows struct {
	pgx.Rows
	tx       pgx.Tx
	ctx      context.Context
	closed   bool
	closeErr error
}

// Next 處理 next。
func (r *tenantRows) Next() bool {
	ok := r.Rows.Next()
	if !ok {
		r.Close()
	}
	return ok
}

// Close 處理 close。
func (r *tenantRows) Close() {
	if r.closed {
		return
	}
	r.closed = true
	rowsErr := r.Rows.Err()
	r.Rows.Close()
	if rowsErr == nil {
		rowsErr = r.Rows.Err()
	}
	if rowsErr != nil {
		r.closeErr = rowsErr
		_ = r.tx.Rollback(r.ctx)
		return
	}
	if err := r.tx.Commit(r.ctx); err != nil {
		r.closeErr = err
	}
}

// Err 處理 err。
func (r *tenantRows) Err() error {
	if err := r.Rows.Err(); err != nil {
		return err
	}
	return r.closeErr
}

type tenantRow struct {
	row pgx.Row
	tx  pgx.Tx
	ctx context.Context
}

// Scan 處理 scan。
func (r tenantRow) Scan(dest ...any) error {
	if err := r.row.Scan(dest...); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if commitErr := r.tx.Commit(r.ctx); commitErr != nil {
				return commitErr
			}
			return err
		}
		_ = r.tx.Rollback(r.ctx)
		return err
	}
	return r.tx.Commit(r.ctx)
}

type errorRow struct {
	err error
}

// Scan 處理 scan。
func (r errorRow) Scan(dest ...any) error {
	return r.err
}
