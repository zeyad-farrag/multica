package handler

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// Tests for maybeMarkFixerReplyPosted, the helper that stamps
// comment.posted_to_github_at after Marcus's bmad-pr-resolve skill posts a
// reply to GitHub. The full GitHub reply path is exercised by the existing
// integration tests; here we cover the validation + stamping logic directly
// to keep the test self-contained (no GitHub mocks needed).

func TestMaybeMarkFixerReplyPosted_HappyPath(t *testing.T) {
	ctx := context.Background()
	issueID, threadID, crCommentID, fixerReplyID := seedFixerReplyChain(t, ctx)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM comment WHERE issue_id = $1`, issueID)
		testPool.Exec(ctx, `DELETE FROM issue_review_thread WHERE id = $1`, threadID)
		testPool.Exec(ctx, `DELETE FROM issue WHERE id = $1`, issueID)
	})
	_ = crCommentID

	got := testHandler.maybeMarkFixerReplyPosted(ctx, parseUUID(issueID), fixerReplyID, parseUUID(threadID))
	if got == "" {
		t.Fatal("expected a non-empty timestamp; got empty")
	}

	// DB row reflects the stamp.
	var posted pgtype.Timestamptz
	if err := testPool.QueryRow(ctx,
		`SELECT posted_to_github_at FROM comment WHERE id = $1`, fixerReplyID,
	).Scan(&posted); err != nil {
		t.Fatalf("read posted_to_github_at: %v", err)
	}
	if !posted.Valid {
		t.Fatal("expected posted_to_github_at to be set after mark; got NULL")
	}

	// Idempotent: a second call returns the (unchanged) timestamp.
	got2 := testHandler.maybeMarkFixerReplyPosted(ctx, parseUUID(issueID), fixerReplyID, parseUUID(threadID))
	if got2 == "" {
		t.Fatal("idempotent re-mark returned empty; expected the original timestamp")
	}
	// Same string — COALESCE preserves the original now() value.
	if got != got2 {
		t.Fatalf("idempotent re-mark changed the timestamp: %q -> %q", got, got2)
	}
}

func TestMaybeMarkFixerReplyPosted_RejectsCrossIssue(t *testing.T) {
	ctx := context.Background()
	issueID, threadID, _, fixerReplyID := seedFixerReplyChain(t, ctx)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM comment WHERE issue_id = $1`, issueID)
		testPool.Exec(ctx, `DELETE FROM issue_review_thread WHERE id = $1`, threadID)
		testPool.Exec(ctx, `DELETE FROM issue WHERE id = $1`, issueID)
	})

	// Create a second issue and pass *its* ID — should not stamp.
	// Compute number = MAX+1 so we don't trip uq_issue_workspace_number
	// (the seedFixerReplyChain helper uses the column default of 0).
	var otherIssueID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO issue (workspace_id, title, status, creator_type, creator_id, position, number)
		VALUES ($1, 'other issue', 'backlog', 'member', $2, 0,
		        (SELECT COALESCE(MAX(number), 0) + 1 FROM issue WHERE workspace_id = $1))
		RETURNING id::text
	`, testWorkspaceID, testUserID).Scan(&otherIssueID); err != nil {
		t.Fatalf("seed other issue: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM issue WHERE id = $1`, otherIssueID) })

	got := testHandler.maybeMarkFixerReplyPosted(ctx, parseUUID(otherIssueID), fixerReplyID, parseUUID(threadID))
	if got != "" {
		t.Fatalf("cross-issue mark should fail; got %q", got)
	}
}

func TestMaybeMarkFixerReplyPosted_RejectsThreadMismatch(t *testing.T) {
	ctx := context.Background()
	issueID, threadID, _, fixerReplyID := seedFixerReplyChain(t, ctx)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM comment WHERE issue_id = $1`, issueID)
		testPool.Exec(ctx, `DELETE FROM issue_review_thread WHERE id = $1`, threadID)
		testPool.Exec(ctx, `DELETE FROM issue WHERE id = $1`, issueID)
	})

	// Seed a SECOND review thread on the same issue, then claim the fixer_reply
	// belongs to *that* thread. Server should reject (parent's review_thread_id
	// is the first thread).
	var otherThreadID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO issue_review_thread
		    (workspace_id, issue_id, pr_repo, pr_number, gh_comment_id,
		     file_path, line, side, severity, title, body, url, author_login)
		VALUES
		    ($1, $2, 'zeyad-farrag/multica', 19, 9999000099,
		     'bar.go', 7, 'RIGHT', 'nitpick', 't', 'b', 'u', 'coderabbitai[bot]')
		RETURNING id::text
	`, testWorkspaceID, issueID).Scan(&otherThreadID); err != nil {
		t.Fatalf("seed other thread: %v", err)
	}

	got := testHandler.maybeMarkFixerReplyPosted(ctx, parseUUID(issueID), fixerReplyID, parseUUID(otherThreadID))
	if got != "" {
		t.Fatalf("thread mismatch should fail; got %q", got)
	}
}

func TestMaybeMarkFixerReplyPosted_RejectsWrongCommentType(t *testing.T) {
	ctx := context.Background()
	issueID, threadID, _, _ := seedFixerReplyChain(t, ctx)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM comment WHERE issue_id = $1`, issueID)
		testPool.Exec(ctx, `DELETE FROM issue_review_thread WHERE id = $1`, threadID)
		testPool.Exec(ctx, `DELETE FROM issue WHERE id = $1`, issueID)
	})

	// A regular `comment`-type row — wrong type for marking.
	var regularID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO comment (issue_id, workspace_id, author_type, author_id, content, type)
		VALUES ($1, $2, 'member', $3, 'regular', 'comment')
		RETURNING id::text
	`, issueID, testWorkspaceID, testUserID).Scan(&regularID); err != nil {
		t.Fatalf("seed regular comment: %v", err)
	}

	got := testHandler.maybeMarkFixerReplyPosted(ctx, parseUUID(issueID), regularID, parseUUID(threadID))
	if got != "" {
		t.Fatalf("non-fixer_reply mark should fail; got %q", got)
	}
}

func TestMaybeMarkFixerReplyPosted_EmptyIDIsNoOp(t *testing.T) {
	ctx := context.Background()
	got := testHandler.maybeMarkFixerReplyPosted(ctx, parseUUID(testWorkspaceID), "", parseUUID(testWorkspaceID))
	if got != "" {
		t.Fatalf("empty fixer_reply_comment_id should be a silent no-op; got %q", got)
	}
}

// seedFixerReplyChain creates an issue, an issue_review_thread, a matching
// cr_review_comment row, and a fixer_reply child comment. Returns string IDs
// for issue / thread / cr_review_comment / fixer_reply.
func seedFixerReplyChain(t *testing.T, ctx context.Context) (string, string, string, string) {
	t.Helper()

	var issueID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO issue (workspace_id, title, status, creator_type, creator_id, position, number)
		VALUES ($1, 'P2 mark test', 'backlog', 'member', $2, 0,
		        (SELECT COALESCE(MAX(number), 0) + 1 FROM issue WHERE workspace_id = $1))
		RETURNING id::text
	`, testWorkspaceID, testUserID).Scan(&issueID); err != nil {
		t.Fatalf("seed issue: %v", err)
	}

	var threadID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO issue_review_thread
		    (workspace_id, issue_id, pr_repo, pr_number, gh_comment_id,
		     file_path, line, side, severity, title, body, url, author_login)
		VALUES
		    ($1, $2, 'zeyad-farrag/multica', 19, 9999000010,
		     'foo.go', 42, 'RIGHT', 'nitpick', 't', 'b', 'u', 'coderabbitai[bot]')
		RETURNING id::text
	`, testWorkspaceID, issueID).Scan(&threadID); err != nil {
		t.Fatalf("seed thread: %v", err)
	}

	var crCommentID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO comment
		    (issue_id, workspace_id, author_type, author_id, content, type, review_thread_id)
		VALUES
		    ($1, $2, 'system', NULL, 'cr finding', 'cr_review_comment', $3)
		RETURNING id::text
	`, issueID, testWorkspaceID, threadID).Scan(&crCommentID); err != nil {
		t.Fatalf("seed cr_review_comment: %v", err)
	}

	agentID := lookupFixtureAgentID(t, ctx)
	var fixerReplyID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO comment
		    (issue_id, workspace_id, author_type, author_id, content, type, parent_id)
		VALUES
		    ($1, $2, 'agent', $3, 'Fixed.', 'fixer_reply', $4)
		RETURNING id::text
	`, issueID, testWorkspaceID, agentID, crCommentID).Scan(&fixerReplyID); err != nil {
		t.Fatalf("seed fixer_reply: %v", err)
	}
	return issueID, threadID, crCommentID, fixerReplyID
}
