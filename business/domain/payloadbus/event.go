package payloadbus

import (
	"encoding/json"

	"github.com/google/uuid"

	"github.com/i33ym/tetra/business/sdk/delegate"
)

// Set of delegate event identifiers for this domain.
const (
	// DomainName represents the name of this domain.
	DomainName = "payload"

	// ActionCreated represents a payload having been created.
	ActionCreated = "created"
)

// ActionCreatedParams represents the parameters for the created action.
type ActionCreatedParams struct {
	PayloadID uuid.UUID
}

// String implements the fmt.Stringer interface.
func (p ActionCreatedParams) String() string {
	return "payloadID:" + p.PayloadID.String()
}

// Marshal returns the event parameters encoded as JSON.
func (p ActionCreatedParams) Marshal() ([]byte, error) {
	return json.Marshal(p)
}

// ActionCreatedData constructs the data for the created action.
func ActionCreatedData(payloadID uuid.UUID) delegate.Data {
	params := ActionCreatedParams{PayloadID: payloadID}

	rawParams, err := params.Marshal()
	if err != nil {
		panic(err)
	}

	return delegate.Data{
		Domain:    DomainName,
		Action:    ActionCreated,
		RawParams: rawParams,
	}
}
