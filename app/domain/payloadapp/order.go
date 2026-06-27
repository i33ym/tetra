package payloadapp

import "github.com/i33ym/tetra/business/domain/payloadbus"

// orderByFields maps the API orderBy keys to the business order-by keys.
var orderByFields = map[string]string{
	"id":          payloadbus.OrderByID,
	"status":      payloadbus.OrderByStatus,
	"kind":        payloadbus.OrderByKind,
	"dateCreated": payloadbus.OrderByDateCreated,
}
