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

// seedActivityAt inserts an activity_log row and force-stamps created_at so
// tests can deterministically place it before/after a since boundary.
func seedActivityAt(t *testing.T, wsID, issueID, actorID, action, details string, createdAt time.Time) string {
	t.Helper()
	var id string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO activity_log (workspace_id, issue_id, actor_type, actor_id, action, details)
		VALUES ($1, $2, 'member', $3, $4, $5::jsonb)
		RETURNING id
	`, wsID, issueID, actorID, action, details).Scan(&id); err != nil {
		t.Fatalf("seed activity: %v", err)
	}
	if _, err := testPool.Exec(context.Background(), `
		UPDATE activity_log SET created_at = $1 WHERE id = $2
	`, createdAt, id); err != nil {
		t.Fatalf("update activity timestamp: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM activity_log WHERE id = $1`, id)
	})
	return id
}

type activityResp struct {
	Activity   []map[string]any `json:"activity"`
	NextCursor *string          `json:"next_cursor"`
	Total      int64            `json:"total"`
}

func callListWorkspaceActivity(t *testing.T, wsID, query string) (*httptest.ResponseRecorder, activityResp) {
	t.Helper()
	req := newRequest(http.MethodGet, "/api/workspaces/"+wsID+"/activity?"+query, nil)
	req = withURLParam(req, "id", wsID)
	rr := httptest.NewRecorder()
	testHandler.ListWorkspaceActivity(rr, req)
	var body activityResp
	if rr.Code == http.StatusOK {
		if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return rr, body
}

func TestListWorkspaceActivity_FilterBySinceAndAction(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	issueID := seedIssueAt(t, testWorkspaceID, "tim4-activity-host", time.Now().UTC(), 7300)
	since := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	old := since.Add(-time.Hour)
	in := since.Add(time.Minute)

	// Below boundary — must be excluded.
	seedActivityAt(t, testWorkspaceID, issueID, testUserID, "gate_bypassed", `{"outcome":"blocked"}`, old)
	// Wrong action — must be excluded when action filter is applied.
	seedActivityAt(t, testWorkspaceID, issueID, testUserID, "assignee_changed", `{}`, in)
	// Match.
	matchID := seedActivityAt(t, testWorkspaceID, issueID, testUserID, "gate_bypassed", `{"outcome":"blocked"}`, in)

	rr, body := callListWorkspaceActivity(t, testWorkspaceID,
		fmt.Sprintf("since=%s&action=gate_bypassed", since.Format(time.RFC3339Nano)))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if len(body.Activity) != 1 {
		t.Fatalf("expected 1 row, got %d: %v", len(body.Activity), body.Activity)
	}
	if body.Activity[0]["id"] != matchID {
		t.Errorf("expected id=%s, got %s", matchID, body.Activity[0]["id"])
	}
	// AC-6: details JSON must round-trip.
	details, _ := body.Activity[0]["details"].(map[string]any)
	if details == nil || details["outcome"] != "blocked" {
		t.Errorf("expected details.outcome=blocked, got %v", body.Activity[0]["details"])
	}
}

func TestListWorkspaceActivity_ActorIDNarrowing(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	issueID := seedIssueAt(t, testWorkspaceID, "tim4-actor-host", time.Now().UTC(), 7310)
	since := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	// Create a second user via INSERT.
	var otherUserID string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO "user" (name, email) VALUES ('Other', $1) RETURNING id
	`, fmt.Sprintf("tim4-other-%d@multica.ai", time.Now().UnixNano())).Scan(&otherUserID); err != nil {
		t.Fatalf("seed other user: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM "user" WHERE id = $1`, otherUserID)
	})

	mineID := seedActivityAt(t, testWorkspaceID, issueID, testUserID, "gate_bypassed", `{}`, since.Add(time.Minute))
	seedActivityAt(t, testWorkspaceID, issueID, otherUserID, "gate_bypassed", `{}`, since.Add(2*time.Minute))

	rr, body := callListWorkspaceActivity(t, testWorkspaceID,
		fmt.Sprintf("since=%s&action=gate_bypassed&actor_id=%s",
			since.Format(time.RFC3339Nano), testUserID))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if len(body.Activity) != 1 {
		t.Fatalf("expected 1 row narrowed by actor_id, got %d", len(body.Activity))
	}
	if body.Activity[0]["id"] != mineID {
		t.Errorf("expected mine=%s, got %s", mineID, body.Activity[0]["id"])
	}
}

func TestListWorkspaceActivity_CursorAdvanceOnTie(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	issueID := seedIssueAt(t, testWorkspaceID, "tim4-cursor-host", time.Now().UTC(), 7320)
	since := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
	tied := since.Add(time.Hour)
	a := seedActivityAt(t, testWorkspaceID, issueID, testUserID, "gate_bypassed", `{}`, tied)
	b := seedActivityAt(t, testWorkspaceID, issueID, testUserID, "gate_bypassed", `{}`, tied)

	first, second := a, b
	if a > b {
		first, second = b, a
	}

	rr, body := callListWorkspaceActivity(t, testWorkspaceID,
		fmt.Sprintf("since=%s&action=gate_bypassed&limit=1", since.Format(time.RFC3339Nano)))
	if rr.Code != http.StatusOK {
		t.Fatalf("page1 status %d", rr.Code)
	}
	if len(body.Activity) != 1 || body.Activity[0]["id"] != first {
		t.Fatalf("expected first=%s, got %v", first, body.Activity)
	}
	if body.NextCursor == nil {
		t.Fatalf("expected next_cursor on page1")
	}

	rr2, body2 := callListWorkspaceActivity(t, testWorkspaceID,
		fmt.Sprintf("since=%s&action=gate_bypassed&limit=1&cursor=%s",
			since.Format(time.RFC3339Nano), *body.NextCursor))
	if rr2.Code != http.StatusOK {
		t.Fatalf("page2 status %d", rr2.Code)
	}
	if len(body2.Activity) != 1 || body2.Activity[0]["id"] != second {
		t.Fatalf("expected second=%s, got %v", second, body2.Activity)
	}
}

func TestListWorkspaceActivity_RequiresSince(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	rr, _ := callListWorkspaceActivity(t, testWorkspaceID, "")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without since, got %d", rr.Code)
	}
	rr2, _ := callListWorkspaceActivity(t, testWorkspaceID, "since=not-rfc3339")
	if rr2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed since, got %d", rr2.Code)
	}
}

func TestListWorkspaceActivity_LimitTooLarge(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	rr, _ := callListWorkspaceActivity(t, testWorkspaceID,
		"since=2026-01-01T00:00:00Z&limit=1001")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for limit=1001, got %d", rr.Code)
	}
}
