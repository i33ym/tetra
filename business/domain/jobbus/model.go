package jobbus

import (
	"time"

	"github.com/google/uuid"
)

// Set of job statuses.
const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
)

// Job represents a unit of asynchronous work tied to a payload.
type Job struct {
	ID          uuid.UUID
	PayloadID   uuid.UUID
	Status      string
	Attempts    int
	MaxAttempts int
	LastError   string
	RunAfter    time.Time
	DateCreated time.Time
	DateUpdated time.Time
}

// NewJob contains the information needed to enqueue a job.
type NewJob struct {
	PayloadID   uuid.UUID
	MaxAttempts int
}
