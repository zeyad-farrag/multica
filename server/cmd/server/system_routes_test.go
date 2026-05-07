package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/realtime"
)

func newSystemRouteTestRouter(t *testing.T, secret string) http.Handler {
	t.Helper()
	t.Setenv("TEAM_APP_URL", "http://team-app.test")
	t.Setenv("TEAM_APP_SHARED_SECRET", secret)
	hub := realtime.NewHub()
	go hub.Run()
	bus := events.New()
	registerListeners(bus, hub)
	return NewRouter(testPool, hub, bus, analytics.NoopClient{}, nil)
}

func TestSystemRoutesRequireSharedSecretHeader(t *testing.T) {
	router := newSystemRouteTestRouter(t, "route-secret")
	paths := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/system/workspaces/" + testWorkspaceID, ""},
		{http.MethodGet, "/api/system/workspaces/" + testWorkspaceID + "/issues", ""},
		{http.MethodGet, "/api/system/workspaces/" + testWorkspaceID + "/comments", ""},
		{http.MethodGet, "/api/system/workspaces/" + testWorkspaceID + "/activity", ""},
		{http.MethodGet, "/api/system/workspaces/" + testWorkspaceID + "/members", ""},
		{http.MethodPost, "/api/system/inbox", `{}`},
	}

	for _, tc := range paths {
		req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s without shared secret: expected 401, got %d: %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestSystemInboxRouteAuthAndCollision(t *testing.T) {
	router := newSystemRouteTestRouter(t, "route-secret")
	validBody := `{
		"workspace_id":"` + testWorkspaceID + `",
		"recipient_type":"member",
		"recipient_id":"` + testUserID + `",
		"type":"approval_requested",
		"severity":"info",
		"issue_id":null,
		"title":"route inbox test",
		"body":"body",
		"actor_type":"system",
		"actor_id":null,
		"details":{}
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/system/inbox", bytes.NewBufferString(validBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Team-App-Secret", "wrong")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong shared secret: expected 401, got %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/system/inbox", bytes.NewBufferString(validBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Team-App-Secret", "route-secret")
	req.AddCookie(&http.Cookie{Name: "token", Value: "malformed"})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("correct shared secret with malformed PAT cookie: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	missingTitle := `{
		"workspace_id":"` + testWorkspaceID + `",
		"recipient_type":"member",
		"recipient_id":"` + testUserID + `",
		"type":"approval_requested",
		"severity":"info",
		"issue_id":null,
		"body":"body",
		"actor_type":"system",
		"actor_id":null,
		"details":{}
	}`
	req = httptest.NewRequest(http.MethodPost, "/api/system/inbox", bytes.NewBufferString(missingTitle))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Team-App-Secret", "route-secret")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing required key: expected 400, got %d: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/inbox", bytes.NewBufferString(validBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Team-App-Secret", "route-secret")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("/api/inbox path collision without PAT: expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}
