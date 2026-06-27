package checkapp

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/i33ym/tetra/foundation/blob"
	"github.com/i33ym/tetra/foundation/logger"
	"github.com/i33ym/tetra/foundation/web"
)

// Config contains all the mandatory systems required by handlers.
type Config struct {
	Build string
	Log   *logger.Logger
	DB    *pgxpool.Pool
	Blob  *blob.Store
}

// Routes adds specific routes for this group.
func Routes(app *web.App, cfg Config) {
	const version = "v1"

	api := newApp(cfg)

	app.HandlerFuncNoMid(http.MethodGet, version, "/liveness", api.liveness)
	app.HandlerFuncNoMid(http.MethodGet, version, "/readiness", api.readiness)
}
