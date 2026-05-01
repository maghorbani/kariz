package internal

import (
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	_ "github.com/golang-migrate/migrate/v4/database/postgres"

	"github.com/kariz/kariz/internal/config"
)

// ConnectDB establishes a connection to PostgreSQL and verifies it with a ping.
func ConnectDB(cfg *config.Config) (*sqlx.DB, error) {
	db, err := sqlx.Connect("postgres", cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	slog.Info("connected to PostgreSQL",
		"dsn", cfg.DatabaseURL,
	)

	return db, nil
}

// RunMigrations applies all pending database migrations using the provided
// embedded filesystem. The caller is responsible for embedding the migrations
// directory and passing it here (Go's embed directive cannot use ".." paths,
// so the embed must live in a package at or above the migrations/ directory).
func RunMigrations(migrationsFS fs.FS, subdir string, databaseURL string) error {
	d, err := iofs.New(migrationsFS, subdir)
	if err != nil {
		return fmt.Errorf("failed to open migrations source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", d, databaseURL)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	version, dirty, _ := m.Version()
	slog.Info("database migrations applied",
		"version", version,
		"dirty", dirty,
	)

	return nil
}
