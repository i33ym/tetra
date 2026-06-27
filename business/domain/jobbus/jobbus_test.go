package jobbus

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/i33ym/tetra/business/sdk/sqldb"
	"github.com/i33ym/tetra/foundation/logger"
)

type fakeStorer struct {
	requeued     []uuid.UUID
	requeueAfter time.Time
	buried       []uuid.UUID
}

func (f *fakeStorer) NewWithTx(_ sqldb.DBTX) (Storer, error)             { return f, nil }
func (f *fakeStorer) Create(_ context.Context, _ Job) error              { return nil }
func (f *fakeStorer) Dequeue(_ context.Context, _, _ int) ([]Job, error) { return nil, nil }
func (f *fakeStorer) Complete(_ context.Context, _ uuid.UUID) error      { return nil }
func (f *fakeStorer) Depth(_ context.Context) (map[string]int, error)    { return nil, nil }

func (f *fakeStorer) Requeue(_ context.Context, jobID uuid.UUID, runAfter time.Time, _ string) error {
	f.requeued = append(f.requeued, jobID)
	f.requeueAfter = runAfter
	return nil
}

func (f *fakeStorer) Bury(_ context.Context, jobID uuid.UUID, _ string) error {
	f.buried = append(f.buried, jobID)
	return nil
}

func discardLogger() *logger.Logger {
	return logger.New(io.Discard, logger.LevelError, "test", func(context.Context) string { return "" })
}

func TestFailRequeuesWithBackoff(t *testing.T) {
	fs := &fakeStorer{}
	b := NewBusiness(discardLogger(), fs)

	j := Job{ID: uuid.New(), Attempts: 1, MaxAttempts: 5}

	if err := b.Fail(context.Background(), j, time.Second, "boom"); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	if len(fs.requeued) != 1 {
		t.Fatalf("expected 1 requeue, got %d", len(fs.requeued))
	}
	if len(fs.buried) != 0 {
		t.Fatalf("expected 0 buries, got %d", len(fs.buried))
	}

	// backoff = base * 2^attempts = 1s * 2^1 = 2s.
	delta := time.Until(fs.requeueAfter)
	if delta < time.Second || delta > 4*time.Second {
		t.Errorf("run_after delta = %v, want ~2s", delta)
	}
}

func TestFailBuriesAtMaxAttempts(t *testing.T) {
	fs := &fakeStorer{}
	b := NewBusiness(discardLogger(), fs)

	j := Job{ID: uuid.New(), Attempts: 5, MaxAttempts: 5}

	if err := b.Fail(context.Background(), j, time.Second, "boom"); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	if len(fs.buried) != 1 {
		t.Fatalf("expected 1 bury, got %d", len(fs.buried))
	}
	if len(fs.requeued) != 0 {
		t.Fatalf("expected 0 requeues, got %d", len(fs.requeued))
	}
}
