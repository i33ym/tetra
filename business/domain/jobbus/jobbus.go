// Package jobbus provides a durable Postgres-backed job queue. Multiple worker
// replicas claim disjoint jobs concurrently via SELECT ... FOR UPDATE SKIP
// LOCKED, giving at-least-once processing with lease-based crash recovery.
package jobbus

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/i33ym/tetra/business/sdk/sqldb"
	"github.com/i33ym/tetra/foundation/logger"
	"github.com/i33ym/tetra/foundation/otel"
)

// maxBackoff caps the exponential backoff between retry attempts.
const maxBackoff = 5 * time.Minute

// Storer interface declares the behavior this package needs to persist and
// retrieve jobs.
type Storer interface {
	NewWithTx(tx sqldb.DBTX) (Storer, error)
	Create(ctx context.Context, j Job) error
	Dequeue(ctx context.Context, limit int, leaseSeconds int) ([]Job, error)
	Complete(ctx context.Context, jobID uuid.UUID) error
	Requeue(ctx context.Context, jobID uuid.UUID, runAfter time.Time, lastErr string) error
	Bury(ctx context.Context, jobID uuid.UUID, lastErr string) error
	Depth(ctx context.Context) (map[string]int, error)
}

// Business manages the set of APIs for job queue access.
type Business struct {
	log    *logger.Logger
	storer Storer
}

// NewBusiness constructs a job business API for use.
func NewBusiness(log *logger.Logger, storer Storer) *Business {
	return &Business{
		log:    log,
		storer: storer,
	}
}

// NewWithTx constructs a new business value that uses the specified transaction.
func (b *Business) NewWithTx(tx sqldb.DBTX) (*Business, error) {
	storer, err := b.storer.NewWithTx(tx)
	if err != nil {
		return nil, err
	}

	return &Business{
		log:    b.log,
		storer: storer,
	}, nil
}

// Enqueue adds a new job to the queue.
func (b *Business) Enqueue(ctx context.Context, nj NewJob) (Job, error) {
	ctx, span := otel.AddSpan(ctx, "business.jobbus.enqueue")
	defer span.End()

	now := time.Now()

	j := Job{
		ID:          uuid.New(),
		PayloadID:   nj.PayloadID,
		Status:      StatusQueued,
		Attempts:    0,
		MaxAttempts: nj.MaxAttempts,
		RunAfter:    now,
		DateCreated: now,
		DateUpdated: now,
	}

	if err := b.storer.Create(ctx, j); err != nil {
		return Job{}, fmt.Errorf("create: %w", err)
	}

	return j, nil
}

// Dequeue atomically claims up to limit ready jobs, marking them running with a
// lease of leaseSeconds.
func (b *Business) Dequeue(ctx context.Context, limit int, leaseSeconds int) ([]Job, error) {
	ctx, span := otel.AddSpan(ctx, "business.jobbus.dequeue")
	defer span.End()

	jobs, err := b.storer.Dequeue(ctx, limit, leaseSeconds)
	if err != nil {
		return nil, fmt.Errorf("dequeue: %w", err)
	}

	return jobs, nil
}

// Complete marks a job as successfully done.
func (b *Business) Complete(ctx context.Context, jobID uuid.UUID) error {
	ctx, span := otel.AddSpan(ctx, "business.jobbus.complete")
	defer span.End()

	return b.storer.Complete(ctx, jobID)
}

// Fail either requeues the job with exponential backoff or buries it after the
// maximum number of attempts is exhausted.
func (b *Business) Fail(ctx context.Context, j Job, backoffBase time.Duration, lastErr string) error {
	ctx, span := otel.AddSpan(ctx, "business.jobbus.fail")
	defer span.End()

	if j.Attempts >= j.MaxAttempts {
		return b.storer.Bury(ctx, j.ID, lastErr)
	}

	backoff := backoffBase * time.Duration(1<<uint(j.Attempts))
	if backoff > maxBackoff {
		backoff = maxBackoff
	}
	runAfter := time.Now().Add(backoff)

	return b.storer.Requeue(ctx, j.ID, runAfter, lastErr)
}

// Depth returns the number of jobs grouped by status.
func (b *Business) Depth(ctx context.Context) (map[string]int, error) {
	ctx, span := otel.AddSpan(ctx, "business.jobbus.depth")
	defer span.End()

	return b.storer.Depth(ctx)
}
