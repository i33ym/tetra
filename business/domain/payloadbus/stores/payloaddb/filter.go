package payloaddb

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/i33ym/tetra/business/domain/payloadbus"
)

// applyFilter appends a WHERE clause for each non-nil filter field, adding the
// matching positional argument to args in lockstep ($1, $2, ...).
func applyFilter(filter payloadbus.QueryFilter, args *[]any, buf *bytes.Buffer) {
	var clauses []string

	if filter.Status != nil {
		*args = append(*args, filter.Status.String())
		clauses = append(clauses, "status = $"+strconv.Itoa(len(*args)))
	}

	if filter.Kind != nil {
		*args = append(*args, *filter.Kind)
		clauses = append(clauses, "kind = $"+strconv.Itoa(len(*args)))
	}

	if len(clauses) > 0 {
		buf.WriteString(" WHERE ")
		buf.WriteString(strings.Join(clauses, " AND "))
	}
}
