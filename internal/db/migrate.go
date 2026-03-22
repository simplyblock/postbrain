package db

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // pgx/v5 database driver
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ExpectedVersion is the schema version the binary was compiled against.
// Override at build time with:
//
//	go build -ldflags "-X github.com/simplyblock/postbrain/internal/db.ExpectedVersion=5"
var ExpectedVersion = 0

//go:embed migrations/*.sql
var migrationsFS embed.FS

// advisoryLockKey is the PostgreSQL advisory lock key used to serialise
// migration runs across multiple instances. The value spells "postbrai"
// as an int64.
const advisoryLockKey = int64(0x706f737462726169) // 8101067571501756777

// CheckAndMigrate applies pending schema migrations under a PostgreSQL advisory
// lock. It:
//  1. Acquires an advisory lock to prevent concurrent migration runs.
//  2. Checks the current schema version against ExpectedVersion.
//  3. Returns an error if the database schema is ahead of the binary.
//  4. Returns an error if the schema is in a dirty state.
//  5. Applies pending migrations with migrate.Up().
//  6. Releases the advisory lock.
//
// TODO(task-infra): add an integration test that exercises CheckAndMigrate
// against a real PostgreSQL instance spun up by testcontainers.
func CheckAndMigrate(ctx context.Context, pool *pgxpool.Pool, autoMigrate bool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("migrate: acquire connection: %w", err)
	}
	defer conn.Release()

	// Acquire advisory lock (session-level; released when connection is returned).
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", advisoryLockKey); err != nil {
		return fmt.Errorf("migrate: acquire advisory lock: %w", err)
	}
	defer func() {
		if _, unlockErr := conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", advisoryLockKey); unlockErr != nil {
			slog.Error("migrate: release advisory lock", "error", unlockErr)
		}
	}()

	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("migrate: create iofs source: %w", err)
	}

	// Build a DSN from the pool config for the migrate driver.
	connConfig := conn.Conn().Config()
	dsn := fmt.Sprintf("pgx5://%s:%s@%s:%d/%s",
		connConfig.User,
		connConfig.Password,
		connConfig.Host,
		connConfig.Port,
		connConfig.Database,
	)

	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	if err != nil {
		return fmt.Errorf("migrate: create migrator: %w", err)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			slog.Error("migrate: close source", "error", srcErr)
		}
		if dbErr != nil {
			slog.Error("migrate: close db", "error", dbErr)
		}
	}()

	version, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return fmt.Errorf("migrate: get version: %w", err)
	}

	if dirty {
		return fmt.Errorf("migrate: schema is dirty at version %d — run migrate force to recover", version)
	}

	if !errors.Is(err, migrate.ErrNilVersion) && ExpectedVersion > 0 && int(version) > ExpectedVersion {
		return fmt.Errorf("migrate: schema version %d is ahead of binary version %d", version, ExpectedVersion)
	}

	if !autoMigrate {
		slog.Info("migrate: auto_migrate disabled, skipping")
		return nil
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate: apply migrations: %w", err)
	}

	newVersion, _, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return fmt.Errorf("migrate: get version after migration: %w", err)
	}
	slog.Info("migrate: schema up to date", "version", newVersion)

	return nil
}
