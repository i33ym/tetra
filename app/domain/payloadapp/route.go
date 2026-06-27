package payloadapp

import (
	"net/http"

	"github.com/i33ym/tetra/business/domain/jobbus"
	"github.com/i33ym/tetra/business/domain/payloadbus"
	"github.com/i33ym/tetra/business/sdk/sqldb"
	"github.com/i33ym/tetra/foundation/blob"
	"github.com/i33ym/tetra/foundation/logger"
	"github.com/i33ym/tetra/foundation/web"
)

// Config contains all the mandatory systems required by handlers.
type Config struct {
	Log            *logger.Logger
	PayloadBus     *payloadbus.Business
	JobBus         *jobbus.Business
	Blob           *blob.Store
	DB             sqldb.Beginner
	MaxUploadBytes int64
	MaxAttempts    int
}

// Routes adds specific routes for this group.
func Routes(app *web.App, cfg Config) {
	const version = "v1"

	api := newApp(cfg)

	app.HandlerFunc(http.MethodPost, version, "/payloads", api.create)
	app.HandlerFunc(http.MethodGet, version, "/payloads", api.query)
	app.HandlerFunc(http.MethodGet, version, "/payloads/{payload_id}", api.queryByID)
	app.RawHandlerFunc(http.MethodGet, version, "/payloads/{payload_id}/content", api.content)
}
