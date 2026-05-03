package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// Tests for the type-based write guards in CreateComment.
//
// The comment.type CHECK constraint accepts many values for forward compat;
// these guards make sure only `comment` and (constrained) `fixer_reply` are
// writable through the user API. cr_review_comment is webhook-only;
// system / status_change / progress_update / debug / impl_plan /
// completion_note / change_log / review are reserved.

// TestCreateComment_RejectsCRReviewComment confirms the type that is
// only legitimate from the GitHub webhook handler can't be written via
// the user/agent API.
func TestCreateComment_RejectsCRReviewComment(t *testing.T) {
	ctx := context.Background()

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
		"title": "cr_review_comment guard",
	})
	testHandler.CreateIssue(w, req)
	var issue IssueResponse
	json.NewDecoder(w.Body).Decode(&issue)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM comment WHERE issue_id = $1`, issue.ID)
		testPool.Exec(ctx, `DELETE FROM issue WHERE id = $1`, issue.ID)
	})

	// Member POST.
	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/issues/"+issue.ID+"/comments", map[string]any{
		"content": "fake CR finding",
		"type":    "cr_review_comment",
	})
	req = withURLParam(req, "id", issue.ID)
	testHandler.CreateComment(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("member POST cr_review_comment: expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "webhook") {
		t.Fatalf("expected error message to reference webhook ownership, got %s", w.Body.String())
	}

	// Agent POST (a stolen agent token shouldn't gain this either).
	agentID := lookupFixtureAgentID(t, ctx)
	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/issues/"+issue.ID+"/comments", map[string]any{
		"content": "fake CR finding via agent",
		"type":    "cr_review_comment",
	})
	req = withURLParam(req, "id", issue.ID)
	req.Header.Set("X-Agent-ID", agentID)
	testHandler.CreateComment(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("agent POST cr_review_comment: expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// TestCreateComment_RejectsReservedTypes covers the reserved-type set:
// posting any of {system, status_change, progress_update, debug, impl_plan,
// completion_note, change_log, review} via the user API returns 403.
func TestCreateComment_RejectsReservedTypes(t *testing.T) {
	ctx := context.Background()

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
		"title": "reserved-type guard",
	})
	testHandler.CreateIssue(w, req)
	var issue IssueResponse
	json.NewDecoder(w.Body).Decode(&issue)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM comment WHERE issue_id = $1`, issue.ID)
		testPool.Exec(ctx, `DELETE FROM issue WHERE id = $1`, issue.ID)
	})

	reservedTypes := []string{
		"system", "status_change", "progress_update",
		"debug", "impl_plan", "completion_note", "change_log", "review",
	}
	for _, ty := range reservedTypes {
		t.Run(ty, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := newRequest("POST", "/api/issues/"+issue.ID+"/comments", map[string]any{
				"content": "should not be allowed",
				"type":    ty,
			})
			req = withURLParam(req, "id", issue.ID)
			testHandler.CreateComment(w, req)
			if w.Code != http.StatusForbidden {
				t.Fatalf("type=%s: expected 403, got %d: %s", ty, w.Code, w.Body.String())
			}
		})
	}
}

// TestCreateComment_FixerReplyInvariants exercises the four parent-chain
// invariants for fixer_reply: agent author, parent_id required, parent
// type must be cr_review_comment, parent must have review_thread_id.
func TestCreateComment_FixerReplyInvariants(t *testing.T) {
	ctx := context.Background()

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, map[string]any{
		"title": "fixer_reply invariants",
	})
	testHandler.CreateIssue(w, req)
	var issue IssueResponse
	json.NewDecoder(w.Body).Decode(&issue)

	// Stub: pretend a CR webhook landed by inserting an issue_review_thread row
	// and a matching cr_review_comment row directly. This bypasses the GitHub
	// webhook signature path that the real handler uses.
	var threadID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO issue_review_thread
		    (workspace_id, issue_id, pr_repo, pr_number, gh_comment_id,
		     file_path, line, side, severity, title, body, url, author_login)
		VALUES
		    ($1, $2, 'zeyad-farrag/multica', 19, 9999000001,
		     'foo.go', 42, 'RIGHT', 'nitpick', 'Title', 'Body', 'https://example/x', 'coderabbitai[bot]')
		RETURNING id
	`, testWorkspaceID, issue.ID).Scan(&threadID); err != nil {
		t.Fatalf("seed issue_review_thread: %v", err)
	}
	var crCommentID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO comment
		    (issue_id, workspace_id, author_type, author_id, content, type, review_thread_id)
		VALUES
		    ($1, $2, 'system', NULL, '<rendered CR finding>', 'cr_review_comment', $3)
		RETURNING id
	`, issue.ID, testWorkspaceID, threadID).Scan(&crCommentID); err != nil {
		t.Fatalf("seed cr_review_comment: %v", err)
	}

	// Also create a regular `comment`-type row to test the wrong-parent case.
	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/issues/"+issue.ID+"/comments", map[string]any{
		"content": "regular human comment",
	})
	req = withURLParam(req, "id", issue.ID)
	testHandler.CreateComment(w, req)
	var regular CommentResponse
	json.NewDecoder(w.Body).Decode(&regular)

	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM comment WHERE issue_id = $1`, issue.ID)
		testPool.Exec(ctx, `DELETE FROM issue_review_thread WHERE id = $1`, threadID)
		testPool.Exec(ctx, `DELETE FROM issue WHERE id = $1`, issue.ID)
	})

	agentID := lookupFixtureAgentID(t, ctx)

	post := func(body map[string]any, headers map[string]string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := newRequest("POST", "/api/issues/"+issue.ID+"/comments", body)
		r = withURLParam(r, "id", issue.ID)
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		testHandler.CreateComment(w, r)
		return w
	}

	t.Run("rejected without agent author (member)", func(t *testing.T) {
		w := post(
			map[string]any{
				"content":   "Fixed in abc1234.",
				"type":      "fixer_reply",
				"parent_id": crCommentID,
			},
			nil,
		)
		if w.Code != http.StatusForbidden {
			t.Fatalf("member fixer_reply: expected 403, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "agent") {
			t.Fatalf("expected error to mention agent requirement, got %s", w.Body.String())
		}
	})

	t.Run("rejected without parent_id", func(t *testing.T) {
		w := post(
			map[string]any{
				"content": "Fixed.",
				"type":    "fixer_reply",
			},
			map[string]string{"X-Agent-ID": agentID},
		)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("missing parent_id: expected 400, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "parent_id") {
			t.Fatalf("expected error to mention parent_id, got %s", w.Body.String())
		}
	})

	t.Run("rejected when parent is type=comment", func(t *testing.T) {
		w := post(
			map[string]any{
				"content":   "Reply to wrong parent",
				"type":      "fixer_reply",
				"parent_id": regular.ID,
			},
			map[string]string{"X-Agent-ID": agentID},
		)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("wrong-parent-type: expected 400, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "cr_review_comment") {
			t.Fatalf("expected error to name the required parent type, got %s", w.Body.String())
		}
	})

	t.Run("rejected when parent has no review_thread_id", func(t *testing.T) {
		// Manually craft a pathological cr_review_comment row WITHOUT a
		// review_thread_id (would not happen via the webhook, but the guard
		// should still defend against this drift).
		var orphanID string
		if err := testPool.QueryRow(ctx, `
			INSERT INTO comment
			    (issue_id, workspace_id, author_type, author_id, content, type, review_thread_id)
			VALUES
			    ($1, $2, 'system', NULL, 'orphan', 'cr_review_comment', NULL)
			RETURNING id
		`, issue.ID, testWorkspaceID).Scan(&orphanID); err != nil {
			t.Fatalf("seed orphan cr_review_comment: %v", err)
		}
		defer testPool.Exec(ctx, `DELETE FROM comment WHERE id = $1`, orphanID)

		w := post(
			map[string]any{
				"content":   "Should not stick",
				"type":      "fixer_reply",
				"parent_id": orphanID,
			},
			map[string]string{"X-Agent-ID": agentID},
		)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("orphan parent: expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("happy path: agent + cr_review_comment parent", func(t *testing.T) {
		w := post(
			map[string]any{
				"content":   "Fixed in abc1234: extracted helper into scripts/lib/docker-helpers.sh.",
				"type":      "fixer_reply",
				"parent_id": crCommentID,
			},
			map[string]string{"X-Agent-ID": agentID},
		)
		if w.Code != http.StatusCreated {
			t.Fatalf("happy path: expected 201, got %d: %s", w.Code, w.Body.String())
		}
		var c CommentResponse
		json.NewDecoder(w.Body).Decode(&c)
		if c.Type != "fixer_reply" {
			t.Fatalf("expected type=fixer_reply, got %q", c.Type)
		}
		if c.ParentID == nil || *c.ParentID != crCommentID {
			t.Fatalf("expected parent_id=%s, got %v", crCommentID, c.ParentID)
		}
	})
}

// lookupFixtureAgentID returns the UUID of the pre-seeded "Handler Test Agent"
// row created by setupHandlerTestFixture. Used by guard tests that need an
// agent identity for X-Agent-ID without rolling a new fixture.
func lookupFixtureAgentID(t *testing.T, ctx context.Context) string {
	t.Helper()
	var id string
	if err := testPool.QueryRow(ctx,
		`SELECT id FROM agent WHERE workspace_id = $1 AND name = $2`,
		testWorkspaceID, "Handler Test Agent",
	).Scan(&id); err != nil {
		t.Fatalf("look up fixture agent: %v", err)
	}
	return id
}

// helper: avoid unused import
var _ = pgtype.UUID{}
