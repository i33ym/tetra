// The worker binary runs the background processing loop only: it dequeues jobs
// from the Postgres queue and calls the external processor. It scales
// independently of the API on queue-depth/CPU.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/i33ym/tetra/app/sdk/debug"
	"github.com/i33ym/tetra/app/sdk/metrics"
	"github.com/i33ym/tetra/app/sdk/worker"
	"github.com/i33ym/tetra/business/domain/jobbus"
	"github.com/i33ym/tetra/business/domain/jobbus/stores/jobdb"
	"github.com/i33ym/tetra/business/domain/payloadbus"
	"github.com/i33ym/tetra/business/domain/payloadbus/stores/payloaddb"
	"github.com/i33ym/tetra/business/sdk/delegate"
	"github.com/i33ym/tetra/business/sdk/processor"
	"github.com/i33ym/tetra/business/sdk/sqldb"
	"github.com/i33ym/tetra/foundation/config"
	"github.com/i33ym/tetra/foundation/logger"
	"github.com/i33ym/tetra/foundation/otel"
)

const serviceName = "tetra-worker"

// tag is set at build time via -ldflags '-X main.tag=...'.
var tag = "develop"

func main() {
	var log *logger.Logger

	events := logger.Events{
		Error: func(ctx context.Context, r logger.Record) {
			log.Info(ctx, "******* SEND ALERT *******")
		},
	}

	log = logger.NewWithEvents(os.Stdout, logger.LevelInfo, serviceName, otel.GetTraceID, events)

	ctx := context.Background()

	if err := run(ctx, log); err != nil {
		log.Error(ctx, "startup", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, log *logger.Logger) error {
	cfg, err := config.Load(os.Getenv("TETRA_CONFIG"))
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	log.BuildInfo(ctx)
	log.Info(ctx, "startup", "config", cfg.String())

	// -------------------------------------------------------------------------
	// Tracing

	traceProvider, traceTeardown, err := otel.InitTracing(otel.Config{
		ServiceName: serviceName,
		Version:     tag,
		Host:        cfg.Otel.Host,
		Probability: cfg.Otel.Probability,
	})
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}
	defer traceTeardown(context.Background())

	tracer := traceProvider.Tracer(serviceName)

	// -------------------------------------------------------------------------
	// Database

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
	// Business wiring

	dlg := delegate.New(log)
	payloadBus := payloadbus.NewBusiness(log, dlg, payloaddb.NewStore(log, pool))
	jobBus := jobbus.NewBusiness(log, jobdb.NewStore(log, pool))
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

	// -------------------------------------------------------------------------
	// Debug + health servers

	go func() {
		log.Info(ctx, "startup", "status", "debug router started", "host", cfg.HTTP.DebugHost)
		if err := http.ListenAndServe(cfg.HTTP.DebugHost, debug.Mux()); err != nil {
			log.Error(ctx, "shutdown", "status", "debug router closed", "err", err)
		}
	}()

	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/v1/liveness", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "up"})
	})
	healthMux.HandleFunc("/v1/readiness", func(w http.ResponseWriter, r *http.Request) {
		cx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := sqldb.StatusCheck(cx, pool); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not ready"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	healthSrv := http.Server{
		Addr:              cfg.HTTP.APIHost,
		Handler:           healthMux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Info(ctx, "startup", "status", "health router started", "host", healthSrv.Addr)
		if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error(ctx, "shutdown", "status", "health router closed", "err", err)
		}
	}()

	// -------------------------------------------------------------------------
	// Run worker

	workerCtx, cancelWorker := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	go func() {
		w.Run(workerCtx)
		close(workerDone)
	}()

	// -------------------------------------------------------------------------
	// Shutdown

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	sig := <-shutdown
	log.Info(ctx, "shutdown", "status", "shutdown started", "signal", sig)
	defer log.Info(ctx, "shutdown", "status", "shutdown complete", "signal", sig)

	cancelWorker()

	select {
	case <-workerDone:
	case <-time.After(cfg.HTTP.ShutdownTimeout.Duration):
		log.Info(ctx, "shutdown", "status", "worker drain timed out")
	}

	shCtx, cancel := context.WithTimeout(ctx, cfg.HTTP.ShutdownTimeout.Duration)
	defer cancel()
	_ = healthSrv.Shutdown(shCtx)

	return nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
