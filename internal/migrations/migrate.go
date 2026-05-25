package migrations

import (
	"embed"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed *.sql
var migrationFiles embed.FS

// Run applies all pending migrations to the database.
// It is safe to run multiple times — already-applied migrations are skipped.
// Never deletes or modifies existing data.
func Run(databaseURL string) error {
	// Load migrations from embedded SQL files
	src, err := iofs.New(migrationFiles, ".")
	if err != nil {
		return fmt.Errorf("load migration source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, databaseURL)
	if err != nil {
		return fmt.Errorf("init migration engine: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil {
		// ErrNoChange means all migrations are already applied — not an error
		if err == migrate.ErrNoChange {
			return nil
		}
		return fmt.Errorf("run migrations: %w", err)
	}

	return nil
}

// Version returns the current migration version and whether it's dirty.
func Version(databaseURL string) (uint, bool, error) {
	src, err := iofs.New(migrationFiles, ".")
	if err != nil {
		return 0, false, fmt.Errorf("load migration source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, databaseURL)
	if err != nil {
		return 0, false, fmt.Errorf("init migration engine: %w", err)
	}
	defer m.Close()

	version, dirty, err := m.Version()
	if err != nil {
		return 0, false, fmt.Errorf("get migration version: %w", err)
	}
	return version, dirty, nil
}

// Drop drops all tables (USE WITH CAUTION — for dev only).
func Drop(databaseURL string) error {
	src, err := iofs.New(migrationFiles, ".")
	if err != nil {
		return fmt.Errorf("load migration source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, databaseURL)
	if err != nil {
		return fmt.Errorf("init migration engine: %w", err)
	}
	defer m.Close()

	if err := m.Drop(); err != nil {
		return fmt.Errorf("drop tables: %w", err)
	}
	return nil
}
