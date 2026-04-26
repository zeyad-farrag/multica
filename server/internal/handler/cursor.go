package handler

// SPEC: §6.1 #3/#5/#6 — M-PR#3 read-portion cursor + limit helpers (Story 1.4 / TIM-9).
// Shared by ListIssuesUpdatedSinceForWorkspace, ListCommentsForBackfill,
// and ListWorkspaceActivity. Cursor format is <RFC3339Nano>:<UUID>; opaque
// to the caller. parseLimit is strict — malformed or out-of-range values
// surface as 400 instead of being silently coerced.

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

var (
	errInvalidCursor = errors.New("invalid_cursor")
	errInvalidLimit  = errors.New("invalid_limit")
)

// encodeCursor produces the opaque <RFC3339Nano>:<UUID> string used by
// the workspace-scoped read endpoints. The UUID arg is taken in its
// canonical string form so callers can pass uuidToString(row.ID) directly.
func encodeCursor(t time.Time, id string) string {
	return t.UTC().Format(time.RFC3339Nano) + ":" + id
}

// parseCursor decodes a cursor query parameter. Empty string returns
// present=false with no error (first page). Non-empty input must split on
// the LAST ":" because RFC3339 timestamps contain colons. Malformed input
// returns errInvalidCursor.
func parseCursor(s string) (ts pgtype.Timestamptz, id pgtype.UUID, present bool, err error) {
	if s == "" {
		return pgtype.Timestamptz{}, pgtype.UUID{}, false, nil
	}
	idx := strings.LastIndex(s, ":")
	if idx <= 0 || idx == len(s)-1 {
		return pgtype.Timestamptz{}, pgtype.UUID{}, false, errInvalidCursor
	}
	tsRaw := s[:idx]
	idRaw := s[idx+1:]
	t, perr := time.Parse(time.RFC3339Nano, tsRaw)
	if perr != nil {
		return pgtype.Timestamptz{}, pgtype.UUID{}, false, errInvalidCursor
	}
	var u pgtype.UUID
	if scanErr := u.Scan(idRaw); scanErr != nil || !u.Valid {
		return pgtype.Timestamptz{}, pgtype.UUID{}, false, errInvalidCursor
	}
	return pgtype.Timestamptz{Time: t, Valid: true}, u, true, nil
}

// parseLimit pulls the "limit" query param. Empty returns def. Non-numeric,
// <1, or > max returns errInvalidLimit. Strict by design — programmatic
// callers (the team-app reconciler) deserve loud failures over silent
// coercion.
func parseLimit(r *http.Request, def, max int) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > max {
		return 0, errInvalidLimit
	}
	return n, nil
}
