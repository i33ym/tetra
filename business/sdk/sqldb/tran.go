package sqldb

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Beginner represents a value that can begin a transaction. *pgxpool.Pool
// satisfies this interface, so the pool can be passed directly to the
// transaction middleware.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}
