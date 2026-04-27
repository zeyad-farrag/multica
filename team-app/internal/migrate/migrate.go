// Package migrate applies the embedded *.up.sql / *.down.sql files in lexical order
// against a Postgres connection. No Flyway / Goose / golang-migrate dependency — the
// runtime stays single-binary per architecture §149-§153. Each file is wrapped in its
// own transaction by the SQL itself (BEGIN; ... COMMIT;), so a failure rolls back to
// the prior state.
package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/multica-ai/multica/team-app/migrations"
)

// Executor is the minimal pgx surface needed to apply migrations. Both *pgx.Conn and
// *pgxpool.Pool satisfy it, so callers can hand either in.
type Executor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Up applies every *.up.sql file in lex order.
func Up(ctx context.Context, db Executor) error {
	files, err := migrations.UpFiles()
	if err != nil {
		return err
	}
	return apply(ctx, db, files)
}

// Down applies every *.down.sql file in reverse lex order.
func Down(ctx context.Context, db Executor) error {
	files, err := migrations.DownFiles()
	if err != nil {
		return err
	}
	return apply(ctx, db, files)
}

func apply(ctx context.Context, db Executor, files []string) error {
	for _, name := range files {
		body, err := migrations.Read(name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if _, err := db.Exec(ctx, string(body)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
	}
	return nil
}
