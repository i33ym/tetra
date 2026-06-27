// Package debug provides the debug mux exposing pprof, expvar, and the
// Prometheus metrics endpoint.
package debug

import (
	"expvar"
	"net/http"
	"net/http/pprof"

	"github.com/i33ym/tetra/app/sdk/metrics"
)

// Mux registers the debug routes and returns the handler.
func Mux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	mux.Handle("/debug/vars", expvar.Handler())
	mux.Handle("/metrics", metrics.Handler())

	return mux
}
