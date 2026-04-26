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

// seedActivityAt inserts an activity_log row tied to the test workspace and
// returns its UUID. Cleanup is automatic.
func seedActivityAt(t *testing.T, action string, details map[string]any, createdAt time.Time) string {
	t.Helper()
	ctx := context.Background()
	// Inject a fresh issue per row so we have a valid issue_id FK target.
	var issueID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO issue (workspace_id, title, status, priority, creator_type, creator_id, position, number)
		VALUES ($1, 'activity-fixture', 'todo', 'medium', 'member', $2, 0,
		        (SELECT COALESCE(MAX(number), 0) + 1 FROM issue WHERE workspace_id = $1))
		RETURNING id
	`, testWorkspaceID, testUserID).Scan(&issueID); err != nil {
		t.Fatalf("seed activity issue: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, issueID) })

	detailsJSON, _ := json.Marshal(details)
	var id string
	err := testPool.QueryRow(ctx, `
		INSERT INTO activity_log (workspace_id, issue_id, actor_type, actor_id, action, details, created_at)
		VALUES ($1, $2, 'member', $3, $4, $5, $6)
		RETURNING id
	`, testWorkspaceID, issueID, testUserID, action, detailsJSON, createdAt).Scan(&id)
	if err != nil {
		t.Fatalf("seed activity: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM activity_log WHERE id = $1`, id) })
	return id
}

func callListWorkspaceActivity(t *testing.T, qs url.Values) (int, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/workspaces/"+testWorkspaceID+"/activity?"+qs.Encode(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", testWorkspaceID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	testHandler.ListWorkspaceActivity(w, req)
	var body map[string]any
	if w.Body.Len() > 0 {
		_ = json.NewDecoder(w.Body).Decode(&body)
	}
	return w.Code, body
}

// TestWorkspaceActivityFilterSinceAction verifies the `since + action`
// narrowing (AC #6, AC #11). Older rows and other-action rows must be
// excluded; details are passed through verbatim.
func TestWorkspaceActivityFilterSinceAction(t *testing.T) {
	since := time.Now().UTC().Truncate(time.Second)
	// Excluded: older than `since`.
	seedActivityAt(t, "gate_bypassed", map[string]any{"outcome": "blocked"}, since.Add(-time.Hour))
	// Excluded: different action.
	seedActivityAt(t, "assignee_changed", map[string]any{"to_id": "x"}, since.Add(time.Hour))
	// Included.
	wantID := seedActivityAt(t, "gate_bypassed", map[string]any{"outcome": "blocked"}, since.Add(2*time.Hour))

	qs := url.Values{}
	qs.Set("since", since.Format(time.RFC3339))
	qs.Set("action", "gate_bypassed")
	code, body := callListWorkspaceActivity(t, qs)
	if code != 200 {
		t.Fatalf("status=%d body=%v", code, body)
	}
	rows, _ := body["activity"].([]any)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d (%v)", len(rows), rows)
	}
	got := rows[0].(map[string]any)
	if got["id"].(string) != wantID {
		t.Fatalf("got id %s, want %s", got["id"], wantID)
	}
	// AC #6: rows with details.outcome='blocked' must not be silently dropped.
	rawDetails, _ := got["details"].(map[string]any)
	if rawDetails == nil || rawDetails["outcome"] != "blocked" {
		t.Fatalf("expected details.outcome=blocked, got %v", got["details"])
	}
}

// TestWorkspaceActivityCursorAdvances verifies pagination across the
// (created_at, id) tuple (AC #6, AC #11).
func TestWorkspaceActivityCursorAdvances(t *testing.T) {
	since := time.Now().UTC().Truncate(time.Second)
	id1 := seedActivityAt(t, "issue_created", nil, since.Add(time.Second))
	id2 := seedActivityAt(t, "issue_created", nil, since.Add(2*time.Second))
	id3 := seedActivityAt(t, "issue_created", nil, since.Add(3*time.Second))

	qs := url.Values{}
	qs.Set("since", since.Format(time.RFC3339))
	qs.Set("action", "issue_created")
	qs.Set("limit", "2")
	code, body := callListWorkspaceActivity(t, qs)
	if code != 200 {
		t.Fatalf("page1 status=%d body=%v", code, body)
	}
	page1, _ := body["activity"].([]any)
	if len(page1) != 2 ||
		page1[0].(map[string]any)["id"].(string) != id1 ||
		page1[1].(map[string]any)["id"].(string) != id2 {
		t.Fatalf("page1 wrong: %v", page1)
	}
	cursor, _ := body["next_cursor"].(string)
	if cursor == "" {
		t.Fatal("page1 expected next_cursor")
	}

	qs.Set("cursor", cursor)
	code, body = callListWorkspaceActivity(t, qs)
	if code != 200 {
		t.Fatalf("page2 status=%d body=%v", code, body)
	}
	page2, _ := body["activity"].([]any)
	if len(page2) != 1 || page2[0].(map[string]any)["id"].(string) != id3 {
		t.Fatalf("page2 expected [%s], got %v", id3, page2)
	}
	if next, _ := body["next_cursor"].(string); next != "" {
		t.Fatalf("page2 expected nil next_cursor, got %s", next)
	}
}

func TestWorkspaceActivityMissingSince(t *testing.T) {
	code, body := callListWorkspaceActivity(t, url.Values{})
	if code != 400 {
		t.Fatalf("expected 400, got %d body=%v", code, body)
	}
}

func TestWorkspaceActivityInvalidActor(t *testing.T) {
	qs := url.Values{}
	qs.Set("since", time.Now().UTC().Format(time.RFC3339))
	qs.Set("actor_id", "not-uuid")
	code, body := callListWorkspaceActivity(t, qs)
	if code != 400 {
		t.Fatalf("expected 400, got %d body=%v", code, body)
	}
}
