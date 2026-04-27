// TIM-6 Integration test for 001_init: real Postgres (no mocks per architecture §326),
// isolated in an ephemeral schema so it never collides with other databases sharing
// the test cluster.
//
// Set TEAM_APP_TEST_DATABASE_URL or DATABASE_URL to a reachable Postgres 17 instance.
// The test skips (does not fail) when neither is set, matching the existing pattern
// in server/cmd/server/integration_test.go.
package migrations

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var expectedTables = []string{
	// Mirror tables (4) — created first.
	"mirror_workspace",
	"mirror_user",
	"mirror_member",
	"mirror_issue",
	// Domain tables (8) — created next.
	"member_schedule",
	"member_leave",
	"work_item",
	"time_entry",
	"time_confirm",
	"time_confirm_history",
	"workload_anomaly",
	"activity_log",
}

// dialTestDB connects to the test Postgres or skips. Mirrors the policy used by
// server/cmd/server/integration_test.go: missing DB is "skip", not "fail".
func dialTestDB(ctx context.Context, t *testing.T) *pgx.Conn {
	t.Helper()
	dbURL := os.Getenv("TEAM_APP_TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("TEAM_APP_TEST_DATABASE_URL / DATABASE_URL not set; skipping integration test")
	}
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Skipf("could not connect to Postgres: %v", err)
	}
	if err := conn.Ping(ctx); err != nil {
		_ = conn.Close(context.Background())
		t.Skipf("could not ping Postgres: %v", err)
	}
	return conn
}

// ephemeralSchema returns a fresh schema name for this test run. Random suffix avoids
// collisions when multiple test invocations run in parallel against the shared CI DB.
func ephemeralSchema(t *testing.T) string {
	t.Helper()
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return "team_app_test_" + hex.EncodeToString(buf)
}

// TestMigration001UpDownRoundTrip is the canonical AC1 / AC3 / AC4 / AC5 / AC6 check.
// It applies 001_init.up.sql in an ephemeral schema, asserts the expected shape, then
// applies 001_init.down.sql and asserts the schema is empty.
func TestMigration001UpDownRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	conn := dialTestDB(ctx, t)
	defer conn.Close(context.Background())

	schema := ephemeralSchema(t)
	if _, err := conn.Exec(ctx, fmt.Sprintf("CREATE SCHEMA %s", schema)); err != nil {
		t.Fatalf("create schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer dropCancel()
		_, _ = conn.Exec(dropCtx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schema))
	})

	if _, err := conn.Exec(ctx, fmt.Sprintf("SET search_path TO %s, public", schema)); err != nil {
		t.Fatalf("set search_path: %v", err)
	}

	upBody, err := Read("001_init.up.sql")
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	if _, err := conn.Exec(ctx, string(upBody)); err != nil {
		t.Fatalf("apply 001_init.up.sql: %v", err)
	}

	t.Run("AC1_all_12_tables_exist", func(t *testing.T) {
		got, err := tableNamesInSchema(ctx, conn, schema)
		if err != nil {
			t.Fatalf("query pg_tables: %v", err)
		}
		want := append([]string(nil), expectedTables...)
		sort.Strings(want)
		if !equalSlices(got, want) {
			t.Errorf("expected exactly the 12 schema tables\n got:  %v\n want: %v", got, want)
		}
	})

	t.Run("AC4_idx_time_entry_auto_issue_key_predicate", func(t *testing.T) {
		def, err := indexDef(ctx, conn, schema, "idx_time_entry_auto_issue_key")
		if err != nil {
			t.Fatalf("query pg_indexes: %v", err)
		}
		// Required key columns and the partial-index predicate from AC4.
		mustContainAll(t, def,
			"member_id",
			"issue_id",
			"started_at",
			"kind = 'issue'",
			"source = 'auto'",
			"confirmed = false",
			"user_edited = false",
		)
		if !strings.Contains(strings.ToUpper(def), "UNIQUE INDEX") {
			t.Errorf("expected partial UNIQUE index, got: %s", def)
		}
	})

	t.Run("AC5_idx_workload_anomaly_open_predicate", func(t *testing.T) {
		def, err := indexDef(ctx, conn, schema, "idx_workload_anomaly_open")
		if err != nil {
			t.Fatalf("query pg_indexes: %v", err)
		}
		mustContainAll(t, def,
			"member_id",
			"kind",
			"window_start",
			"window_end",
			"resolved_at IS NULL",
		)
		if !strings.Contains(strings.ToUpper(def), "UNIQUE INDEX") {
			t.Errorf("expected partial UNIQUE index, got: %s", def)
		}
	})

	t.Run("AC3_time_entry_minutes_check_rejects_10_accepts_15", func(t *testing.T) {
		// Use kind='other' / source='manual' so the row trivially satisfies the
		// kind ↔ issue_id / work_item_id / snapshot XOR check; only the 15-min
		// CHECK is exercised.
		insert := func(minutes int) error {
			_, err := conn.Exec(ctx, fmt.Sprintf(
				"INSERT INTO %s.time_entry (workspace_id, member_id, kind, source, started_at, ended_at, minutes) "+
					"VALUES (gen_random_uuid(), gen_random_uuid(), 'other', 'manual', "+
					"  '2026-01-01 09:00:00+00', '2026-01-01 09:%02d:00+00', $1)",
				schema, minutes,
			), minutes)
			return err
		}
		if err := insert(10); err == nil {
			t.Errorf("expected minutes=10 to be rejected by CHECK")
		} else if !isCheckViolation(err) {
			t.Errorf("expected check_violation for minutes=10, got: %v", err)
		}
		if err := insert(15); err != nil {
			t.Errorf("expected minutes=15 to be accepted, got: %v", err)
		}
	})

	t.Run("AC3_work_item_duration_check_rejects_10_accepts_15", func(t *testing.T) {
		insert := func(duration int) error {
			_, err := conn.Exec(ctx, fmt.Sprintf(
				"INSERT INTO %s.work_item (workspace_id, title, category, assignee_id, creator_id, scheduled_for, duration_minutes) "+
					"VALUES (gen_random_uuid(), 'standup', 'meeting', gen_random_uuid(), gen_random_uuid(), "+
					"  '2026-01-02 10:00:00+00', $1)",
				schema,
			), duration)
			return err
		}
		if err := insert(10); err == nil {
			t.Errorf("expected duration_minutes=10 to be rejected by CHECK")
		} else if !isCheckViolation(err) {
			t.Errorf("expected check_violation for duration_minutes=10, got: %v", err)
		}
		if err := insert(15); err != nil {
			t.Errorf("expected duration_minutes=15 to be accepted, got: %v", err)
		}
	})

	// AC6: strict inverse — applying the down migration leaves the schema empty.
	downBody, err := Read("001_init.down.sql")
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	if _, err := conn.Exec(ctx, string(downBody)); err != nil {
		t.Fatalf("apply 001_init.down.sql: %v", err)
	}

	t.Run("AC6_down_leaves_schema_empty", func(t *testing.T) {
		got, err := tableNamesInSchema(ctx, conn, schema)
		if err != nil {
			t.Fatalf("query pg_tables: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("expected schema %s to be empty after down migration, got: %v", schema, got)
		}
	})
}

func tableNamesInSchema(ctx context.Context, conn *pgx.Conn, schema string) ([]string, error) {
	rows, err := conn.Query(ctx, "SELECT tablename FROM pg_tables WHERE schemaname = $1 ORDER BY tablename", schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

func indexDef(ctx context.Context, conn *pgx.Conn, schema, indexName string) (string, error) {
	var def string
	err := conn.QueryRow(ctx,
		"SELECT indexdef FROM pg_indexes WHERE schemaname = $1 AND indexname = $2",
		schema, indexName,
	).Scan(&def)
	if err != nil {
		return "", fmt.Errorf("index %s not found in schema %s: %w", indexName, schema, err)
	}
	return def, nil
}

func mustContainAll(t *testing.T, haystack string, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if !strings.Contains(haystack, n) {
			t.Errorf("expected substring %q in:\n  %s", n, haystack)
		}
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// isCheckViolation reports whether the error is a Postgres CHECK constraint failure
// (sqlstate 23514).
func isCheckViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23514"
	}
	return false
}
