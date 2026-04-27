package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNewRouter_Healthz_Returns200OKWithBody covers AC4: the team-app server
// listens on :8080 and exposes /healthz. The compose stack and any downstream
// liveness probe (Story 1.6 migration runner, Story 1.9 system-PAT boot
// validation, future ingress) all depend on /healthz returning 200 with a
// non-empty body. A silent regression here breaks boot for every consumer.
//
// pool is nil because /healthz must not require database connectivity — the
// healthcheck deliberately answers before pgxpool is in scope so the orchestrator
// can mark the container live during DB warm-up.
func TestNewRouter_Healthz_Returns200OKWithBody(t *testing.T) {
	r := newRouter(nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("GET /healthz status = %d; want %d", got, want)
	}

	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if got := strings.TrimSpace(string(body)); got != "ok" {
		t.Fatalf("GET /healthz body = %q; want %q", got, "ok")
	}
}

// TestNewRouter_UnknownPath_Returns404 documents the contract that the
// scaffold router rejects unknown paths cleanly. The /api/v1 group is
// reserved for later stories and currently has zero handlers, so any path
// under it must 404 — not 500, not panic. Future per-domain mounts (Stories
// 1.5+) replace this expectation domain by domain.
func TestNewRouter_UnknownPath_Returns404(t *testing.T) {
	r := newRouter(nil)

	cases := []string{
		"/",
		"/api/v1",
		"/api/v1/orgs",
		"/gates/issue-update",
	}
	for _, path := range cases {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if got, want := rec.Code, http.StatusNotFound; got != want {
				t.Fatalf("GET %s status = %d; want %d", path, got, want)
			}
		})
	}
}

// TestNewRouter_HealthzMethodNotAllowed_NonGet documents that /healthz only
// accepts GET. Chi returns 405 Method Not Allowed for the path when registered
// with r.Get; this guards the boot/health contract from accidental method
// drift in later stories.
func TestNewRouter_HealthzMethodNotAllowed_NonGet(t *testing.T) {
	r := newRouter(nil)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/healthz", nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code == http.StatusOK {
				t.Fatalf("%s /healthz status = 200; want non-2xx", method)
			}
		})
	}
}
