package payloaddb

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/i33ym/tetra/business/domain/payloadbus"
	"github.com/i33ym/tetra/business/types/status"
)

type payloadDB struct {
	PayloadID        uuid.UUID `db:"payload_id"`
	Kind             string    `db:"kind"`
	Status           string    `db:"status"`
	BodyText         string    `db:"body_text"`
	OriginalFilename string    `db:"original_filename"`
	ContentType      string    `db:"content_type"`
	ObjectKey        string    `db:"object_key"`
	SizeBytes        int64     `db:"size_bytes"`
	ResultText       string    `db:"result_text"`
	ErrorText        string    `db:"error_text"`
	DateCreated      time.Time `db:"date_created"`
	DateUpdated      time.Time `db:"date_updated"`
}

func toDBPayload(p payloadbus.Payload) payloadDB {
	return payloadDB{
		PayloadID:        p.ID,
		Kind:             p.Kind,
		Status:           p.Status.String(),
		BodyText:         p.BodyText,
		OriginalFilename: p.OriginalFilename,
		ContentType:      p.ContentType,
		ObjectKey:        p.ObjectKey,
		SizeBytes:        p.SizeBytes,
		ResultText:       p.ResultText,
		ErrorText:        p.ErrorText,
		DateCreated:      p.DateCreated.UTC(),
		DateUpdated:      p.DateUpdated.UTC(),
	}
}

func toBusPayload(db payloadDB) (payloadbus.Payload, error) {
	st, err := status.Parse(db.Status)
	if err != nil {
		return payloadbus.Payload{}, fmt.Errorf("parse status: %w", err)
	}

	p := payloadbus.Payload{
		ID:               db.PayloadID,
		Kind:             db.Kind,
		Status:           st,
		BodyText:         db.BodyText,
		OriginalFilename: db.OriginalFilename,
		ContentType:      db.ContentType,
		ObjectKey:        db.ObjectKey,
		SizeBytes:        db.SizeBytes,
		ResultText:       db.ResultText,
		ErrorText:        db.ErrorText,
		DateCreated:      db.DateCreated.Local(),
		DateUpdated:      db.DateUpdated.Local(),
	}

	return p, nil
}

func toBusPayloads(dbs []payloadDB) ([]payloadbus.Payload, error) {
	ps := make([]payloadbus.Payload, len(dbs))
	for i, db := range dbs {
		p, err := toBusPayload(db)
		if err != nil {
			return nil, err
		}
		ps[i] = p
	}

	return ps, nil
}
