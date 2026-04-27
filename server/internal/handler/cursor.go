package handler

// SPEC: §6.1 #3, §22 M-PR#3 — Story 1.4 read portion. Shared cursor + limit
// helpers for the team-app reconciler/autofill read endpoints. Cursor format
// is opaque to clients: <RFC3339Nano timestamp>:<UUID>. Tuple ordering on
// (timestamp, id) is what makes pagination correct under concurrent writes
// when multiple rows share the same timestamp.

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
)

// errInvalidCursor is returned by parseCursor when the input cannot be split
// into a valid <timestamp>:<uuid> pair.
var errInvalidCursor = errors.New("invalid_cursor")

// parseCursor decodes a cursor string of the form "<RFC3339Nano>:<UUID>".
// Empty input returns zero values with no error (signals "first page").
// The split is on the LAST colon because RFC3339Nano timestamps embed colons
// in both the time portion (HH:MM:SS) and the timezone offset (+00:00); UUIDs
// only contain hyphens, so splitting on the last colon is unambiguous.
func parseCursor(s string) (time.Time, pgtype.UUID, error) {
	if s == "" {
		return time.Time{}, pgtype.UUID{}, nil
	}
	idx := strings.LastIndex(s, ":")
	if idx <= 0 || idx == len(s)-1 {
		return time.Time{}, pgtype.UUID{}, errInvalidCursor
	}
	tsPart := s[:idx]
	idPart := s[idx+1:]
	t, err := time.Parse(time.RFC3339Nano, tsPart)
	if err != nil {
		return time.Time{}, pgtype.UUID{}, errInvalidCursor
	}
	var u pgtype.UUID
	if err := u.Scan(idPart); err != nil || !u.Valid {
		return time.Time{}, pgtype.UUID{}, errInvalidCursor
	}
	return t, u, nil
}

// encodeCursor renders the (timestamp, id) pair as the opaque cursor string
// returned to the caller as next_cursor.
func encodeCursor(t time.Time, id pgtype.UUID) string {
	return t.UTC().Format(time.RFC3339Nano) + ":" + util.UUIDToString(id)
}

// parseLimit returns a strict-validated page size. Empty → defaultN; any
// other input must parse as a positive integer ≤ maxN. Programmatic callers
// (the standalone reconciler) get explicit 400 errors instead of silent
// clamping so bugs surface immediately.
func parseLimit(raw string, defaultN, maxN int) (int, error) {
	if raw == "" {
		return defaultN, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("invalid_limit")
	}
	if n < 1 || n > maxN {
		return 0, errors.New("invalid_limit")
	}
	return n, nil
}

// writeInvalidCursor emits the canonical 400 response specified by AC #3.
func writeInvalidCursor(w http.ResponseWriter) {
	writeError(w, http.StatusBadRequest, "invalid_cursor")
}
