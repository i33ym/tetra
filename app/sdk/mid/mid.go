// Package mid provides app level middleware support.
package mid

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/i33ym/tetra/foundation/web"
)

func checkIsError(e web.Encoder) error {
	err, hasError := e.(error)
	if hasError {
		return err
	}

	return nil
}

// =============================================================================

type ctxKey int

const (
	trKey ctxKey = iota + 1
)

func setTran(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, trKey, tx)
}

// GetTran retrieves the active transaction from the context.
func GetTran(ctx context.Context) (pgx.Tx, error) {
	v, ok := ctx.Value(trKey).(pgx.Tx)
	if !ok {
		return nil, errors.New("transaction not found in context")
	}

	return v, nil
}
