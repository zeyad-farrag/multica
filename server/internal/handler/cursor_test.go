package handler

import (
	"errors"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCursorRoundTrip(t *testing.T) {
	id := "11111111-2222-3333-4444-555555555555"
	now := time.Date(2026, 4, 26, 10, 30, 45, 123456789, time.UTC)
	encoded := encodeCursor(now, id)
	ts, gotID, present, err := parseCursor(encoded)
	if err != nil {
		t.Fatalf("parseCursor: %v", err)
	}
	if !present {
		t.Fatalf("expected present=true")
	}
	if !ts.Time.Equal(now) {
		t.Fatalf("ts mismatch: got %v, want %v", ts.Time, now)
	}
	if uuidToString(gotID) != id {
		t.Fatalf("id mismatch: got %s, want %s", uuidToString(gotID), id)
	}
}

func TestCursorEmpty(t *testing.T) {
	_, _, present, err := parseCursor("")
	if err != nil {
		t.Fatalf("parseCursor empty: unexpected error %v", err)
	}
	if present {
		t.Fatal("expected present=false for empty cursor")
	}
}

func TestCursorMalformed(t *testing.T) {
	cases := []string{
		"not-a-cursor",
		"2026-04-26T10:30:45Z",                   // no UUID
		":11111111-2222-3333-4444-555555555555", // empty timestamp
		"2026-04-26T10:30:45Z:",                  // empty UUID
		"2026-04-26T10:30:45Z:not-a-uuid",
		"garbage-ts:11111111-2222-3333-4444-555555555555",
	}
	for _, c := range cases {
		_, _, _, err := parseCursor(c)
		if !errors.Is(err, errInvalidCursor) {
			t.Errorf("parseCursor(%q): expected errInvalidCursor, got %v", c, err)
		}
	}
}

func TestParseLimit(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		def     int
		max     int
		want    int
		wantErr bool
	}{
		{"default", "", 200, 1000, 200, false},
		{"valid", "500", 200, 1000, 500, false},
		{"max boundary", "1000", 200, 1000, 1000, false},
		{"over max", "1001", 200, 1000, 0, true},
		{"zero", "0", 200, 1000, 0, true},
		{"negative", "-1", 200, 1000, 0, true},
		{"non-numeric", "many", 200, 1000, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/?limit="+tt.raw, nil)
			if tt.raw == "" {
				req = httptest.NewRequest("GET", "/", nil)
			}
			n, err := parseLimit(req, tt.def, tt.max)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.wantErr && n != tt.want {
				t.Fatalf("got %d, want %d", n, tt.want)
			}
		})
	}
}
