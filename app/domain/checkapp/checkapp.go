// Package checkapp maintains the liveness and readiness handlers.
package checkapp

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/i33ym/tetra/app/sdk/errs"
	"github.com/i33ym/tetra/business/sdk/sqldb"
	"github.com/i33ym/tetra/foundation/blob"
	"github.com/i33ym/tetra/foundation/logger"
	"github.com/i33ym/tetra/foundation/web"
)

type app struct {
	build string
	log   *logger.Logger
	db    *pgxpool.Pool
	blob  *blob.Store
}

func newApp(cfg Config) *app {
	return &app{
		build: cfg.Build,
		log:   cfg.Log,
		db:    cfg.DB,
		blob:  cfg.Blob,
	}
}

// Info reports the health/identity of the service.
type Info struct {
	Status     string `json:"status"`
	Build      string `json:"build,omitempty"`
	Host       string `json:"host,omitempty"`
	Name       string `json:"name,omitempty"`
	PodIP      string `json:"podIP,omitempty"`
	Node       string `json:"node,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	GOMAXPROCS int    `json:"GOMAXPROCS,omitempty"`
}

// Encode implements the web.Encoder interface.
func (i Info) Encode() ([]byte, string, error) {
	data, err := json.Marshal(i)
	return data, "application/json", err
}

func (a *app) liveness(_ context.Context, _ *http.Request) web.Encoder {
	host, err := os.Hostname()
	if err != nil {
		host = "unavailable"
	}

	return Info{
		Status:     "up",
		Build:      a.build,
		Host:       host,
		Name:       os.Getenv("KUBERNETES_NAME"),
		PodIP:      os.Getenv("KUBERNETES_POD_IP"),
		Node:       os.Getenv("KUBERNETES_NODE_NAME"),
		Namespace:  os.Getenv("KUBERNETES_NAMESPACE"),
		GOMAXPROCS: runtime.GOMAXPROCS(0),
	}
}

func (a *app) readiness(ctx context.Context, _ *http.Request) web.Encoder {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	if err := sqldb.StatusCheck(ctx, a.db); err != nil {
		return errs.Errorf(errs.Unavailable, "database not ready: %s", err)
	}

	if a.blob != nil {
		if err := a.blob.HealthCheck(ctx); err != nil {
			return errs.Errorf(errs.Unavailable, "object store not ready: %s", err)
		}
	}

	return Info{Status: "ok"}
}
