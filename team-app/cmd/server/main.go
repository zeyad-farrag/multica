package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// main boots the team-app standalone server.
//
// Boot sequence (AC4, AC5):
//  1. Configure structured slog with the team-app field contract
//     (org_id, workspace_id, actor_user_id, outcome, duration_ms).
//  2. Validate required env vars (AR8) — missing/empty/unparseable → exit(1).
//  3. Connect to Postgres via pgxpool.New(ctx, DATABASE_URL).
//  4. Mount the Chi router from router.go and ListenAndServe(":8080").
//
// TIME-RULE: this server uses time.Now() directly per Architecture Enforcement
// Guidelines #9 — there is no clock.Clock interface. Tests inject time via
// struct fields when a service needs to control it.
func main() {
	logger := newLogger()
	slog.SetDefault(logger)

	if err := validateEnv(); err != nil {
		var miss *MissingEnvVarError
		if errors.As(err, &miss) {
			logger.Error("boot validation failed",
				slog.String("missing_env_var", miss.Name),
				slog.String("reason", miss.Reason),
				slog.String("outcome", "boot_aborted"),
			)
		} else {
			logger.Error("boot validation failed",
				slog.String("error", err.Error()),
				slog.String("outcome", "boot_aborted"),
			)
		}
		os.Exit(1)
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		logger.Error("boot validation failed",
			slog.String("missing_env_var", "DATABASE_URL"),
			slog.String("outcome", "boot_aborted"),
		)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		logger.Error("postgres pool init failed",
			slog.String("error", err.Error()),
			slog.String("outcome", "boot_aborted"),
		)
		os.Exit(1)
	}
	defer pool.Close()

	r := newRouter(pool)

	addr := ":8080"
	logger.Info("team-app server listening",
		slog.String("addr", addr),
		slog.String("outcome", "boot_ok"),
	)
	if err := http.ListenAndServe(addr, r); err != nil {
		logger.Error("http server exited",
			slog.String("error", err.Error()),
			slog.String("outcome", "shutdown_error"),
		)
		os.Exit(1)
	}
}

// newLogger returns a JSON slog.Logger with the structured-fields contract
// pre-applied as empty defaults. Handlers/services override these with
// the live values via slog.With when they have context.
func newLogger() *slog.Logger {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(h).With(
		slog.String("org_id", ""),
		slog.String("workspace_id", ""),
		slog.String("actor_user_id", ""),
		slog.String("outcome", ""),
		slog.Int64("duration_ms", 0),
	)
}
