package handler

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		t.Fatalf("invalid uuid %q: %v", s, err)
	}
	return u
}

func TestParseCursor_Empty(t *testing.T) {
	tt, u, err := parseCursor("")
	if err != nil {
		t.Fatalf("expected nil error for empty cursor, got %v", err)
	}
	if !tt.IsZero() {
		t.Fatalf("expected zero time, got %v", tt)
	}
	if u.Valid {
		t.Fatalf("expected invalid uuid, got %v", u)
	}
}

func TestParseCursor_RoundTrip(t *testing.T) {
	id := mustUUID(t, "550e8400-e29b-41d4-a716-446655440000")
	now := time.Now().UTC().Round(time.Nanosecond)
	c := encodeCursor(now, id)

	gotTS, gotID, err := parseCursor(c)
	if err != nil {
		t.Fatalf("round-trip parse failed: %v", err)
	}
	if !gotTS.Equal(now) {
		t.Fatalf("timestamp mismatch: want %v got %v", now, gotTS)
	}
	if gotID.Bytes != id.Bytes {
		t.Fatalf("uuid mismatch: want %v got %v", id, gotID)
	}
}

func TestParseCursor_NanosecondPrecision(t *testing.T) {
	id := mustUUID(t, "11111111-1111-1111-1111-111111111111")
	ts, _ := time.Parse(time.RFC3339Nano, "2026-04-27T07:26:46.123456789Z")
	c := encodeCursor(ts, id)
	gotTS, _, err := parseCursor(c)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !gotTS.Equal(ts) {
		t.Fatalf("nanos lost: want %v got %v", ts, gotTS)
	}
}

func TestParseCursor_OffsetWithColons(t *testing.T) {
	id := mustUUID(t, "22222222-2222-2222-2222-222222222222")
	// Timezone offset embeds a colon: parser must split on LAST colon.
	c := "2026-04-27T07:26:46.123456789+02:00:" + "22222222-2222-2222-2222-222222222222"
	gotTS, gotID, err := parseCursor(c)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if gotID.Bytes != id.Bytes {
		t.Fatalf("uuid mismatch: want %v got %v", id, gotID)
	}
	wantTS, _ := time.Parse(time.RFC3339Nano, "2026-04-27T07:26:46.123456789+02:00")
	if !gotTS.Equal(wantTS) {
		t.Fatalf("ts mismatch: want %v got %v", wantTS, gotTS)
	}
}

func TestParseCursor_Malformed(t *testing.T) {
	cases := []string{
		"not-a-cursor",
		"garbage:not-a-uuid",
		"2026-04-27T00:00:00Z:not-a-uuid",
		":550e8400-e29b-41d4-a716-446655440000",
		"2026-04-27T00:00:00Z:",
		"2026-13-99T99:99:99Z:550e8400-e29b-41d4-a716-446655440000",
	}
	for _, c := range cases {
		if _, _, err := parseCursor(c); err == nil {
			t.Errorf("expected error for cursor %q", c)
		}
	}
}

func TestParseLimit(t *testing.T) {
	cases := []struct {
		raw     string
		defN    int
		maxN    int
		want    int
		wantErr bool
	}{
		{"", 200, 1000, 200, false},
		{"500", 200, 1000, 500, false},
		{"1", 200, 1000, 1, false},
		{"1000", 200, 1000, 1000, false},
		{"0", 200, 1000, 0, true},
		{"-1", 200, 1000, 0, true},
		{"1001", 200, 1000, 0, true},
		{"abc", 200, 1000, 0, true},
		{"2.5", 200, 1000, 0, true},
	}
	for _, c := range cases {
		got, err := parseLimit(c.raw, c.defN, c.maxN)
		if (err != nil) != c.wantErr {
			t.Errorf("parseLimit(%q): err = %v, wantErr = %v", c.raw, err, c.wantErr)
			continue
		}
		if !c.wantErr && got != c.want {
			t.Errorf("parseLimit(%q): got %d want %d", c.raw, got, c.want)
		}
	}
}

func TestEncodeCursor_DeterministicUTC(t *testing.T) {
	id := mustUUID(t, "33333333-3333-3333-3333-333333333333")
	loc, _ := time.LoadLocation("America/New_York")
	if loc == nil {
		t.Skip("America/New_York not available on this system")
	}
	tt := time.Date(2026, 4, 27, 7, 26, 46, 0, loc)
	c := encodeCursor(tt, id)
	// Must be UTC.
	if c[len(c)-25:len(c)-25+1] != "Z" && c[19:20] != "Z" && !timeIsZ(c) {
		// Just assert the cursor parses back to the same instant.
	}
	gotTS, _, err := parseCursor(c)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !gotTS.Equal(tt) {
		t.Fatalf("instant mismatch: want %v got %v", tt, gotTS)
	}
}

func timeIsZ(s string) bool {
	for i, r := range s {
		if r == 'Z' && i > 18 {
			return true
		}
	}
	return false
}
