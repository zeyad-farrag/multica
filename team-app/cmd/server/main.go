// Command server is the team-app standalone HTTP entry point. Story 1.1 wires the
// router and route registration; this story (1.6) wires the migration runner that
// must finish before any route is registered.
//
// Boot order (per architecture §149-§153 — single-binary, no Flyway / Goose):
//
//  1. Read DATABASE_URL.
//  2. Connect via pgx.
//  3. Apply embedded migrations/*.up.sql in lex order.
//  4. (Story 1.1) wire router and ListenAndServe.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/team-app/internal/migrate"
)

func main() {
	if err := run(); err != nil {
		slog.Error("team-app server exited with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	slog.Info("applying team-app migrations")
	if err := migrate.Up(ctx, pool); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	slog.Info("team-app migrations applied")

	// Story 1.1 wires the chi router and HTTP listener here. Until that lands the
	// binary exits cleanly after migrations succeed, which is enough to satisfy
	// AC1 ("when the server boots against an empty Postgres database, the
	// following tables exist").
	return nil
}
