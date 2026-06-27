// Package payloaddb provides the Postgres (pgx) implementation of the payload
// Storer interface.
package payloaddb

import (
	"bytes"
	"context"
	"errors"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/i33ym/tetra/business/domain/payloadbus"
	"github.com/i33ym/tetra/business/sdk/order"
	"github.com/i33ym/tetra/business/sdk/page"
	"github.com/i33ym/tetra/business/sdk/sqldb"
	"github.com/i33ym/tetra/foundation/logger"
)

const columns = `payload_id, kind, status, body_text, original_filename, content_type, object_key, size_bytes, result_text, error_text, date_created, date_updated`

// Store manages the set of APIs for payload database access.
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
func (s *Store) NewWithTx(tx sqldb.DBTX) (payloadbus.Storer, error) {
	return &Store{
		log: s.log,
		db:  tx,
	}, nil
}

// Create inserts a new payload into the database.
func (s *Store) Create(ctx context.Context, p payloadbus.Payload) error {
	const q = `
	INSERT INTO payloads
		(payload_id, kind, status, body_text, original_filename, content_type, object_key, size_bytes, result_text, error_text, date_created, date_updated)
	VALUES
		($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

	db := toDBPayload(p)
	if _, err := s.db.Exec(ctx, q,
		db.PayloadID, db.Kind, db.Status, db.BodyText, db.OriginalFilename,
		db.ContentType, db.ObjectKey, db.SizeBytes, db.ResultText, db.ErrorText,
		db.DateCreated, db.DateUpdated); err != nil {
		return sqldb.TranslateError(err)
	}

	return nil
}

// Update modifies a payload in the database.
func (s *Store) Update(ctx context.Context, p payloadbus.Payload) error {
	const q = `
	UPDATE payloads SET
		kind = $2, status = $3, body_text = $4, original_filename = $5, content_type = $6,
		object_key = $7, size_bytes = $8, result_text = $9, error_text = $10, date_updated = $11
	WHERE payload_id = $1`

	db := toDBPayload(p)
	if _, err := s.db.Exec(ctx, q,
		db.PayloadID, db.Kind, db.Status, db.BodyText, db.OriginalFilename,
		db.ContentType, db.ObjectKey, db.SizeBytes, db.ResultText, db.ErrorText,
		db.DateUpdated); err != nil {
		return sqldb.TranslateError(err)
	}

	return nil
}

// QueryByID gets the specified payload from the database.
func (s *Store) QueryByID(ctx context.Context, payloadID uuid.UUID) (payloadbus.Payload, error) {
	const q = `SELECT ` + columns + ` FROM payloads WHERE payload_id = $1`

	rows, err := s.db.Query(ctx, q, payloadID)
	if err != nil {
		return payloadbus.Payload{}, sqldb.TranslateError(err)
	}

	db, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[payloadDB])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return payloadbus.Payload{}, payloadbus.ErrNotFound
		}
		return payloadbus.Payload{}, sqldb.TranslateError(err)
	}

	return toBusPayload(db)
}

// Query retrieves a list of payloads matching the filter.
func (s *Store) Query(ctx context.Context, filter payloadbus.QueryFilter, orderBy order.By, page page.Page) ([]payloadbus.Payload, error) {
	var args []any

	buf := bytes.NewBufferString(`SELECT ` + columns + ` FROM payloads`)
	applyFilter(filter, &args, buf)

	clause, err := orderByClause(orderBy)
	if err != nil {
		return nil, err
	}
	buf.WriteString(clause)

	args = append(args, page.RowsPerPage())
	buf.WriteString(" LIMIT $" + strconv.Itoa(len(args)))

	args = append(args, (page.Number()-1)*page.RowsPerPage())
	buf.WriteString(" OFFSET $" + strconv.Itoa(len(args)))

	rows, err := s.db.Query(ctx, buf.String(), args...)
	if err != nil {
		return nil, sqldb.TranslateError(err)
	}

	dbs, err := pgx.CollectRows(rows, pgx.RowToStructByName[payloadDB])
	if err != nil {
		return nil, sqldb.TranslateError(err)
	}

	return toBusPayloads(dbs)
}

// Count returns the number of payloads matching the filter.
func (s *Store) Count(ctx context.Context, filter payloadbus.QueryFilter) (int, error) {
	var args []any

	buf := bytes.NewBufferString(`SELECT count(1) FROM payloads`)
	applyFilter(filter, &args, buf)

	var count int
	if err := s.db.QueryRow(ctx, buf.String(), args...).Scan(&count); err != nil {
		return 0, sqldb.TranslateError(err)
	}

	return count, nil
}
