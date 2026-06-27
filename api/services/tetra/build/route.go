// Package build maintains the route set for the tetra API service.
package build

import (
	"github.com/i33ym/tetra/app/domain/checkapp"
	"github.com/i33ym/tetra/app/domain/payloadapp"
	"github.com/i33ym/tetra/app/sdk/mux"
	"github.com/i33ym/tetra/foundation/web"
)

// Routes returns the route adder for the service.
func Routes() add {
	return add{}
}

type add struct{}

// Add implements the mux.RouteAdder interface.
func (add) Add(app *web.App, cfg mux.Config) {
	checkapp.Routes(app, checkapp.Config{
		Build: cfg.Build,
		Log:   cfg.Log,
		DB:    cfg.DB,
		Blob:  cfg.Blob,
	})

	payloadapp.Routes(app, payloadapp.Config{
		Log:            cfg.Log,
		PayloadBus:     cfg.PayloadBus,
		JobBus:         cfg.JobBus,
		Blob:           cfg.Blob,
		DB:             cfg.DB,
		MaxUploadBytes: cfg.MaxUploadBytes,
		MaxAttempts:    cfg.MaxAttempts,
	})
}
