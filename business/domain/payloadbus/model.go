package payloadbus

import (
	"time"

	"github.com/google/uuid"

	"github.com/i33ym/tetra/business/types/status"
)

// Set of payload kinds.
const (
	KindText = "text"
	KindFile = "file"
)

// Payload represents an ingested payload and its processing state.
type Payload struct {
	ID               uuid.UUID
	Kind             string
	Status           status.Status
	BodyText         string
	OriginalFilename string
	ContentType      string
	ObjectKey        string
	SizeBytes        int64
	ResultText       string
	ErrorText        string
	DateCreated      time.Time
	DateUpdated      time.Time
}

// NewPayload contains the information needed to create a new payload. The bytes
// (if any) have already been written to object storage by the time this is
// constructed, so it carries only metadata.
type NewPayload struct {
	Kind             string
	BodyText         string
	OriginalFilename string
	ContentType      string
	ObjectKey        string
	SizeBytes        int64
}
