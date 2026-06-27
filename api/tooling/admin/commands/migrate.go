// Package commands implements the admin CLI subcommands.
package commands

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/i33ym/tetra/foundation/config"
	"github.com/i33ym/tetra/migrations"
)

func newMigrator(cfg config.DB) (*migrate.Migrate, error) {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("iofs source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, dbURL(cfg))
	if err != nil {
		return nil, fmt.Errorf("new migrate: %w", err)
	}

	return m, nil
}

func dbURL(cfg config.DB) string {
	sslMode := "require"
	if cfg.DisableTLS {
		sslMode = "disable"
	}

	u := url.URL{
		Scheme:   "pgx5",
		User:     url.UserPassword(cfg.User, cfg.Password),
		Host:     cfg.Host,
		Path:     cfg.Name,
		RawQuery: "sslmode=" + sslMode,
	}

	return u.String()
}

// MigrateUp applies all pending migrations.
func MigrateUp(cfg config.DB) error {
	m, err := newMigrator(cfg)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}

	fmt.Println("migrations complete")
	return nil
}

// MigrateDown rolls back the most recent migration.
func MigrateDown(cfg config.DB) error {
	m, err := newMigrator(cfg)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Steps(-1); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate down: %w", err)
	}

	fmt.Println("rolled back one migration")
	return nil
}

// MigrateVersion prints the current migration version and dirty state.
func MigrateVersion(cfg config.DB) error {
	m, err := newMigrator(cfg)
	if err != nil {
		return err
	}
	defer m.Close()

	v, dirty, err := m.Version()
	if err != nil {
		if errors.Is(err, migrate.ErrNilVersion) {
			fmt.Println("version: none (no migrations applied)")
			return nil
		}
		return fmt.Errorf("version: %w", err)
	}

	fmt.Printf("version: %d dirty: %t\n", v, dirty)
	return nil
}
