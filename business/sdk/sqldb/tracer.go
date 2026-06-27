package sqldb

import (
	"context"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/i33ym/tetra/foundation/otel"
)

// Tracer implements pgx.QueryTracer to emit an otel span per query with the SQL
// statement attached. It degrades to a no-op when no tracer is in the context.
type Tracer struct{}

// NewTracer constructs a query tracer.
func NewTracer() *Tracer {
	return &Tracer{}
}

type spanCtxKey struct{}

// TraceQueryStart opens a span for the query.
func (t *Tracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	ctx, span := otel.AddSpan(ctx, "db.query", attribute.String("db.statement", data.SQL))
	return context.WithValue(ctx, spanCtxKey{}, span)
}

// TraceQueryEnd closes the span opened in TraceQueryStart.
func (t *Tracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	span, ok := ctx.Value(spanCtxKey{}).(trace.Span)
	if !ok {
		return
	}

	if data.Err != nil {
		span.RecordError(data.Err)
	}
	span.SetAttributes(attribute.String("db.rows_affected", data.CommandTag.String()))
	span.End()
}
