// Package payloadbus provides business access to the payload domain.
package payloadbus

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/i33ym/tetra/business/sdk/delegate"
	"github.com/i33ym/tetra/business/sdk/order"
	"github.com/i33ym/tetra/business/sdk/page"
	"github.com/i33ym/tetra/business/sdk/sqldb"
	"github.com/i33ym/tetra/business/types/status"
	"github.com/i33ym/tetra/foundation/logger"
	"github.com/i33ym/tetra/foundation/otel"
)

// Set of error variables for CRUD operations.
var (
	ErrNotFound = errors.New("payload not found")
)

// Storer interface declares the behavior this package needs to persist and
// retrieve payloads.
type Storer interface {
	NewWithTx(tx sqldb.DBTX) (Storer, error)
	Create(ctx context.Context, p Payload) error
	Update(ctx context.Context, p Payload) error
	QueryByID(ctx context.Context, payloadID uuid.UUID) (Payload, error)
	Query(ctx context.Context, filter QueryFilter, orderBy order.By, page page.Page) ([]Payload, error)
	Count(ctx context.Context, filter QueryFilter) (int, error)
}

// Business manages the set of APIs for payload access.
type Business struct {
	log      *logger.Logger
	storer   Storer
	delegate *delegate.Delegate
}

// NewBusiness constructs a payload business API for use.
func NewBusiness(log *logger.Logger, delegate *delegate.Delegate, storer Storer) *Business {
	return &Business{
		log:      log,
		storer:   storer,
		delegate: delegate,
	}
}

// NewWithTx constructs a new business value that will use the specified
// transaction in any store related calls.
func (b *Business) NewWithTx(tx sqldb.DBTX) (*Business, error) {
	storer, err := b.storer.NewWithTx(tx)
	if err != nil {
		return nil, err
	}

	return &Business{
		log:      b.log,
		storer:   storer,
		delegate: b.delegate,
	}, nil
}

// Create adds a new payload to the system.
func (b *Business) Create(ctx context.Context, np NewPayload) (Payload, error) {
	ctx, span := otel.AddSpan(ctx, "business.payloadbus.create")
	defer span.End()

	now := time.Now()

	p := Payload{
		ID:               uuid.New(),
		Kind:             np.Kind,
		Status:           status.Pending,
		BodyText:         np.BodyText,
		OriginalFilename: np.OriginalFilename,
		ContentType:      np.ContentType,
		ObjectKey:        np.ObjectKey,
		SizeBytes:        np.SizeBytes,
		DateCreated:      now,
		DateUpdated:      now,
	}

	if err := b.storer.Create(ctx, p); err != nil {
		return Payload{}, fmt.Errorf("create: %w", err)
	}

	// Notify other domains a payload was created (synchronous, in-transaction).
	if err := b.delegate.Call(ctx, ActionCreatedData(p.ID)); err != nil {
		return Payload{}, fmt.Errorf("delegate call: %w", err)
	}

	return p, nil
}

// UpdateResult records the outcome of processing a payload.
func (b *Business) UpdateResult(ctx context.Context, payloadID uuid.UUID, st status.Status, result string, errText string) (Payload, error) {
	ctx, span := otel.AddSpan(ctx, "business.payloadbus.updateresult")
	defer span.End()

	p, err := b.storer.QueryByID(ctx, payloadID)
	if err != nil {
		return Payload{}, fmt.Errorf("query: payloadID[%s]: %w", payloadID, err)
	}

	p.Status = st
	p.ResultText = result
	p.ErrorText = errText
	p.DateUpdated = time.Now()

	if err := b.storer.Update(ctx, p); err != nil {
		return Payload{}, fmt.Errorf("update: %w", err)
	}

	return p, nil
}

// SetStatus updates only the status of a payload (e.g. to processing).
func (b *Business) SetStatus(ctx context.Context, payloadID uuid.UUID, st status.Status) (Payload, error) {
	return b.UpdateResult(ctx, payloadID, st, "", "")
}

// QueryByID finds the payload by the specified ID.
func (b *Business) QueryByID(ctx context.Context, payloadID uuid.UUID) (Payload, error) {
	ctx, span := otel.AddSpan(ctx, "business.payloadbus.querybyid")
	defer span.End()

	p, err := b.storer.QueryByID(ctx, payloadID)
	if err != nil {
		return Payload{}, fmt.Errorf("query: payloadID[%s]: %w", payloadID, err)
	}

	return p, nil
}

// Query retrieves a list of existing payloads.
func (b *Business) Query(ctx context.Context, filter QueryFilter, orderBy order.By, page page.Page) ([]Payload, error) {
	ctx, span := otel.AddSpan(ctx, "business.payloadbus.query")
	defer span.End()

	ps, err := b.storer.Query(ctx, filter, orderBy, page)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}

	return ps, nil
}

// Count returns the total number of payloads matching the filter.
func (b *Business) Count(ctx context.Context, filter QueryFilter) (int, error) {
	ctx, span := otel.AddSpan(ctx, "business.payloadbus.count")
	defer span.End()

	return b.storer.Count(ctx, filter)
}
