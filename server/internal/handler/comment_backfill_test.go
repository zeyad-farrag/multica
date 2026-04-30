package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestListCommentsForBackfillIncludesAgentRowsAndCursor(t *testing.T) {
	ctx := context.Background()
	issueID := "00000000-0000-0000-0000-000000000021"
	authorID := "00000000-0000-0000-0000-000000000031"
	ts := time.Date(2026, 4, 30, 8, 0, 0, 0, time.UTC)
	seedUpdatedSinceIssue(t, issueID, "comments", 9201, ts)
	if _, err := testPool.Exec(ctx, `UPDATE workspace SET settings = '{"timezone":"UTC"}'::jsonb WHERE id = $1`, testWorkspaceID); err != nil {
		t.Fatalf("set timezone: %v", err)
	}
	for _, row := range []struct {
		id         string
		authorType string
		content    string
	}{
		{"00000000-0000-0000-0000-000000000041", "member", "member row"},
		{"00000000-0000-0000-0000-000000000042", "agent", "agent row"},
	} {
		_, err := testPool.Exec(ctx, `
			INSERT INTO comment (id, issue_id, workspace_id, author_type, author_id, content, type, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, 'status_change', $7, $7)
		`, row.id, issueID, testWorkspaceID, row.authorType, authorID, row.content, ts)
		if err != nil {
			t.Fatalf("seed comment: %v", err)
		}
	}

	w := httptest.NewRecorder()
	req := newRequest("GET", "/api/workspaces/"+testWorkspaceID+"/comments?author_id="+authorID+"&type=status_change&date=2026-04-30&limit=1", nil)
	req = withURLParam(req, "id", testWorkspaceID)
	testHandler.ListCommentsForBackfill(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first page: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var first struct {
		Comments   []CommentResponse `json:"comments"`
		NextCursor *string           `json:"next_cursor"`
		Total      int64             `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&first); err != nil {
		t.Fatalf("decode first page: %v", err)
	}
	if len(first.Comments) != 1 || first.Comments[0].AuthorType != "member" || first.NextCursor == nil || first.Total != 2 {
		t.Fatalf("unexpected first page: %+v", first)
	}

	w = httptest.NewRecorder()
	req = newRequest("GET", "/api/workspaces/"+testWorkspaceID+"/comments?author_id="+authorID+"&type=status_change&date=2026-04-30&limit=1&cursor="+*first.NextCursor, nil)
	req = withURLParam(req, "id", testWorkspaceID)
	testHandler.ListCommentsForBackfill(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("second page: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var second struct {
		Comments []CommentResponse `json:"comments"`
	}
	if err := json.NewDecoder(w.Body).Decode(&second); err != nil {
		t.Fatalf("decode second page: %v", err)
	}
	if len(second.Comments) != 1 || second.Comments[0].AuthorType != "agent" {
		t.Fatalf("agent-authored row was not returned: %+v", second.Comments)
	}
}

func TestListCommentsForBackfillRequiresParams(t *testing.T) {
	w := httptest.NewRecorder()
	req := newRequest("GET", "/api/workspaces/"+testWorkspaceID+"/comments?type=status_change&date=2026-04-30", nil)
	req = withURLParam(req, "id", testWorkspaceID)
	testHandler.ListCommentsForBackfill(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
