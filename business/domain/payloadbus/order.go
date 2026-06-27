package payloadbus

import "github.com/i33ym/tetra/business/sdk/order"

// DefaultOrderBy represents the default way we sort.
var DefaultOrderBy = order.NewBy(OrderByDateCreated, order.DESC)

// Set of opaque order-by keys. The store maps these to real columns so the
// client never names a column directly (SQL-injection guard for ORDER BY).
const (
	OrderByID          = "payload_id"
	OrderByStatus      = "status"
	OrderByKind        = "kind"
	OrderByDateCreated = "date_created"
)
