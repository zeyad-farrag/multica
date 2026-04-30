package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

var errInvalidCursor = errors.New("invalid cursor")

func parseCursor(s string) (time.Time, pgtype.UUID, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return time.Time{}, pgtype.UUID{}, errInvalidCursor
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, pgtype.UUID{}, errInvalidCursor
	}
	var id pgtype.UUID
	if err := id.Scan(parts[1]); err != nil || !id.Valid {
		return time.Time{}, pgtype.UUID{}, errInvalidCursor
	}
	return t, id, nil
}

func encodeCursor(t time.Time, id pgtype.UUID) string {
	return t.Format(time.RFC3339Nano) + ":" + uuidToString(id)
}

func parseLimit(r *http.Request, defaultLimit, maxLimit int32) (int32, bool) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return defaultLimit, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > int(maxLimit) {
		return 0, false
	}
	return int32(n), true
}

func cursorTimestamptz(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func parseRequiredUUID(s string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(s); err != nil || !id.Valid {
		return pgtype.UUID{}, false
	}
	return id, true
}
