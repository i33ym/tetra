package payloadbus

import "github.com/i33ym/tetra/business/types/status"

// QueryFilter holds the available fields a query can be filtered on. A nil
// field is not applied.
type QueryFilter struct {
	Status *status.Status
	Kind   *string
}
