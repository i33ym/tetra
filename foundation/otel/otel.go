// Package otel provides otel support for tracing.
package otel

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const defaultTraceID = "00000000000000000000000000000000"

// Config defines the information needed to init tracing.
type Config struct {
	ServiceName    string
	Version        string
	Host           string
	ExcludedRoutes map[string]struct{}
	Probability    float64
}

// InitTracing configures open telemetry to be used with the service. When the
// host is empty a no-op provider is installed so the service runs without a
// collector.
func InitTracing(cfg Config) (trace.TracerProvider, func(ctx context.Context), error) {
	var traceProvider trace.TracerProvider
	teardown := func(ctx context.Context) {}

	switch cfg.Host {
	case "":
		traceProvider = noop.NewTracerProvider()

	default:
		exporter, err := otlptrace.New(
			context.Background(),
			otlptracegrpc.NewClient(
				otlptracegrpc.WithInsecure(),
				otlptracegrpc.WithEndpoint(cfg.Host),
			),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("creating new exporter: %w", err)
		}

		res := resource.NewSchemaless(
			attribute.String("service.name", cfg.ServiceName),
			attribute.String("service.version", cfg.Version),
		)

		tp := sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sdktrace.ParentBased(newEndpointExcluder(cfg.ExcludedRoutes, cfg.Probability))),
			sdktrace.WithBatcher(exporter),
			sdktrace.WithResource(res),
		)

		teardown = func(ctx context.Context) {
			tp.Shutdown(ctx)
		}

		traceProvider = tp
	}

	// Set the global provider so instrumentation libraries (otelhttp) use it,
	// while we also pass it explicitly where needed.
	otel.SetTracerProvider(traceProvider)

	// Extract incoming trace contexts and inject into outgoing requests.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return traceProvider, teardown, nil
}

// InjectTracing initializes the request for tracing by saving the tracer and
// trace id in the context for later use.
func InjectTracing(ctx context.Context, tracer trace.Tracer) context.Context {
	ctx = setTracer(ctx, tracer)

	traceID := trace.SpanFromContext(ctx).SpanContext().TraceID().String()
	if traceID == defaultTraceID {
		traceID = uuid.NewString()
	}
	ctx = setTraceID(ctx, traceID)

	return ctx
}

// AddSpan adds an otel span to the existing trace.
func AddSpan(ctx context.Context, spanName string, keyValues ...attribute.KeyValue) (context.Context, trace.Span) {
	tracer, ok := ctx.Value(tracerKey).(trace.Tracer)
	if !ok || tracer == nil {
		return ctx, trace.SpanFromContext(ctx)
	}

	ctx, span := tracer.Start(ctx, spanName)
	span.SetAttributes(keyValues...)

	return ctx, span
}

// AddTraceToRequest adds the current trace context to the request headers so it
// can be delivered to the service being called.
func AddTraceToRequest(ctx context.Context, r *http.Request) {
	hc := propagation.HeaderCarrier(r.Header)
	otel.GetTextMapPropagator().Inject(ctx, hc)
}
