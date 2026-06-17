package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"nexus-pro-be/internal/utils/tenantctx"
)

type tenantDBTX struct {
	pool *pgxpool.Pool
}

func newTenantDBTX(pool *pgxpool.Pool) tenantDBTX {
	return tenantDBTX{pool: pool}
}

func (db tenantDBTX) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	tenantID := firstTenantID(ctx, args)
	if tenantID == "" {
		return db.pool.Exec(ctx, sql, args...)
	}
	tx, err := db.beginTenant(ctx, tenantID)
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

func (db tenantDBTX) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	tenantID := firstTenantID(ctx, args)
	if tenantID == "" {
		return db.pool.Query(ctx, sql, args...)
	}
	tx, err := db.beginTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return &tenantRows{Rows: rows, tx: tx, ctx: ctx}, nil
}

func (db tenantDBTX) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	tenantID := firstTenantID(ctx, args)
	if tenantID == "" {
		return db.pool.QueryRow(ctx, sql, args...)
	}
	tx, err := db.beginTenant(ctx, tenantID)
	if err != nil {
		return errorRow{err: err}
	}
	return tenantRow{row: tx.QueryRow(ctx, sql, args...), tx: tx, ctx: ctx}
}

func firstTenantID(ctx context.Context, args []interface{}) string {
	if tenantID := tenantctx.TenantIDFromContext(ctx); tenantID != "" {
		return tenantID
	}
	return tenantctx.TenantIDFromArgs(args)
}

func (db tenantDBTX) beginTenant(ctx context.Context, tenantID string) (pgx.Tx, error) {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
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

func (r *tenantRows) Next() bool {
	ok := r.Rows.Next()
	if !ok {
		r.Close()
	}
	return ok
}

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

func (r tenantRow) Scan(dest ...any) error {
	if err := r.row.Scan(dest...); err != nil {
		_ = r.tx.Rollback(r.ctx)
		return err
	}
	return r.tx.Commit(r.ctx)
}

type errorRow struct {
	err error
}

func (r errorRow) Scan(dest ...any) error {
	return r.err
}
