// The admin binary runs operational tasks: database migrations and MinIO bucket
// bootstrap. It is bundled into the service images so an init container can run
// it before the service starts.
package main

import (
	"fmt"
	"os"

	"github.com/i33ym/tetra/api/tooling/admin/commands"
	"github.com/i33ym/tetra/foundation/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "admin:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv("TETRA_CONFIG"))
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if len(os.Args) < 2 {
		return fmt.Errorf("usage: admin <migrate-up|migrate-down|migrate-version|minio-bootstrap>")
	}

	switch os.Args[1] {
	case "migrate-up":
		return commands.MigrateUp(cfg.DB)
	case "migrate-down":
		return commands.MigrateDown(cfg.DB)
	case "migrate-version":
		return commands.MigrateVersion(cfg.DB)
	case "minio-bootstrap":
		return commands.MinioBootstrap(cfg.MinIO)
	default:
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
}
