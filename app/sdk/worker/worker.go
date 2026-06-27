// Package worker runs the background processing loop: it dequeues jobs from the
// Postgres queue, calls the external processor, and records the outcome. It is
// used both by the standalone worker service and the in-process dev mode.
package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/i33ym/tetra/app/sdk/metrics"
	"github.com/i33ym/tetra/business/domain/jobbus"
	"github.com/i33ym/tetra/business/domain/payloadbus"
	"github.com/i33ym/tetra/business/sdk/processor"
	"github.com/i33ym/tetra/business/types/status"
	"github.com/i33ym/tetra/foundation/logger"
	"github.com/i33ym/tetra/foundation/otel"
)

// Config holds the worker dependencies and tuning knobs.
type Config struct {
	Log          *logger.Logger
	Tracer       trace.Tracer
	PayloadBus   *payloadbus.Business
	JobBus       *jobbus.Business
	Processor    *processor.Client
	Concurrency  int
	PollInterval time.Duration
	LeaseSeconds int
	BackoffBase  time.Duration
}

// Worker dequeues and processes jobs.
type Worker struct {
	cfg Config
}

// New constructs a worker.
func New(cfg Config) *Worker {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 1
	}
	return &Worker{cfg: cfg}
}

// Run starts the dispatcher loop and blocks until ctx is canceled, draining
// in-flight jobs before returning.
func (w *Worker) Run(ctx context.Context) {
	log := w.cfg.Log

	sem := make(chan struct{}, w.cfg.Concurrency)
	var wg sync.WaitGroup

	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	log.Info(ctx, "worker", "status", "started", "concurrency", w.cfg.Concurrency)

	for {
		select {
		case <-ctx.Done():
			log.Info(ctx, "worker", "status", "draining in-flight jobs")
			wg.Wait()
			log.Info(ctx, "worker", "status", "stopped")
			return

		case <-ticker.C:
			free := w.cfg.Concurrency - len(sem)
			if free <= 0 {
				continue
			}

			jobs, err := w.cfg.JobBus.Dequeue(ctx, free, w.cfg.LeaseSeconds)
			if err != nil {
				if ctx.Err() == nil {
					log.Error(ctx, "worker", "msg", "dequeue", "err", err)
				}
				continue
			}

			w.refreshDepth(ctx)

			for _, job := range jobs {
				wg.Add(1)
				sem <- struct{}{}

				go func(job jobbus.Job) {
					defer wg.Done()
					defer func() { <-sem }()

					// Detach from cancellation so an in-flight job can drain on
					// shutdown; the processor client enforces its own timeout.
					w.process(context.WithoutCancel(ctx), job)
				}(job)
			}
		}
	}
}

func (w *Worker) refreshDepth(ctx context.Context) {
	depth, err := w.cfg.JobBus.Depth(ctx)
	if err != nil {
		return
	}

	for _, st := range []string{jobbus.StatusQueued, jobbus.StatusRunning, jobbus.StatusDone, jobbus.StatusFailed} {
		metrics.SetQueueDepth(st, float64(depth[st]))
	}
}

func (w *Worker) process(ctx context.Context, job jobbus.Job) {
	log := w.cfg.Log

	ctx = otel.InjectTracing(ctx, w.cfg.Tracer)
	ctx, span := otel.AddSpan(ctx, "worker.process")
	defer span.End()

	metrics.IncInflight()
	defer metrics.DecInflight()
	start := time.Now()

	p, err := w.cfg.PayloadBus.QueryByID(ctx, job.PayloadID)
	if err != nil {
		w.fail(ctx, job, fmt.Sprintf("load payload: %s", err))
		metrics.ObserveJob("failed", time.Since(start))
		return
	}

	if _, err := w.cfg.PayloadBus.SetStatus(ctx, p.ID, status.Processing); err != nil {
		log.Error(ctx, "worker", "msg", "set processing", "err", err)
	}

	req := processor.Request{
		PayloadID:   p.ID.String(),
		Kind:        p.Kind,
		Text:        p.BodyText,
		ObjectKey:   p.ObjectKey,
		ContentType: p.ContentType,
		SizeBytes:   p.SizeBytes,
	}

	resp, err := w.cfg.Processor.Process(ctx, req)
	if err != nil {
		log.Error(ctx, "worker", "msg", "process", "jobID", job.ID, "attempts", job.Attempts, "err", err)
		w.fail(ctx, job, err.Error())
		metrics.ObserveJob("failed", time.Since(start))
		return
	}

	if _, err := w.cfg.PayloadBus.UpdateResult(ctx, p.ID, status.Done, resp.Result, ""); err != nil {
		log.Error(ctx, "worker", "msg", "update result", "err", err)
	}

	if err := w.cfg.JobBus.Complete(ctx, job.ID); err != nil {
		log.Error(ctx, "worker", "msg", "complete", "err", err)
	}

	metrics.ObserveJob("done", time.Since(start))
	log.Info(ctx, "worker", "status", "processed", "jobID", job.ID, "payloadID", p.ID)
}

// fail records the failure. If this was the last attempt the payload is marked
// failed; the job itself is requeued with backoff or buried by jobbus.Fail.
func (w *Worker) fail(ctx context.Context, job jobbus.Job, reason string) {
	if job.Attempts >= job.MaxAttempts {
		if _, err := w.cfg.PayloadBus.UpdateResult(ctx, job.PayloadID, status.Failed, "", reason); err != nil {
			w.cfg.Log.Error(ctx, "worker", "msg", "mark payload failed", "err", err)
		}
	}

	if err := w.cfg.JobBus.Fail(ctx, job, w.cfg.BackoffBase, reason); err != nil {
		w.cfg.Log.Error(ctx, "worker", "msg", "fail job", "err", err)
	}
}
