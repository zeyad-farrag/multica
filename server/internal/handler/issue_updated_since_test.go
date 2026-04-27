package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// seedIssueAt inserts an issue and force-stamps updated_at to the given time
// so tests can deterministically reproduce equal-timestamp tie-break cases.
func seedIssueAt(t *testing.T, wsID string, title string, updatedAt time.Time, number int32) string {
	t.Helper()
	var id string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO issue (workspace_id, title, status, priority, creator_id, creator_type, number, position)
		VALUES ($1, $2, 'todo', 'medium', $3, 'member', $4, 0)
		RETURNING id
	`, wsID, title, testUserID, number).Scan(&id); err != nil {
		t.Fatalf("seed issue: %v", err)
	}
	if _, err := testPool.Exec(context.Background(), `
		UPDATE issue SET updated_at = $1, created_at = $1 WHERE id = $2
	`, updatedAt, id); err != nil {
		t.Fatalf("update timestamps: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, id)
	})
	return id
}

type issuesUpdatedSinceResp struct {
	Issues     []map[string]any `json:"issues"`
	NextCursor *string          `json:"next_cursor"`
	Total      int64            `json:"total"`
}

func callListIssuesUpdatedSinceForWorkspace(t *testing.T, wsID, query string) (*httptest.ResponseRecorder, issuesUpdatedSinceResp) {
	t.Helper()
	req := newRequest(http.MethodGet, "/api/workspaces/"+wsID+"/issues?"+query, nil)
	req = withURLParam(req, "id", wsID)
	rr := httptest.NewRecorder()
	testHandler.ListIssuesUpdatedSinceForWorkspace(rr, req)
	var body issuesUpdatedSinceResp
	if rr.Code == http.StatusOK {
		if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
	}
	return rr, body
}

func TestListIssuesUpdatedSince_FiltersBySince(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	base := time.Now().UTC().Truncate(time.Microsecond)
	old := seedIssueAt(t, testWorkspaceID, "tim4-old", base.Add(-2*time.Hour), 7100)
	newer := seedIssueAt(t, testWorkspaceID, "tim4-new", base, 7101)
	newest := seedIssueAt(t, testWorkspaceID, "tim4-newest", base.Add(time.Minute), 7102)

	since := base.Format(time.RFC3339Nano)
	rr, body := callListIssuesUpdatedSinceForWorkspace(t, testWorkspaceID, "updated_since="+since)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	got := map[string]bool{}
	for _, i := range body.Issues {
		got[i["id"].(string)] = true
	}
	if got[old] {
		t.Errorf("expected old row excluded, but it was returned")
	}
	if !got[newer] || !got[newest] {
		t.Errorf("expected new and newest included, got %v", got)
	}
}

func TestListIssuesUpdatedSince_StableTieBreak(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	base := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	a := seedIssueAt(t, testWorkspaceID, "tim4-tie-a", base, 7110)
	b := seedIssueAt(t, testWorkspaceID, "tim4-tie-b", base, 7111)

	first, second := a, b
	if a > b {
		first, second = b, a
	}

	since := base.Format(time.RFC3339Nano)
	rr, body := callListIssuesUpdatedSinceForWorkspace(t, testWorkspaceID,
		fmt.Sprintf("updated_since=%s&limit=1", since))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if len(body.Issues) != 1 || body.Issues[0]["id"] != first {
		t.Fatalf("expected first page = [%s], got %v", first, body.Issues)
	}
	if body.NextCursor == nil {
		t.Fatalf("expected next_cursor to be set when more rows exist")
	}

	rr2, body2 := callListIssuesUpdatedSinceForWorkspace(t, testWorkspaceID,
		fmt.Sprintf("updated_since=%s&limit=1&cursor=%s", since, *body.NextCursor))
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200 page2, got %d", rr2.Code)
	}
	if len(body2.Issues) != 1 || body2.Issues[0]["id"] != second {
		t.Fatalf("expected second page = [%s] strictly after, got %v", second, body2.Issues)
	}
}

func TestListIssuesUpdatedSince_NextCursorOnlyWhenMore(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	base := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Microsecond)
	seedIssueAt(t, testWorkspaceID, "tim4-final-a", base, 7120)
	seedIssueAt(t, testWorkspaceID, "tim4-final-b", base.Add(time.Second), 7121)

	since := base.Format(time.RFC3339Nano)
	rr, body := callListIssuesUpdatedSinceForWorkspace(t, testWorkspaceID,
		fmt.Sprintf("updated_since=%s&limit=10", since))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if body.NextCursor != nil {
		t.Errorf("expected next_cursor=nil when no more pages, got %v", *body.NextCursor)
	}
}

func TestListIssuesUpdatedSince_LimitTooLarge(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	rr, _ := callListIssuesUpdatedSinceForWorkspace(t, testWorkspaceID,
		"updated_since=2026-01-01T00:00:00Z&limit=1001")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for limit=1001, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestListIssuesUpdatedSince_MalformedSince(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	rr, _ := callListIssuesUpdatedSinceForWorkspace(t, testWorkspaceID, "updated_since=not-rfc3339")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestListIssuesUpdatedSince_MissingSince(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	rr, _ := callListIssuesUpdatedSinceForWorkspace(t, testWorkspaceID, "")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when updated_since missing, got %d", rr.Code)
	}
}

func TestListIssuesUpdatedSince_MalformedCursor(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	rr, _ := callListIssuesUpdatedSinceForWorkspace(t, testWorkspaceID,
		"updated_since=2026-01-01T00:00:00Z&cursor=garbage")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed cursor, got %d", rr.Code)
	}
	var body map[string]string
	json.NewDecoder(rr.Body).Decode(&body)
	if body["error"] != "invalid_cursor" {
		t.Errorf("expected error=invalid_cursor, got %q", body["error"])
	}
}

// Regression: bare /api/issues (no updated_since) must remain byte-for-byte
// identical to the legacy header-driven endpoint. AC-2.
func TestListIssues_LegacyShapeUnchanged(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	seedIssueAt(t, testWorkspaceID, "tim4-legacy", time.Now().UTC(), 7130)

	req := newRequest(http.MethodGet, "/api/issues", nil)
	rr := httptest.NewRecorder()
	testHandler.ListIssues(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["issues"]; !ok {
		t.Errorf("legacy response missing 'issues' key")
	}
	if _, ok := body["total"]; !ok {
		t.Errorf("legacy response missing 'total' key")
	}
	if _, ok := body["next_cursor"]; ok {
		t.Errorf("legacy response should NOT include next_cursor (cursor path is workspace-scoped only)")
	}
}
