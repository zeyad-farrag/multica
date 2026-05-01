package handler

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultHandlerTestDatabaseURL = "postgres://multica:multica@localhost:5432/multica?sslmode=disable"

type embeddedHandlerTestDatabase struct {
	url         string
	postgres    *embeddedpostgres.EmbeddedPostgres
	runtimePath string
}

func (db *embeddedHandlerTestDatabase) Stop() error {
	stopErr := db.postgres.Stop()
	removeErr := os.RemoveAll(db.runtimePath)
	if stopErr != nil {
		return stopErr
	}
	return removeErr
}

func openHandlerTestPool(ctx context.Context) (*pgxpool.Pool, func() error, string, error) {
	if dbURL, ok := handlerTestDatabaseURL(); ok {
		pool, err := newHandlerTestPool(ctx, dbURL)
		if err != nil {
			return nil, nil, "", err
		}
		return pool, func() error {
			pool.Close()
			return nil
		}, "DATABASE_URL", nil
	}

	pool, err := newHandlerTestPool(ctx, defaultHandlerTestDatabaseURL)
	if err == nil {
		return pool, func() error {
			pool.Close()
			return nil
		}, "localhost:5432 default", nil
	}

	fallback, fallbackErr := startEmbeddedHandlerTestDatabase()
	if fallbackErr != nil {
		return nil, nil, "", fmt.Errorf("local handler test database unavailable: %w; embedded fallback failed: %w", err, fallbackErr)
	}

	pool, poolErr := newHandlerTestPool(ctx, fallback.url)
	if poolErr != nil {
		_ = fallback.Stop()
		return nil, nil, "", poolErr
	}

	if migrateErr := applyHandlerTestMigrations(ctx, pool); migrateErr != nil {
		pool.Close()
		_ = fallback.Stop()
		return nil, nil, "", migrateErr
	}

	return pool, func() error {
		pool.Close()
		return fallback.Stop()
	}, "embedded-postgres fallback", nil
}

func handlerTestDatabaseURL() (string, bool) {
	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	return dbURL, dbURL != ""
}

func newHandlerTestPool(ctx context.Context, dbURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func startEmbeddedHandlerTestDatabase() (*embeddedHandlerTestDatabase, error) {
	port, err := freeHandlerTestPort()
	if err != nil {
		return nil, err
	}

	runtimePath := filepath.Join(os.TempDir(), "multica-handler-tests-"+randomID())
	cachePath := filepath.Join(os.TempDir(), "multica-handler-testdb-cache")

	config := embeddedpostgres.DefaultConfig().
		Version(embeddedpostgres.V17).
		Port(port).
		Database("multica").
		Username("multica").
		Password("multica").
		RuntimePath(runtimePath).
		CachePath(cachePath).
		StartTimeout(45 * time.Second).
		Logger(io.Discard)

	postgres := embeddedpostgres.NewDatabase(config)
	if err := postgres.Start(); err != nil {
		return nil, err
	}

	return &embeddedHandlerTestDatabase{
		url:         config.GetConnectionURL() + "?sslmode=disable",
		postgres:    postgres,
		runtimePath: runtimePath,
	}, nil
}

func freeHandlerTestPort() (uint32, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address type %T", listener.Addr())
	}

	return uint32(tcpAddr.Port), nil
}

func applyHandlerTestMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	files, err := handlerTestMigrationFiles()
	if err != nil {
		return fmt.Errorf("resolve migrations: %w", err)
	}

	for _, file := range files {
		version := extractHandlerTestMigrationVersion(file)

		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`,
			version,
		).Scan(&exists); err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if exists {
			continue
		}

		sql, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", file, err)
		}

		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("apply migration %s: %w", file, err)
		}

		if _, err := pool.Exec(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1)`,
			version,
		); err != nil {
			return fmt.Errorf("record migration %s: %w", version, err)
		}
	}

	return nil
}

func handlerTestMigrationFiles() ([]string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("resolve current file")
	}

	dir := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "migrations"))
	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no migrations found in %s", dir)
	}

	sort.Strings(files)
	return files, nil
}

func extractHandlerTestMigrationVersion(filename string) string {
	return strings.TrimSuffix(filepath.Base(filename), ".up.sql")
}
