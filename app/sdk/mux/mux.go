// Package mux provides support to bind domain level routes to the application
// mux and construct the web.App with the standard middleware chain.
package mux

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace"

	"github.com/i33ym/tetra/app/sdk/mid"
	"github.com/i33ym/tetra/business/domain/jobbus"
	"github.com/i33ym/tetra/business/domain/payloadbus"
	"github.com/i33ym/tetra/foundation/blob"
	"github.com/i33ym/tetra/foundation/logger"
	"github.com/i33ym/tetra/foundation/web"
)

// Config contains all the mandatory systems required by handlers.
type Config struct {
	Build          string
	Log            *logger.Logger
	DB             *pgxpool.Pool
	Tracer         trace.Tracer
	PayloadBus     *payloadbus.Business
	JobBus         *jobbus.Business
	Blob           *blob.Store
	MaxUploadBytes int64
	MaxAttempts    int
}

// RouteAdder defines behavior that sets the routes to bind for an instance.
type RouteAdder interface {
	Add(app *web.App, cfg Config)
}

// Options represent optional parameters.
type Options struct {
	corsOrigins []string
}

// WithCORS provides configuration options for CORS.
func WithCORS(origins []string) func(opts *Options) {
	return func(opts *Options) {
		opts.corsOrigins = origins
	}
}

// WebAPI constructs a http.Handler with all application routes bound. The
// app-wide middleware chain runs outermost-first: Otel, Logger, Errors,
// Metrics, Panics.
func WebAPI(cfg Config, routeAdder RouteAdder, options ...func(opts *Options)) http.Handler {
	logFunc := func(ctx context.Context, msg string, args ...any) {
		cfg.Log.Info(ctx, msg, args...)
	}

	app := web.NewApp(
		logFunc,
		cfg.Tracer,
		mid.Otel(cfg.Tracer),
		mid.Logger(cfg.Log),
		mid.Errors(cfg.Log),
		mid.Metrics(),
		mid.Panics(),
	)

	var opts Options
	for _, option := range options {
		option(&opts)
	}

	if len(opts.corsOrigins) > 0 {
		app.EnableCORS(opts.corsOrigins)
	}

	routeAdder.Add(app, cfg)

	return app
}
