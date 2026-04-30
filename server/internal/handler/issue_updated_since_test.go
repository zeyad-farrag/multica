package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func seedUpdatedSinceIssue(t *testing.T, id, title string, number int, updatedAt time.Time) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `
		INSERT INTO issue (
			id, workspace_id, title, status, priority, creator_type, creator_id,
			position, number, created_at, updated_at
		) VALUES ($1, $2, $3, 'todo', 'none', 'member', $4, $5, $6, $7, $7)
	`, id, testWorkspaceID, title, testUserID, float64(number), number, updatedAt)
	if err != nil {
		t.Fatalf("seed issue: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, id)
	})
}

func TestListIssuesUpdatedSinceCursorStrictAfter(t *testing.T) {
	sameTS := time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC)
	oldTS := sameTS.Add(-time.Hour)
	seedUpdatedSinceIssue(t, "00000000-0000-0000-0000-000000000011", "old", 9101, oldTS)
	seedUpdatedSinceIssue(t, "00000000-0000-0000-0000-000000000012", "first", 9102, sameTS)
	seedUpdatedSinceIssue(t, "00000000-0000-0000-0000-000000000013", "second", 9103, sameTS)

	w := httptest.NewRecorder()
	req := newRequest("GET", "/api/workspaces/"+testWorkspaceID+"/issues?updated_since="+sameTS.Format(time.RFC3339)+"&limit=1", nil)
	req = withURLParam(req, "id", testWorkspaceID)
	testHandler.ListIssuesUpdatedSinceForWorkspace(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first page: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var first struct {
		Issues     []IssueResponse `json:"issues"`
		NextCursor *string         `json:"next_cursor"`
		Total      int64           `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&first); err != nil {
		t.Fatalf("decode first page: %v", err)
	}
	if len(first.Issues) != 1 || first.Issues[0].Title != "first" {
		t.Fatalf("unexpected first page: %+v", first.Issues)
	}
	if first.NextCursor == nil {
		t.Fatal("expected next_cursor on first page")
	}
	if first.Total != 2 {
		t.Fatalf("total = %d, want 2", first.Total)
	}

	w = httptest.NewRecorder()
	req = newRequest("GET", "/api/workspaces/"+testWorkspaceID+"/issues?updated_since="+sameTS.Format(time.RFC3339)+"&limit=1&cursor="+*first.NextCursor, nil)
	req = withURLParam(req, "id", testWorkspaceID)
	testHandler.ListIssuesUpdatedSinceForWorkspace(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("second page: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var second struct {
		Issues []IssueResponse `json:"issues"`
	}
	if err := json.NewDecoder(w.Body).Decode(&second); err != nil {
		t.Fatalf("decode second page: %v", err)
	}
	if len(second.Issues) != 1 || second.Issues[0].Title != "second" {
		t.Fatalf("unexpected second page: %+v", second.Issues)
	}
}

func TestListIssuesUpdatedSinceRejectsInvalidLimit(t *testing.T) {
	w := httptest.NewRecorder()
	req := newRequest("GET", "/api/workspaces/"+testWorkspaceID+"/issues?updated_since=2026-04-30T10:00:00Z&limit=1001", nil)
	req = withURLParam(req, "id", testWorkspaceID)
	testHandler.ListIssuesUpdatedSinceForWorkspace(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListIssuesLegacyShapeUnchanged(t *testing.T) {
	w := httptest.NewRecorder()
	req := newRequest("GET", "/api/issues", nil)
	testHandler.ListIssues(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode legacy body: %v", err)
	}
	if _, ok := body["issues"]; !ok {
		t.Fatal("legacy response missing issues")
	}
	if _, ok := body["total"]; !ok {
		t.Fatal("legacy response missing total")
	}
	if _, ok := body["next_cursor"]; ok {
		t.Fatal("legacy response unexpectedly included next_cursor")
	}
}
