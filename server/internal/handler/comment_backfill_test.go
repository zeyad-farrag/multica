package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// seedIssueForComment inserts a parent issue scoped to the test workspace
// and registers cleanup. Returns the issue UUID.
func seedIssueForComment(t *testing.T, title string) string {
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
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, id) })
	return id
}

// seedCommentAt inserts a comment with the specified actor, type, and
// created_at. Returns the comment UUID. Cleanup is automatic.
func seedCommentAt(t *testing.T, issueID, authorType, authorID, commentType, content string, createdAt time.Time) string {
	t.Helper()
	ctx := context.Background()
	var id string
	err := testPool.QueryRow(ctx, `
		INSERT INTO comment (issue_id, workspace_id, author_type, author_id, content, type, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, issueID, testWorkspaceID, authorType, authorID, content, commentType, createdAt).Scan(&id)
	if err != nil {
		t.Fatalf("seed comment: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM comment WHERE id = $1`, id) })
	return id
}

func callListCommentsForBackfill(t *testing.T, qs url.Values) (int, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/workspaces/"+testWorkspaceID+"/comments?"+qs.Encode(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", testWorkspaceID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	testHandler.ListCommentsForBackfill(w, req)
	var body map[string]any
	if w.Body.Len() > 0 {
		_ = json.NewDecoder(w.Body).Decode(&body)
	}
	return w.Code, body
}

// uniqueDate returns a deterministic UTC date in the future used for the day
// window so other tests' fixtures can't pollute the result.
func uniqueDate(t *testing.T) (string, time.Time) {
	t.Helper()
	day := time.Date(2030, time.Month((t.Name()[0]%12)+1), int((t.Name()[1]%27)+1), 0, 0, 0, 0, time.UTC)
	return day.Format("2006-01-02"), day
}

// TestCommentsBackfillFiltersByAuthorTypeDate verifies the (author_id, type,
// date) filter (AC #4, AC #11). Other rows on the same day or other days
// for the same author/type must be excluded.
func TestCommentsBackfillFiltersByAuthorTypeDate(t *testing.T) {
	issueID := seedIssueForComment(t, "comments backfill")
	dateStr, day := uniqueDate(t)
	matching := seedCommentAt(t, issueID, "member", testUserID, "status_change", "match-1", day.Add(time.Hour))
	// Different type — excluded.
	seedCommentAt(t, issueID, "member", testUserID, "comment", "wrong-type", day.Add(2*time.Hour))
	// Different day — excluded.
	seedCommentAt(t, issueID, "member", testUserID, "status_change", "wrong-day", day.Add(48*time.Hour))

	qs := url.Values{}
	qs.Set("author_id", testUserID)
	qs.Set("type", "status_change")
	qs.Set("date", dateStr)
	code, body := callListCommentsForBackfill(t, qs)
	if code != 200 {
		t.Fatalf("status=%d body=%v", code, body)
	}
	comments, _ := body["comments"].([]any)
	if len(comments) != 1 {
		t.Fatalf("expected 1 matching comment, got %d (%v)", len(comments), comments)
	}
	got := comments[0].(map[string]any)["id"].(string)
	if got != matching {
		t.Fatalf("got %s, want %s", got, matching)
	}
}

// TestCommentsBackfillReturnsAgentAuthored verifies AC #5 — agent-authored
// comments must be returned, not silently filtered. Spec §6.1 #6 said
// otherwise but the epic AC + §14 are canonical.
func TestCommentsBackfillReturnsAgentAuthored(t *testing.T) {
	issueID := seedIssueForComment(t, "agent-author backfill")
	dateStr, day := uniqueDate(t)

	// Find any agent in the test workspace to use as the author.
	var agentID string
	if err := testPool.QueryRow(context.Background(),
		`SELECT id FROM agent WHERE workspace_id = $1 LIMIT 1`,
		testWorkspaceID,
	).Scan(&agentID); err != nil {
		t.Fatalf("locate agent: %v", err)
	}

	agentComment := seedCommentAt(t, issueID, "agent", agentID, "status_change", "agent-msg", day.Add(time.Hour))

	qs := url.Values{}
	qs.Set("author_id", agentID)
	qs.Set("type", "status_change")
	qs.Set("date", dateStr)
	code, body := callListCommentsForBackfill(t, qs)
	if code != 200 {
		t.Fatalf("status=%d body=%v", code, body)
	}
	comments, _ := body["comments"].([]any)
	if len(comments) != 1 {
		t.Fatalf("expected agent comment to be returned, got %d", len(comments))
	}
	if got := comments[0].(map[string]any)["id"].(string); got != agentComment {
		t.Fatalf("got %s, want %s", got, agentComment)
	}
	if at := comments[0].(map[string]any)["author_type"].(string); at != "agent" {
		t.Fatalf("expected author_type=agent, got %s", at)
	}
}

// TestCommentsBackfillCursorAdvances verifies that the cursor advances and
// excludes already-returned rows under tie-break (AC #4, AC #11).
func TestCommentsBackfillCursorAdvances(t *testing.T) {
	issueID := seedIssueForComment(t, "cursor advance")
	dateStr, day := uniqueDate(t)

	// Three comments at different sub-second times to give a deterministic order.
	id1 := seedCommentAt(t, issueID, "member", testUserID, "comment", "c1", day.Add(time.Hour))
	id2 := seedCommentAt(t, issueID, "member", testUserID, "comment", "c2", day.Add(time.Hour+time.Second))
	id3 := seedCommentAt(t, issueID, "member", testUserID, "comment", "c3", day.Add(time.Hour+2*time.Second))

	qs := url.Values{}
	qs.Set("author_id", testUserID)
	qs.Set("type", "comment")
	qs.Set("date", dateStr)
	qs.Set("limit", "2")
	code, body := callListCommentsForBackfill(t, qs)
	if code != 200 {
		t.Fatalf("page1 status=%d body=%v", code, body)
	}
	page1, _ := body["comments"].([]any)
	if len(page1) != 2 {
		t.Fatalf("page1 expected 2, got %d", len(page1))
	}
	if page1[0].(map[string]any)["id"].(string) != id1 || page1[1].(map[string]any)["id"].(string) != id2 {
		t.Fatalf("page1 wrong order: %v", page1)
	}
	cursor, _ := body["next_cursor"].(string)
	if cursor == "" {
		t.Fatal("page1 expected next_cursor")
	}

	qs.Set("cursor", cursor)
	code, body = callListCommentsForBackfill(t, qs)
	if code != 200 {
		t.Fatalf("page2 status=%d body=%v", code, body)
	}
	page2, _ := body["comments"].([]any)
	if len(page2) != 1 || page2[0].(map[string]any)["id"].(string) != id3 {
		t.Fatalf("page2 expected [%s], got %v", id3, page2)
	}
}

func TestCommentsBackfillMissingParams(t *testing.T) {
	dateStr, _ := uniqueDate(t)
	cases := []struct {
		name string
		set  func(url.Values)
	}{
		{"no author_id", func(q url.Values) { q.Set("type", "comment"); q.Set("date", dateStr) }},
		{"no type", func(q url.Values) { q.Set("author_id", testUserID); q.Set("date", dateStr) }},
		{"no date", func(q url.Values) { q.Set("author_id", testUserID); q.Set("type", "comment") }},
		{"bad date", func(q url.Values) {
			q.Set("author_id", testUserID)
			q.Set("type", "comment")
			q.Set("date", "April 1")
		}},
		{"bad author", func(q url.Values) {
			q.Set("author_id", "not-uuid")
			q.Set("type", "comment")
			q.Set("date", dateStr)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			qs := url.Values{}
			tc.set(qs)
			code, body := callListCommentsForBackfill(t, qs)
			if code != 400 {
				t.Fatalf("%s: expected 400, got %d body=%v", tc.name, code, body)
			}
		})
	}
}

// just ensure compile-time fmt import isn't tree-shaken away if needed later.
var _ = fmt.Sprintf
