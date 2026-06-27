// The mockproc binary is a stand-in for the real external payload processor. It
// sleeps a jittered latency, randomly fails, and echoes a fake result. It is
// fully otel-instrumented so the worker -> processor call shows as one trace.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/i33ym/tetra/foundation/logger"
	"github.com/i33ym/tetra/foundation/otel"
)

const serviceName = "mockproc"

func main() {
	log := logger.New(os.Stdout, logger.LevelInfo, serviceName, otel.GetTraceID)

	ctx := context.Background()
	if err := run(ctx, log); err != nil {
		log.Error(ctx, "startup", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, log *logger.Logger) error {
	host := envStr("MOCKPROC_HOST", "0.0.0.0:7000")
	otelHost := os.Getenv("TETRA_OTEL_HOST")
	failureRate := envFloat("MOCKPROC_FAILURE_RATE", 0.1)
	minLatency := envDuration("MOCKPROC_MIN_LATENCY", 200*time.Millisecond)
	maxLatency := envDuration("MOCKPROC_MAX_LATENCY", 1500*time.Millisecond)

	traceProvider, traceTeardown, err := otel.InitTracing(otel.Config{
		ServiceName: serviceName,
		Version:     "develop",
		Host:        otelHost,
		Probability: 1.0,
	})
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}
	defer traceTeardown(context.Background())
	_ = traceProvider

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/liveness", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"up"}`))
	})
	mux.HandleFunc("/process", processHandler(log, failureRate, minLatency, maxLatency))

	// otelhttp extracts the incoming W3C trace context so this server's span is a
	// child of the worker's client span.
	handler := otelhttp.NewHandler(mux, "mockproc")

	srv := http.Server{
		Addr:              host,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Info(ctx, "startup", "status", "mockproc started", "host", host, "failureRate", failureRate)
		serverErrors <- srv.ListenAndServe()
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		return fmt.Errorf("server error: %w", err)

	case sig := <-shutdown:
		log.Info(ctx, "shutdown", "status", "shutdown started", "signal", sig)
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			srv.Close()
			return fmt.Errorf("could not stop server gracefully: %w", err)
		}
	}

	return nil
}

type processRequest struct {
	PayloadID string `json:"payload_id"`
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	ObjectKey string `json:"object_key"`
	SizeBytes int64  `json:"size_bytes"`
}

func processHandler(log *logger.Logger, failureRate float64, minLatency, maxLatency time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		var req processRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		// Simulate processing time.
		delay := minLatency
		if maxLatency > minLatency {
			delay += time.Duration(rand.Int63n(int64(maxLatency - minLatency)))
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			http.Error(w, "canceled", http.StatusRequestTimeout)
			return
		}

		// Randomly fail to exercise the retry/backoff path.
		if rand.Float64() < failureRate {
			log.Info(ctx, "process", "payloadID", req.PayloadID, "outcome", "simulated_failure")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"simulated processing failure"}`))
			return
		}

		var result string
		switch req.Kind {
		case "text":
			result = fmt.Sprintf("processed text payload %s (%d chars)", req.PayloadID, len(req.Text))
		default:
			result = fmt.Sprintf("processed file payload %s (%d bytes)", req.PayloadID, req.SizeBytes)
		}

		log.Info(ctx, "process", "payloadID", req.PayloadID, "outcome", "ok")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"result": result})
	}
}

func envStr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v, ok := os.LookupEnv(key); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
