package postgres

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const maxTraceSQLLength = 500

type queryTracer struct {
	tracer trace.Tracer
}

func newQueryTracer() pgx.QueryTracer {
	return queryTracer{tracer: otel.Tracer("nexus-pro-be/internal/platform/postgres")}
}

func (t queryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if t.tracer == nil {
		t.tracer = otel.Tracer("nexus-pro-be/internal/platform/postgres")
	}
	operation := sqlOperation(data.SQL)
	ctx, _ = t.tracer.Start(ctx, "postgres."+operation,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system.name", "postgresql"),
			attribute.String("db.operation.name", operation),
			attribute.String("db.query.text", traceSQL(data.SQL)),
			attribute.Int("db.query.parameter_count", len(data.Args)),
		),
	)
	return ctx
}

func (queryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	span := trace.SpanFromContext(ctx)
	defer span.End()

	if tag := data.CommandTag.String(); tag != "" {
		span.SetAttributes(
			attribute.String("db.response.status_code", tag),
			attribute.Int64("db.response.rows_affected", data.CommandTag.RowsAffected()),
		)
	}
	if data.Err != nil {
		span.RecordError(data.Err)
		span.SetStatus(codes.Error, data.Err.Error())
	}
}

func sqlOperation(sql string) string {
	sql = stripLeadingLineComments(sql)
	fields := strings.Fields(sql)
	if len(fields) == 0 {
		return "query"
	}
	switch op := strings.ToLower(fields[0]); op {
	case "select", "insert", "update", "delete", "with", "begin", "commit", "rollback":
		return op
	default:
		return "query"
	}
}

func stripLeadingLineComments(sql string) string {
	sql = strings.TrimSpace(sql)
	for strings.HasPrefix(sql, "--") {
		newline := strings.IndexByte(sql, '\n')
		if newline < 0 {
			return ""
		}
		sql = strings.TrimSpace(sql[newline+1:])
	}
	return sql
}

func traceSQL(sql string) string {
	sql = strings.Join(strings.Fields(sql), " ")
	if len(sql) <= maxTraceSQLLength {
		return sql
	}
	return sql[:maxTraceSQLLength]
}
