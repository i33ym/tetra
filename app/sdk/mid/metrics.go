package mid

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/i33ym/tetra/app/sdk/errs"
	"github.com/i33ym/tetra/app/sdk/metrics"
	"github.com/i33ym/tetra/foundation/web"
)

// Metrics records Prometheus metrics for each request (count, latency, status).
func Metrics() web.MidFunc {
	m := func(next web.HandlerFunc) web.HandlerFunc {
		h := func(ctx context.Context, r *http.Request) web.Encoder {
			now := time.Now()

			resp := next(ctx, r)

			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = r.URL.Path
			}

			code := http.StatusOK
			if err := checkIsError(resp); err != nil {
				code = http.StatusInternalServerError

				var appErr *errs.Error
				if errors.As(err, &appErr) {
					code = appErr.HTTPStatus()
				}
			} else if resp == nil {
				code = http.StatusNoContent
			}

			metrics.ObserveHTTP(r.Method, route, code, time.Since(now))

			return resp
		}

		return h
	}

	return m
}
