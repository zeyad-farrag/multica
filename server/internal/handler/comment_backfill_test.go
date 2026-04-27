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

// seedCommentAt inserts a comment row with a force-stamped created_at so
// tests can deterministically place it in or out of a workspace-day window.
func seedCommentAt(t *testing.T, wsID, issueID, authorID, authorType, commentType string, createdAt time.Time) string {
	t.Helper()
	var id string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO comment (issue_id, workspace_id, author_type, author_id, content, type)
		VALUES ($1, $2, $3, $4, 'tim4-test-content', $5)
		RETURNING id
	`, issueID, wsID, authorType, authorID, commentType).Scan(&id); err != nil {
		t.Fatalf("seed comment: %v", err)
	}
	if _, err := testPool.Exec(context.Background(), `
		UPDATE comment SET created_at = $1, updated_at = $1 WHERE id = $2
	`, createdAt, id); err != nil {
		t.Fatalf("update comment timestamps: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM comment WHERE id = $1`, id)
	})
	return id
}

type commentsBackfillResp struct {
	Comments   []map[string]any `json:"comments"`
	NextCursor *string          `json:"next_cursor"`
	Total      int64            `json:"total"`
}

func callListCommentsForBackfill(t *testing.T, wsID, query string) (*httptest.ResponseRecorder, commentsBackfillResp) {
	t.Helper()
	req := newRequest(http.MethodGet, "/api/workspaces/"+wsID+"/comments?"+query, nil)
	req = withURLParam(req, "id", wsID)
	rr := httptest.NewRecorder()
	testHandler.ListCommentsForBackfill(rr, req)
	var body commentsBackfillResp
	if rr.Code == http.StatusOK {
		if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
	}
	return rr, body
}

func TestListCommentsForBackfill_FilterByAuthorTypeDate(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	issueID := seedIssueAt(t, testWorkspaceID, "tim4-comment-host", time.Now().UTC(), 7200)

	// Date window: 2026-04-25 UTC (workspace TZ defaults to UTC).
	day := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	out := time.Date(2026, 4, 24, 23, 0, 0, 0, time.UTC) // outside window
	in := day                                             // inside window

	// Match: status_change by testUserID inside the day.
	matchID := seedCommentAt(t, testWorkspaceID, issueID, testUserID, "member", "status_change", in)
	// Wrong type — same author/day.
	seedCommentAt(t, testWorkspaceID, issueID, testUserID, "member", "comment", in)
	// Wrong day — same author/type.
	seedCommentAt(t, testWorkspaceID, issueID, testUserID, "member", "status_change", out)

	rr, body := callListCommentsForBackfill(t, testWorkspaceID,
		fmt.Sprintf("author_id=%s&type=status_change&date=2026-04-25", testUserID))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if len(body.Comments) != 1 {
		t.Fatalf("expected 1 comment, got %d: %v", len(body.Comments), body.Comments)
	}
	if body.Comments[0]["id"] != matchID {
		t.Errorf("expected id=%s, got %s", matchID, body.Comments[0]["id"])
	}
}

// AC-5: agent-authored comments must be returned (not silently filtered out).
func TestListCommentsForBackfill_IncludesAgentAuthored(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	issueID := seedIssueAt(t, testWorkspaceID, "tim4-comment-agent-host", time.Now().UTC(), 7210)

	// Get an agent ID from the test fixture's workspace.
	var agentID string
	if err := testPool.QueryRow(context.Background(),
		`SELECT id FROM agent WHERE workspace_id = $1 LIMIT 1`, testWorkspaceID).Scan(&agentID); err != nil {
		t.Fatalf("fetch agent: %v", err)
	}

	day := time.Date(2026, 4, 26, 9, 0, 0, 0, time.UTC)
	matchID := seedCommentAt(t, testWorkspaceID, issueID, agentID, "agent", "status_change", day)

	rr, body := callListCommentsForBackfill(t, testWorkspaceID,
		fmt.Sprintf("author_id=%s&type=status_change&date=2026-04-26", agentID))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if len(body.Comments) != 1 {
		t.Fatalf("expected 1 agent-authored comment, got %d (the endpoint MUST NOT silently drop author_type='agent')", len(body.Comments))
	}
	if body.Comments[0]["id"] != matchID {
		t.Errorf("expected agent comment id=%s, got %s", matchID, body.Comments[0]["id"])
	}
	if body.Comments[0]["author_type"] != "agent" {
		t.Errorf("expected author_type=agent, got %v", body.Comments[0]["author_type"])
	}
}

func TestListCommentsForBackfill_PaginationCursor(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	issueID := seedIssueAt(t, testWorkspaceID, "tim4-comment-page-host", time.Now().UTC(), 7220)

	day := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		seedCommentAt(t, testWorkspaceID, issueID, testUserID, "member", "status_change",
			day.Add(time.Duration(i)*time.Hour))
	}

	// limit=2 → cursor set; second page returns the remaining 1.
	rr, body := callListCommentsForBackfill(t, testWorkspaceID,
		fmt.Sprintf("author_id=%s&type=status_change&date=2026-04-27&limit=2", testUserID))
	if rr.Code != http.StatusOK {
		t.Fatalf("page1 status %d", rr.Code)
	}
	if len(body.Comments) != 2 {
		t.Fatalf("expected 2 on page1, got %d", len(body.Comments))
	}
	if body.NextCursor == nil {
		t.Fatalf("expected next_cursor on page1")
	}

	rr2, body2 := callListCommentsForBackfill(t, testWorkspaceID,
		fmt.Sprintf("author_id=%s&type=status_change&date=2026-04-27&limit=2&cursor=%s",
			testUserID, *body.NextCursor))
	if rr2.Code != http.StatusOK {
		t.Fatalf("page2 status %d", rr2.Code)
	}
	if len(body2.Comments) != 1 {
		t.Fatalf("expected 1 on page2, got %d", len(body2.Comments))
	}
	if body2.NextCursor != nil {
		t.Errorf("expected nil next_cursor on terminal page")
	}
}

func TestListCommentsForBackfill_MissingParams(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	cases := []string{
		"",
		"author_id=" + testUserID,
		"author_id=" + testUserID + "&type=status_change",
		"author_id=not-a-uuid&type=status_change&date=2026-04-27",
		"author_id=" + testUserID + "&type=status_change&date=not-a-date",
	}
	for _, q := range cases {
		rr, _ := callListCommentsForBackfill(t, testWorkspaceID, q)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("query %q: expected 400, got %d", q, rr.Code)
		}
	}
}
