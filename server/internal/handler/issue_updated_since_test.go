package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// seedIssueAt inserts an issue and force-sets its updated_at to the given
// time. Returns the issue UUID. Cleanup is registered via t.Cleanup.
func seedIssueAt(t *testing.T, title string, updatedAt time.Time) string {
	t.Helper()
	ctx := context.Background()
	var id string
	err := testPool.QueryRow(ctx, `
		INSERT INTO issue (workspace_id, title, status, priority, creator_type, creator_id, position, number)
		VALUES ($1, $2, 'todo', 'medium', 'member', $3, 0,
		        (SELECT COALESCE(MAX(number), 0) + 1 FROM issue WHERE workspace_id = $1))
		RETURNING id
	`, testWorkspaceID, title, testUserID).Scan(&id)
	if err != nil {
		t.Fatalf("seed issue: %v", err)
	}
	if _, err := testPool.Exec(ctx, `UPDATE issue SET updated_at = $1 WHERE id = $2`, updatedAt, id); err != nil {
		t.Fatalf("set updated_at: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, id) })
	return id
}

func callListIssuesUpdatedSince(t *testing.T, qs url.Values) (int, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/workspaces/"+testWorkspaceID+"/issues?"+qs.Encode(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", testWorkspaceID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	testHandler.ListIssuesUpdatedSinceForWorkspace(w, req)
	var body map[string]any
	if w.Body.Len() > 0 {
		_ = json.NewDecoder(w.Body).Decode(&body)
	}
	return w.Code, body
}

// TestIssuesUpdatedSinceFilter verifies that updated_since returns only issues
// whose updated_at is >= the cutoff (AC #1, AC #11).
func TestIssuesUpdatedSinceFilter(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	// One issue updated before the cutoff (excluded), two after (included).
	seedIssueAt(t, "before-cutoff", now.Add(-2*time.Hour))
	idA := seedIssueAt(t, "after-A", now.Add(time.Hour))
	idB := seedIssueAt(t, "after-B", now.Add(2*time.Hour))

	qs := url.Values{}
	qs.Set("updated_since", now.Format(time.RFC3339))
	code, body := callListIssuesUpdatedSince(t, qs)
	if code != 200 {
		t.Fatalf("status=%d body=%v", code, body)
	}
	issues, _ := body["issues"].([]any)
	gotIDs := map[string]bool{}
	for _, raw := range issues {
		entry := raw.(map[string]any)
		gotIDs[entry["id"].(string)] = true
	}
	if !gotIDs[idA] || !gotIDs[idB] {
		t.Fatalf("expected ids %s and %s in response, got %v", idA, idB, gotIDs)
	}
	for _, raw := range issues {
		entry := raw.(map[string]any)
		updatedAt, _ := time.Parse(time.RFC3339Nano, entry["updated_at"].(string))
		if updatedAt.Before(now) {
			t.Fatalf("issue %s slipped through with updated_at=%v < cutoff=%v",
				entry["id"], updatedAt, now)
		}
	}
}

// TestIssuesUpdatedSinceCursorTieBreak verifies that the strict-after cursor
// tuple (updated_at, id) excludes the cursor row even when timestamps tie
// (AC #3, AC #11). This is the case ORDER BY single-column would mishandle.
func TestIssuesUpdatedSinceCursorTieBreak(t *testing.T) {
	tieAt := time.Now().UTC().Truncate(time.Microsecond)
	// Insert three issues with identical updated_at — paginate by id.
	id1 := seedIssueAt(t, "tie-1", tieAt)
	id2 := seedIssueAt(t, "tie-2", tieAt)
	id3 := seedIssueAt(t, "tie-3", tieAt)
	// Sort the IDs so we can compute the expected order (UUID ASC).
	want := []string{id1, id2, id3}
	for i := 1; i < len(want); i++ {
		for j := i; j > 0 && want[j] < want[j-1]; j-- {
			want[j], want[j-1] = want[j-1], want[j]
		}
	}

	// Page 1: limit=1, no cursor.
	qs := url.Values{}
	qs.Set("updated_since", tieAt.Format(time.RFC3339))
	qs.Set("limit", "1")
	code, body := callListIssuesUpdatedSince(t, qs)
	if code != 200 {
		t.Fatalf("page1 status=%d body=%v", code, body)
	}
	page1 := body["issues"].([]any)
	if len(page1) != 1 {
		t.Fatalf("page1: expected 1, got %d", len(page1))
	}
	got1 := page1[0].(map[string]any)["id"].(string)
	if got1 != want[0] {
		t.Fatalf("page1 id=%s want=%s", got1, want[0])
	}
	cursor, _ := body["next_cursor"].(string)
	if cursor == "" {
		t.Fatal("page1: expected next_cursor")
	}

	// Page 2: cursor advances past page1's last row.
	qs.Set("cursor", cursor)
	code, body = callListIssuesUpdatedSince(t, qs)
	if code != 200 {
		t.Fatalf("page2 status=%d body=%v", code, body)
	}
	page2 := body["issues"].([]any)
	if len(page2) != 1 {
		t.Fatalf("page2: expected 1, got %d", len(page2))
	}
	got2 := page2[0].(map[string]any)["id"].(string)
	if got2 != want[1] {
		t.Fatalf("page2 id=%s want=%s (tie-break must exclude page1's row)", got2, want[1])
	}
}

func TestIssuesUpdatedSinceMissingParam(t *testing.T) {
	code, body := callListIssuesUpdatedSince(t, url.Values{})
	if code != 400 {
		t.Fatalf("expected 400 for missing updated_since, got %d body=%v", code, body)
	}
}

func TestIssuesUpdatedSinceLimitTooLarge(t *testing.T) {
	qs := url.Values{}
	qs.Set("updated_since", time.Now().UTC().Format(time.RFC3339))
	qs.Set("limit", "1001")
	code, body := callListIssuesUpdatedSince(t, qs)
	if code != 400 {
		t.Fatalf("expected 400 for limit=1001, got %d body=%v", code, body)
	}
}

func TestIssuesUpdatedSinceInvalidCursor(t *testing.T) {
	qs := url.Values{}
	qs.Set("updated_since", time.Now().UTC().Format(time.RFC3339))
	qs.Set("cursor", "garbage")
	code, body := callListIssuesUpdatedSince(t, qs)
	if code != 400 {
		t.Fatalf("expected 400 for invalid cursor, got %d body=%v", code, body)
	}
}

// TestListIssuesUnchangedWithoutUpdatedSince is the AC #2 regression: the
// legacy /api/issues handler must remain byte-for-byte identical when
// updated_since is absent. We hit ListIssues directly (the legacy handler)
// and assert the response shape.
func TestListIssuesUnchangedWithoutUpdatedSince(t *testing.T) {
	w := httptest.NewRecorder()
	req := newRequest("GET", "/api/issues?workspace_id="+testWorkspaceID, nil)
	testHandler.ListIssues(w, req)
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["issues"]; !ok {
		t.Fatal("missing issues key in legacy response")
	}
	if _, ok := resp["total"]; !ok {
		t.Fatal("missing total key in legacy response")
	}
	// Legacy response must NOT include next_cursor — that is cursor-path only.
	if _, found := resp["next_cursor"]; found {
		t.Fatal("legacy /api/issues should not include next_cursor (AC #2 byte-for-byte identical)")
	}
}
