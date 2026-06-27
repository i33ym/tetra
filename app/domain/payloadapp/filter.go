package payloadapp

import (
	"errors"
	"net/http"

	"github.com/i33ym/tetra/app/sdk/errs"
	"github.com/i33ym/tetra/business/domain/payloadbus"
	"github.com/i33ym/tetra/business/types/status"
)

type queryParams struct {
	page    string
	rows    string
	orderBy string
	status  string
	kind    string
}

func parseQueryParams(r *http.Request) queryParams {
	v := r.URL.Query()

	return queryParams{
		page:    v.Get("page"),
		rows:    v.Get("rows"),
		orderBy: v.Get("orderBy"),
		status:  v.Get("status"),
		kind:    v.Get("kind"),
	}
}

func parseFilter(qp queryParams) (payloadbus.QueryFilter, error) {
	var filter payloadbus.QueryFilter

	if qp.status != "" {
		st, err := status.Parse(qp.status)
		if err != nil {
			return payloadbus.QueryFilter{}, errs.NewFieldErrors("status", err)
		}
		filter.Status = &st
	}

	if qp.kind != "" {
		if qp.kind != payloadbus.KindText && qp.kind != payloadbus.KindFile {
			return payloadbus.QueryFilter{}, errs.NewFieldErrors("kind", errors.New("must be 'text' or 'file'"))
		}
		k := qp.kind
		filter.Kind = &k
	}

	return filter, nil
}
