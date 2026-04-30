package handler

import (
	"testing"
	"time"
)

func TestCursorEncodeDecodeRoundTrip(t *testing.T) {
	ts := time.Date(2026, 4, 30, 12, 13, 14, 123456789, time.UTC)
	id := parseUUID("00000000-0000-0000-0000-000000000123")

	gotTS, gotID, err := parseCursor(encodeCursor(ts, id))
	if err != nil {
		t.Fatalf("parseCursor: %v", err)
	}
	if !gotTS.Equal(ts) {
		t.Fatalf("timestamp mismatch: got %s want %s", gotTS, ts)
	}
	if uuidToString(gotID) != uuidToString(id) {
		t.Fatalf("id mismatch: got %s want %s", uuidToString(gotID), uuidToString(id))
	}
}

func TestParseCursorRejectsMalformed(t *testing.T) {
	for _, raw := range []string{"", "not-a-cursor", "2026-04-30T12:00:00Z:not-a-uuid"} {
		if _, _, err := parseCursor(raw); err == nil {
			t.Fatalf("expected malformed cursor %q to fail", raw)
		}
	}
}
