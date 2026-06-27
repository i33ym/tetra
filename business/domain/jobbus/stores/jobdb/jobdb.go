// Package jobdb provides the Postgres (pgx) implementation of the job queue
// Storer interface, using FOR UPDATE SKIP LOCKED for safe concurrent dequeue.
package jobdb

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/i33ym/tetra/business/domain/jobbus"
	"github.com/i33ym/tetra/business/sdk/sqldb"
	"github.com/i33ym/tetra/foundation/logger"
)

// Store manages the set of APIs for job database access.
type Store struct {
	log *logger.Logger
	db  sqldb.DBTX
}

// NewStore constructs the api for data access.
func NewStore(log *logger.Logger, db sqldb.DBTX) *Store {
	return &Store{
		log: log,
		db:  db,
	}
}

// NewWithTx constructs a new Store value that uses the provided transaction.
func (s *Store) NewWithTx(tx sqldb.DBTX) (jobbus.Storer, error) {
	return &Store{
		log: s.log,
		db:  tx,
	}, nil
}

// Create inserts a new job into the queue.
func (s *Store) Create(ctx context.Context, j jobbus.Job) error {
	const q = `
	INSERT INTO jobs
		(job_id, payload_id, status, attempts, max_attempts, last_error, run_after, date_created, date_updated)
	VALUES
		($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	if _, err := s.db.Exec(ctx, q,
		j.ID, j.PayloadID, j.Status, j.Attempts, j.MaxAttempts, j.LastError,
		j.RunAfter, j.DateCreated, j.DateUpdated); err != nil {
		return sqldb.TranslateError(err)
	}

	return nil
}

// Dequeue atomically claims up to limit ready jobs. The CTE selects claimable
// rows with FOR UPDATE SKIP LOCKED so concurrent workers never block each other
// or claim the same row, then the UPDATE marks them running with a lease.
func (s *Store) Dequeue(ctx context.Context, limit int, leaseSeconds int) ([]jobbus.Job, error) {
	const q = `
	WITH claimed AS (
		SELECT job_id
		FROM jobs
		WHERE status IN ('queued','running')
		  AND run_after <= now()
		  AND (locked_until IS NULL OR locked_until < now())
		ORDER BY run_after
		FOR UPDATE SKIP LOCKED
		LIMIT $1
	)
	UPDATE jobs j
	SET status       = 'running',
		attempts     = j.attempts + 1,
		locked_at    = now(),
		locked_until = now() + ($2 * interval '1 second'),
		date_updated = now()
	FROM claimed c
	WHERE j.job_id = c.job_id
	RETURNING j.job_id, j.payload_id, j.attempts, j.max_attempts`

	rows, err := s.db.Query(ctx, q, limit, leaseSeconds)
	if err != nil {
		return nil, sqldb.TranslateError(err)
	}
	defer rows.Close()

	var jobs []jobbus.Job
	for rows.Next() {
		var j jobbus.Job
		if err := rows.Scan(&j.ID, &j.PayloadID, &j.Attempts, &j.MaxAttempts); err != nil {
			return nil, err
		}
		j.Status = jobbus.StatusRunning
		jobs = append(jobs, j)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return jobs, nil
}

// Complete marks a job as done and clears its lease.
func (s *Store) Complete(ctx context.Context, jobID uuid.UUID) error {
	const q = `UPDATE jobs SET status = 'done', locked_until = NULL, date_updated = now() WHERE job_id = $1`

	if _, err := s.db.Exec(ctx, q, jobID); err != nil {
		return sqldb.TranslateError(err)
	}

	return nil
}

// Requeue returns a job to the queue with a future run_after (backoff).
func (s *Store) Requeue(ctx context.Context, jobID uuid.UUID, runAfter time.Time, lastErr string) error {
	const q = `
	UPDATE jobs
	SET status = 'queued', run_after = $2, last_error = $3, locked_until = NULL, date_updated = now()
	WHERE job_id = $1`

	if _, err := s.db.Exec(ctx, q, jobID, runAfter, lastErr); err != nil {
		return sqldb.TranslateError(err)
	}

	return nil
}

// Bury marks a job permanently failed after exhausting its attempts.
func (s *Store) Bury(ctx context.Context, jobID uuid.UUID, lastErr string) error {
	const q = `UPDATE jobs SET status = 'failed', last_error = $2, locked_until = NULL, date_updated = now() WHERE job_id = $1`

	if _, err := s.db.Exec(ctx, q, jobID, lastErr); err != nil {
		return sqldb.TranslateError(err)
	}

	return nil
}

// Depth returns the count of jobs grouped by status.
func (s *Store) Depth(ctx context.Context) (map[string]int, error) {
	const q = `SELECT status, count(*) FROM jobs GROUP BY status`

	rows, err := s.db.Query(ctx, q)
	if err != nil {
		return nil, sqldb.TranslateError(err)
	}
	defer rows.Close()

	depth := make(map[string]int)
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		depth[st] = n
	}

	return depth, rows.Err()
}
