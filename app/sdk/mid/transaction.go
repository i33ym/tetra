package mid

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/i33ym/tetra/app/sdk/errs"
	"github.com/i33ym/tetra/business/sdk/sqldb"
	"github.com/i33ym/tetra/foundation/logger"
	"github.com/i33ym/tetra/foundation/web"
)

// BeginCommitRollback starts a transaction for the domain call. It commits when
// the handler returns a non-error response and rolls back otherwise.
func BeginCommitRollback(log *logger.Logger, bgn sqldb.Beginner) web.MidFunc {
	m := func(next web.HandlerFunc) web.HandlerFunc {
		h := func(ctx context.Context, r *http.Request) web.Encoder {
			hasCommitted := false

			log.Info(ctx, "BEGIN TRANSACTION")
			tx, err := bgn.Begin(ctx)
			if err != nil {
				return errs.Errorf(errs.Internal, "BEGIN TRANSACTION: %s", err)
			}

			defer func() {
				if !hasCommitted {
					log.Info(ctx, "ROLLBACK TRANSACTION")
				}

				// Use a background context so rollback still runs if the request
				// context was canceled.
				if err := tx.Rollback(context.Background()); err != nil {
					if errors.Is(err, pgx.ErrTxClosed) {
						return
					}
					log.Info(ctx, "ROLLBACK TRANSACTION", "ERROR", err)
				}
			}()

			ctx = setTran(ctx, tx)

			resp := next(ctx, r)

			if checkIsError(resp) != nil {
				return resp
			}

			log.Info(ctx, "COMMIT TRANSACTION")
			if err := tx.Commit(ctx); err != nil {
				return errs.Errorf(errs.Internal, "COMMIT TRANSACTION: %s", err)
			}

			hasCommitted = true

			return resp
		}

		return h
	}

	return m
}
