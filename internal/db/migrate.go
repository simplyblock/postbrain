package db

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // pgx/v5 database driver
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
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

// advisoryLockTimeout is how long to wait for the migration advisory lock
// before giving up. A long wait usually means another instance is migrating
// or a previous run crashed without releasing the lock session.
const advisoryLockTimeout = "30s"

// schemaPlaceholder is replaced with the configured schema name in migration SQL.
const schemaPlaceholder = "{{POSTBRAIN_SCHEMA}}"

// schemaSource wraps a source.Driver and substitutes {{POSTBRAIN_SCHEMA}} in every
// migration file with the configured schema before the SQL is executed. This
// lets migrations use explicit schema qualification without hard-coding the
// schema name, and without relying on search_path — which PGBouncer rejects as
// a startup parameter (SQLSTATE 08P01).
type schemaSource struct {
	inner  source.Driver
	schema string
}

func (s *schemaSource) Open(rawURL string) (source.Driver, error) {
	d, err := s.inner.Open(rawURL)
	if err != nil {
		return nil, err
	}
	return &schemaSource{inner: d, schema: s.schema}, nil
}

func (s *schemaSource) Close() error                    { return s.inner.Close() }
func (s *schemaSource) First() (uint, error)            { return s.inner.First() }
func (s *schemaSource) Prev(version uint) (uint, error) { return s.inner.Prev(version) }
func (s *schemaSource) Next(version uint) (uint, error) { return s.inner.Next(version) }

func (s *schemaSource) ReadUp(version uint) (io.ReadCloser, string, error) {
	rc, id, err := s.inner.ReadUp(version)
	if err != nil {
		return nil, id, err
	}
	return s.substitute(rc), id, nil
}

func (s *schemaSource) ReadDown(version uint) (io.ReadCloser, string, error) {
	rc, id, err := s.inner.ReadDown(version)
	if err != nil {
		return nil, id, err
	}
	return s.substitute(rc), id, nil
}

func (s *schemaSource) substitute(rc io.ReadCloser) io.ReadCloser {
	b, _ := io.ReadAll(rc)
	_ = rc.Close()
	return io.NopCloser(strings.NewReader(
		strings.ReplaceAll(string(b), schemaPlaceholder, s.schema),
	))
}

// acquireAdvisoryLock scopes lock_timeout to just the advisory-lock acquisition
// by using SET LOCAL inside a short transaction. The session-level advisory
// lock remains held after commit, but the temporary lock_timeout does not leak
// into later migration statements or future reuse of the pooled connection.
func acquireAdvisoryLock(ctx context.Context, conn *pgxpool.Conn) error {
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("migrate: begin advisory lock transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	if _, err := tx.Exec(ctx, "SET LOCAL lock_timeout = '"+advisoryLockTimeout+"'"); err != nil {
		return fmt.Errorf("migrate: set local lock_timeout: %w", err)
	}
	slog.Info("migrate: acquiring advisory lock")
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_lock($1)", advisoryLockKey); err != nil {
		return fmt.Errorf("migrate: acquire advisory lock (timeout %s — another instance may be migrating): %w", advisoryLockTimeout, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("migrate: commit advisory lock transaction: %w", err)
	}
	committed = true
	slog.Info("migrate: advisory lock acquired")
	return nil
}

// CheckAndMigrate applies pending schema migrations under a PostgreSQL advisory
// lock. schema is the PostgreSQL schema that owns the migration objects
// (e.g. "public"); it is substituted for {{POSTBRAIN_SCHEMA}} in every migration
// file. It:
//  1. Acquires an advisory lock to prevent concurrent migration runs.
//  2. Checks the current schema version against ExpectedVersion.
//  3. Returns an error if the database schema is ahead of the binary.
//  4. Returns an error if the schema is in a dirty state.
//  5. Applies pending migrations with migrate.Up().
//  6. Releases the advisory lock.
func CheckAndMigrate(ctx context.Context, pool *pgxpool.Pool, schema string, autoMigrate bool) error {
	m, conn, release, err := newMigrator(ctx, pool, schema)
	if err != nil {
		return err
	}
	defer release()

	if err := acquireAdvisoryLock(ctx, conn); err != nil {
		return err
	}
	defer func() {
		if _, unlockErr := conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", advisoryLockKey); unlockErr != nil {
			slog.Error("migrate: release advisory lock", "error", unlockErr)
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

	if err := EnsureAGEOverlay(ctx, pool); err != nil {
		return fmt.Errorf("migrate: ensure age overlay: %w", err)
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

// MigrationInfo describes a single migration file.
type MigrationInfo struct {
	Version uint
	Name    string
	Applied bool
	Dirty   bool
}

// MigrateStatus returns the list of all known migrations together with their
// applied/pending state. schema is substituted for {{POSTBRAIN_SCHEMA}} in
// migration files (used to locate the schema_migrations table). It does not
// acquire the advisory lock because it only reads state.
func MigrateStatus(ctx context.Context, pool *pgxpool.Pool, schema string) ([]MigrationInfo, error) {
	m, _, release, err := newMigrator(ctx, pool, schema)
	if err != nil {
		return nil, err
	}
	defer release()

	currentVersion, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return nil, fmt.Errorf("migrate: get version: %w", err)
	}

	// Walk embedded migration files to build the full list.
	var infos []MigrationInfo
	_ = fs.WalkDir(migrationsFS, "migrations", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		// Only process .up.sql files to avoid listing down files as separate entries.
		if !strings.HasSuffix(path, ".up.sql") {
			return nil
		}
		base := d.Name() // e.g. "000003_knowledge_layer.up.sql"
		parts := strings.SplitN(base, "_", 2)
		if len(parts) < 2 {
			return nil
		}
		v, parseErr := strconv.ParseUint(parts[0], 10, 32)
		if parseErr != nil {
			return nil
		}
		name := strings.TrimSuffix(parts[1], ".up.sql")
		applied := !errors.Is(err, migrate.ErrNilVersion) && uint(v) <= currentVersion
		isDirty := dirty && uint(v) == currentVersion
		infos = append(infos, MigrationInfo{
			Version: uint(v),
			Name:    name,
			Applied: applied,
			Dirty:   isDirty,
		})
		return nil
	})

	sort.Slice(infos, func(i, j int) bool { return infos[i].Version < infos[j].Version })
	return infos, nil
}

// MigrateDown rolls back the last n migrations. Pass n=1 to roll back one step.
// schema is substituted for {{POSTBRAIN_SCHEMA}} in migration files.
func MigrateDown(ctx context.Context, pool *pgxpool.Pool, schema string, n int) error {
	if n <= 0 {
		n = 1
	}
	m, conn, release, err := newMigrator(ctx, pool, schema)
	if err != nil {
		return err
	}
	defer release()

	if err := acquireAdvisoryLock(ctx, conn); err != nil {
		return err
	}
	defer func() {
		if _, unlockErr := conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", advisoryLockKey); unlockErr != nil {
			slog.Error("migrate: release advisory lock", "error", unlockErr)
		}
	}()

	if err := m.Steps(-n); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate down %d: %w", n, err)
	}

	version, dirty, verErr := m.Version()
	if errors.Is(verErr, migrate.ErrNilVersion) {
		slog.Info("migrate: rolled back to clean state (no migrations applied)")
	} else {
		if verErr != nil {
			slog.Warn("migrate: could not read version after rollback", "err", verErr)
		}
		slog.Info("migrate: rolled back", "steps", n, "current_version", version, "dirty", dirty)
	}
	return nil
}

// MigrateForce sets the schema_migrations version to v without running any SQL.
// Use this to clear a dirty state after manually fixing a failed migration.
// schema is substituted for {{POSTBRAIN_SCHEMA}} in migration files.
func MigrateForce(ctx context.Context, pool *pgxpool.Pool, schema string, v int) error {
	m, conn, release, err := newMigrator(ctx, pool, schema)
	if err != nil {
		return err
	}
	defer release()

	if err := acquireAdvisoryLock(ctx, conn); err != nil {
		return err
	}
	defer func() {
		if _, unlockErr := conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", advisoryLockKey); unlockErr != nil {
			slog.Error("migrate: release advisory lock", "error", unlockErr)
		}
	}()

	if err := m.Force(v); err != nil {
		return fmt.Errorf("migrate force %d: %w", v, err)
	}
	slog.Info("migrate: forced version", "version", v)
	return nil
}

// newMigrator creates a golang-migrate Migrator from the pool's connection config.
// schema is substituted for {{POSTBRAIN_SCHEMA}} in every migration file so
// that DDL targets the correct PostgreSQL schema. An empty schema defaults to
// "public". The caller is responsible for calling release() to return the
// connection to the pool. The returned *pgxpool.Conn is available for the
// caller to acquire advisory locks on.
func newMigrator(ctx context.Context, pool *pgxpool.Pool, schema string) (*migrate.Migrate, *pgxpool.Conn, func(), error) {
	if schema == "" {
		schema = "public"
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, nil, func() {}, fmt.Errorf("migrate: acquire connection: %w", err)
	}

	rawSrc, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		conn.Release()
		return nil, nil, func() {}, fmt.Errorf("migrate: create iofs source: %w", err)
	}
	src := &schemaSource{inner: rawSrc, schema: schema}

	connConfig := conn.Conn().Config()
	dsn, err := buildMigratorDSN(connConfig, schema)
	if err != nil {
		conn.Release()
		return nil, nil, func() {}, fmt.Errorf("migrate: build migrator dsn: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	if err != nil {
		conn.Release()
		return nil, nil, func() {}, fmt.Errorf("migrate: create migrator: %w", err)
	}

	release := func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			slog.Error("migrate: close source", "error", srcErr)
		}
		if dbErr != nil {
			slog.Error("migrate: close db", "error", dbErr)
		}
		conn.Release()
	}

	return m, conn, release, nil
}

// buildMigratorDSN constructs a golang-migrate DSN from the pool's connection
// config. The migrations tracking table is placed in schema so it matches the
// objects created by the migrations themselves. No search_path startup parameter
// is set: PGBouncer rejects that parameter (SQLSTATE 08P01), and with explicit
// schema qualification in the migration SQL it is not needed.
func buildMigratorDSN(connConfig *pgx.ConnConfig, schema string) (string, error) {
	base := fmt.Sprintf("pgx5://%s:%s@%s:%d/%s",
		url.QueryEscape(connConfig.User),
		url.QueryEscape(connConfig.Password),
		connConfig.Host,
		connConfig.Port,
		connConfig.Database,
	)
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("x-migrations-table", fmt.Sprintf(`"%s"."schema_migrations"`, schema))
	q.Set("x-migrations-table-quoted", "1")
	u.RawQuery = q.Encode()
	return u.String(), nil
}
