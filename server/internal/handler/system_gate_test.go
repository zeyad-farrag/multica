package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	teamgate "github.com/multica-ai/multica/server/internal/multica"
)

func createSystemGateIssue(t *testing.T, title string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	if err := testPool.QueryRow(ctx, `
		WITH next_number AS (
			SELECT COALESCE(MAX(number), 0) + 1 AS n
			FROM issue
			WHERE workspace_id = $1
		)
		INSERT INTO issue (workspace_id, title, status, priority, creator_type, creator_id, number)
		SELECT $1, $2, 'backlog', 'none', 'member', $3, n FROM next_number
		RETURNING id
	`, testWorkspaceID, title, testUserID).Scan(&id); err != nil {
		t.Fatalf("create issue: %v", err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, id) })
	return id
}

func withGateClient(t *testing.T, server *httptest.Server) {
	t.Helper()
	original := testHandler.GateClient
	testHandler.GateClient = teamgate.NewGateClient(server.URL, "test-secret")
	t.Cleanup(func() { testHandler.GateClient = original })
}

func TestTeamAppGateFailOpenSingleUpdateAuditsAndCommits(t *testing.T) {
	issueID := createSystemGateIssue(t, "gate fail-open single")
	gate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(750 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer gate.Close()
	withGateClient(t, gate)

	w := httptest.NewRecorder()
	req := newRequest("PATCH", "/api/issues/"+issueID+"?workspace_id="+testWorkspaceID, map[string]any{
		"estimate_minutes": 45,
	})
	req = withURLParam(req, "id", issueID)
	testHandler.UpdateIssue(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpdateIssue expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var estimate int
	if err := testPool.QueryRow(context.Background(), `SELECT estimate_minutes FROM issue WHERE id = $1`, issueID).Scan(&estimate); err != nil {
		t.Fatalf("read estimate: %v", err)
	}
	if estimate != 45 {
		t.Fatalf("expected estimate 45, got %d", estimate)
	}
	var count int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM activity_log WHERE issue_id = $1 AND action = 'gate_bypassed'`, issueID).Scan(&count); err != nil {
		t.Fatalf("count audit rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one gate_bypassed row, got %d", count)
	}
}

func TestTeamAppGateBatchFailOpenAuditsEachIssue(t *testing.T) {
	var calls atomic.Int32
	gate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		time.Sleep(750 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer gate.Close()
	withGateClient(t, gate)

	ids := make([]string, 5)
	for i := range ids {
		ids[i] = createSystemGateIssue(t, "gate fail-open batch")
	}
	w := httptest.NewRecorder()
	req := newRequest("PATCH", "/api/issues/batch?workspace_id="+testWorkspaceID, map[string]any{
		"issue_ids": ids,
		"updates":   map[string]any{"estimate_minutes": 30},
	})
	testHandler.BatchUpdateIssues(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("BatchUpdateIssues expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var count int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM activity_log WHERE issue_id = ANY($1::uuid[]) AND action = 'gate_bypassed'`, ids).Scan(&count); err != nil {
		t.Fatalf("count audit rows: %v", err)
	}
	if count != 5 {
		t.Fatalf("expected five gate_bypassed rows, got %d", count)
	}
	if got := calls.Load(); got != 5 {
		t.Fatalf("expected five gate calls, got %d", got)
	}
}

func TestTeamAppGateBatchDenyRollsBackAndPreservesBody(t *testing.T) {
	var calls atomic.Int32
	gate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		if call == 3 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"capacity conflict","code":"over_capacity"}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer gate.Close()
	withGateClient(t, gate)

	ids := make([]string, 5)
	for i := range ids {
		ids[i] = createSystemGateIssue(t, "gate deny batch")
	}
	w := httptest.NewRecorder()
	req := newRequest("PATCH", "/api/issues/batch?workspace_id="+testWorkspaceID, map[string]any{
		"issue_ids": ids,
		"updates":   map[string]any{"estimate_minutes": 60},
	})
	testHandler.BatchUpdateIssues(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("BatchUpdateIssues expected 409, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["code"] != "over_capacity" || int(body["batch_index"].(float64)) != 2 || int(body["batch_size"].(float64)) != 5 {
		t.Fatalf("unexpected conflict body: %#v", body)
	}
	var changed int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM issue WHERE id = ANY($1::uuid[]) AND estimate_minutes IS NOT NULL`, ids).Scan(&changed); err != nil {
		t.Fatalf("count changed issues: %v", err)
	}
	if changed != 0 {
		t.Fatalf("expected rollback with no changed issues, got %d", changed)
	}
}

func TestTeamAppGateTitleOnlyBatchSkipsGate(t *testing.T) {
	var calls atomic.Int32
	gate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusConflict)
	}))
	defer gate.Close()
	withGateClient(t, gate)

	ids := []string{createSystemGateIssue(t, "title-only gate skip")}
	w := httptest.NewRecorder()
	req := newRequest("PATCH", "/api/issues/batch?workspace_id="+testWorkspaceID, map[string]any{
		"issue_ids": ids,
		"updates":   map[string]any{"title": "title changed without gate"},
	})
	testHandler.BatchUpdateIssues(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("BatchUpdateIssues expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("expected no gate calls for title-only batch, got %d", got)
	}
}

func TestSystemInboxEndpointValidation(t *testing.T) {
	base := map[string]any{
		"workspace_id":   testWorkspaceID,
		"recipient_type": "member",
		"recipient_id":   testUserID,
		"type":           "approval_requested",
		"severity":       "info",
		"issue_id":       nil,
		"title":          "System inbox test",
		"body":           "Body",
		"actor_type":     "system",
		"actor_id":       nil,
		"details":        map[string]any{},
	}

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/system/inbox", base)
	testHandler.SystemCreateInboxItem(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("SystemCreateInboxItem valid body expected 200, got %d: %s", w.Code, w.Body.String())
	}

	delete(base, "title")
	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/system/inbox", base)
	testHandler.SystemCreateInboxItem(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("SystemCreateInboxItem missing key expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
