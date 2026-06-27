package payloadapp

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/i33ym/tetra/app/sdk/errs"
	"github.com/i33ym/tetra/business/domain/payloadbus"
)

// NewPayloadText is the JSON body for a text-only payload.
type NewPayloadText struct {
	Text string `json:"text"`
}

// Decode implements the web.Decoder interface.
func (n *NewPayloadText) Decode(data []byte) error {
	return json.Unmarshal(data, n)
}

// Validate checks the data in the model is considered clean.
func (n NewPayloadText) Validate() error {
	if n.Text == "" {
		return errs.NewFieldErrors("text", errors.New("must not be empty"))
	}
	return nil
}

// =============================================================================

// AppPayload is the API representation of a payload.
type AppPayload struct {
	ID               string `json:"id"`
	Kind             string `json:"kind"`
	Status           string `json:"status"`
	BodyText         string `json:"bodyText,omitempty"`
	OriginalFilename string `json:"originalFilename,omitempty"`
	ContentType      string `json:"contentType,omitempty"`
	SizeBytes        int64  `json:"sizeBytes,omitempty"`
	ResultText       string `json:"resultText,omitempty"`
	ErrorText        string `json:"errorText,omitempty"`
	DateCreated      string `json:"dateCreated"`
	DateUpdated      string `json:"dateUpdated"`
}

// Encode implements the web.Encoder interface.
func (app AppPayload) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toAppPayload(p payloadbus.Payload) AppPayload {
	return AppPayload{
		ID:               p.ID.String(),
		Kind:             p.Kind,
		Status:           p.Status.String(),
		BodyText:         p.BodyText,
		OriginalFilename: p.OriginalFilename,
		ContentType:      p.ContentType,
		SizeBytes:        p.SizeBytes,
		ResultText:       p.ResultText,
		ErrorText:        p.ErrorText,
		DateCreated:      p.DateCreated.Format(time.RFC3339),
		DateUpdated:      p.DateUpdated.Format(time.RFC3339),
	}
}

func toAppPayloads(ps []payloadbus.Payload) []AppPayload {
	items := make([]AppPayload, len(ps))
	for i, p := range ps {
		items[i] = toAppPayload(p)
	}
	return items
}

// =============================================================================

// acceptedResponse is returned from a successful create. It carries a 202
// Accepted status because processing happens asynchronously.
type acceptedResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// Encode implements the web.Encoder interface.
func (r acceptedResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(r)
	return data, "application/json", err
}

// HTTPStatus implements the web httpStatus interface.
func (acceptedResponse) HTTPStatus() int {
	return http.StatusAccepted
}
