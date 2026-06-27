package payloaddb

import (
	"fmt"

	"github.com/i33ym/tetra/business/domain/payloadbus"
	"github.com/i33ym/tetra/business/sdk/order"
)

// orderByFields whitelists the order-by keys to real column names so a client
// can never inject an arbitrary ORDER BY column.
var orderByFields = map[string]string{
	payloadbus.OrderByID:          "payload_id",
	payloadbus.OrderByStatus:      "status",
	payloadbus.OrderByKind:        "kind",
	payloadbus.OrderByDateCreated: "date_created",
}

func orderByClause(orderBy order.By) (string, error) {
	by, exists := orderByFields[orderBy.Field]
	if !exists {
		return "", fmt.Errorf("field %q does not exist", orderBy.Field)
	}

	return " ORDER BY " + by + " " + orderBy.Direction, nil
}
