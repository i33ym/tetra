// The tetra binary runs the HTTP API service: it ingests payloads, stores
// metadata and bytes, and enqueues processing jobs. It can optionally run the
// worker in-process for local development.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/i33ym/tetra/api/services/tetra/build"
	"github.com/i33ym/tetra/app/sdk/debug"
	"github.com/i33ym/tetra/app/sdk/metrics"
	"github.com/i33ym/tetra/app/sdk/mux"
	"github.com/i33ym/tetra/app/sdk/worker"
	"github.com/i33ym/tetra/business/domain/jobbus"
	"github.com/i33ym/tetra/business/domain/jobbus/stores/jobdb"
	"github.com/i33ym/tetra/business/domain/payloadbus"
	"github.com/i33ym/tetra/business/domain/payloadbus/stores/payloaddb"
	"github.com/i33ym/tetra/business/sdk/delegate"
	"github.com/i33ym/tetra/business/sdk/processor"
	"github.com/i33ym/tetra/business/sdk/sqldb"
	"github.com/i33ym/tetra/foundation/blob"
	"github.com/i33ym/tetra/foundation/config"
	"github.com/i33ym/tetra/foundation/logger"
	"github.com/i33ym/tetra/foundation/otel"
)

// tag is set at build time via -ldflags '-X main.tag=...'.
var tag = "develop"

func main() {
	var log *logger.Logger

	events := logger.Events{
		Error: func(ctx context.Context, r logger.Record) {
			log.Info(ctx, "******* SEND ALERT *******")
		},
	}

	traceIDFn := func(ctx context.Context) string {
		return otel.GetTraceID(ctx)
	}

	log = logger.NewWithEvents(os.Stdout, logger.LevelInfo, "tetra", traceIDFn, events)

	ctx := context.Background()

	if err := run(ctx, log); err != nil {
		log.Error(ctx, "startup", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, log *logger.Logger) error {

	// -------------------------------------------------------------------------
	// Configuration

	cfg, err := config.Load(os.Getenv("TETRA_CONFIG"))
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	cfg.Otel.Version = tag

	log.BuildInfo(ctx)
	log.Info(ctx, "startup", "config", cfg.String())

	// -------------------------------------------------------------------------
	// Tracing

	traceProvider, traceTeardown, err := otel.InitTracing(otel.Config{
		ServiceName:    cfg.Otel.ServiceName,
		Version:        tag,
		Host:           cfg.Otel.Host,
		ExcludedRoutes: map[string]struct{}{"/v1/liveness": {}, "/v1/readiness": {}},
		Probability:    cfg.Otel.Probability,
	})
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}
	defer traceTeardown(context.Background())

	tracer := traceProvider.Tracer(cfg.Otel.ServiceName)

	// -------------------------------------------------------------------------
	// Database

	log.Info(ctx, "startup", "status", "initializing database", "host", cfg.DB.Host)

	pool, err := sqldb.Open(sqldb.Config{
		User:         cfg.DB.User,
		Password:     cfg.DB.Password,
		Host:         cfg.DB.Host,
		Name:         cfg.DB.Name,
		MaxOpenConns: cfg.DB.MaxOpenConns,
		MaxIdleConns: cfg.DB.MaxIdleConns,
		DisableTLS:   cfg.DB.DisableTLS,
		Tracer:       sqldb.NewTracer(),
	})
	if err != nil {
		return fmt.Errorf("opening db: %w", err)
	}
	defer pool.Close()

	metrics.RegisterPoolStats(pool)

	// -------------------------------------------------------------------------
	// Object storage

	blobStore, err := blob.Open(blob.Config{
		Endpoint:  cfg.MinIO.Endpoint,
		AccessKey: cfg.MinIO.AccessKey,
		SecretKey: cfg.MinIO.SecretKey,
		Bucket:    cfg.MinIO.Bucket,
		UseSSL:    cfg.MinIO.UseSSL,
		Region:    cfg.MinIO.Region,
	})
	if err != nil {
		return fmt.Errorf("opening blob store: %w", err)
	}

	// -------------------------------------------------------------------------
	// Business wiring

	dlg := delegate.New(log)
	payloadBus := payloadbus.NewBusiness(log, dlg, payloaddb.NewStore(log, pool))
	jobBus := jobbus.NewBusiness(log, jobdb.NewStore(log, pool))

	// Demonstration delegate handler: react to payload-created events.
	dlg.Register(payloadbus.DomainName, payloadbus.ActionCreated, func(ctx context.Context, data delegate.Data) error {
		log.Info(ctx, "delegate", "event", "payload.created", "params", string(data.RawParams))
		return nil
	})

	// -------------------------------------------------------------------------
	// Optional in-process worker (dev convenience)

	if cfg.Worker.InProc {
		log.Info(ctx, "startup", "status", "starting in-process worker")

		proc := processor.New(cfg.Processor.URL, cfg.Processor.Timeout.Duration)
		w := worker.New(worker.Config{
			Log:          log,
			Tracer:       tracer,
			PayloadBus:   payloadBus,
			JobBus:       jobBus,
			Processor:    proc,
			Concurrency:  cfg.Worker.Concurrency,
			PollInterval: cfg.Worker.PollInterval.Duration,
			LeaseSeconds: cfg.Worker.LeaseSeconds,
			BackoffBase:  cfg.Worker.BackoffBase.Duration,
		})

		workerCtx, cancelWorker := context.WithCancel(ctx)
		defer cancelWorker()
		go w.Run(workerCtx)
	}

	// -------------------------------------------------------------------------
	// Debug server (pprof, expvar, /metrics)

	go func() {
		log.Info(ctx, "startup", "status", "debug router started", "host", cfg.HTTP.DebugHost)
		if err := http.ListenAndServe(cfg.HTTP.DebugHost, debug.Mux()); err != nil {
			log.Error(ctx, "shutdown", "status", "debug router closed", "host", cfg.HTTP.DebugHost, "err", err)
		}
	}()

	// -------------------------------------------------------------------------
	// API server

	webAPI := mux.WebAPI(
		mux.Config{
			Build:          tag,
			Log:            log,
			DB:             pool,
			Tracer:         tracer,
			PayloadBus:     payloadBus,
			JobBus:         jobBus,
			Blob:           blobStore,
			MaxUploadBytes: cfg.HTTP.MaxUploadBytes,
			MaxAttempts:    cfg.Worker.MaxAttempts,
		},
		build.Routes(),
		mux.WithCORS(cfg.HTTP.CORSOrigins),
	)

	api := http.Server{
		Addr:              cfg.HTTP.APIHost,
		Handler:           webAPI,
		ReadTimeout:       cfg.HTTP.ReadTimeout.Duration,
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout.Duration,
		WriteTimeout:      cfg.HTTP.WriteTimeout.Duration,
		IdleTimeout:       cfg.HTTP.IdleTimeout.Duration,
		ErrorLog:          logger.NewStdLogger(log, logger.LevelError),
	}

	serverErrors := make(chan error, 1)

	go func() {
		log.Info(ctx, "startup", "status", "api router started", "host", api.Addr)
		serverErrors <- api.ListenAndServe()
	}()

	// -------------------------------------------------------------------------
	// Shutdown

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		return fmt.Errorf("server error: %w", err)

	case sig := <-shutdown:
		log.Info(ctx, "shutdown", "status", "shutdown started", "signal", sig)
		defer log.Info(ctx, "shutdown", "status", "shutdown complete", "signal", sig)

		ctx, cancel := context.WithTimeout(ctx, cfg.HTTP.ShutdownTimeout.Duration)
		defer cancel()

		if err := api.Shutdown(ctx); err != nil {
			api.Close()
			return fmt.Errorf("could not stop server gracefully: %w", err)
		}
	}

	return nil
}
