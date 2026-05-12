package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCRAttemptEndpoints_AuthorizeAndBindAttemptToIssue(t *testing.T) {
	ctx := context.Background()
	issueID := seedCRAttemptIssue(t, testWorkspaceID)
	attemptID := seedCRAttempt(t, issueID, testWorkspaceID, 1)
	otherIssueID := seedCRAttemptIssue(t, testWorkspaceID)
	otherAttemptID := seedCRAttempt(t, otherIssueID, testWorkspaceID, 1)
	seedCRSignal(t, attemptID, "check_run")

	w := httptest.NewRecorder()
	testHandler.ListCRAttempts(w, withURLParam(newRequest("GET", "/api/issues/"+issueID+"/cr-attempts", nil), "id", issueID))
	if w.Code != http.StatusOK {
		t.Fatalf("ListCRAttempts status = %d body=%s", w.Code, w.Body.String())
	}
	var attempts []crAttemptResponse
	if err := json.Unmarshal(w.Body.Bytes(), &attempts); err != nil {
		t.Fatalf("decode attempts: %v", err)
	}
	if len(attempts) != 1 || attempts[0].ID != attemptID {
		t.Fatalf("attempts = %+v, want only %s", attempts, attemptID)
	}

	w = httptest.NewRecorder()
	req := withURLParams(newRequest("GET", "/api/issues/"+issueID+"/cr-attempts/"+attemptID+"/signals", nil), "id", issueID, "attemptID", attemptID)
	testHandler.ListCRSignals(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ListCRSignals status = %d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = withURLParams(newRequest("GET", "/api/issues/"+issueID+"/cr-attempts/"+otherAttemptID+"/signals", nil), "id", issueID, "attemptID", otherAttemptID)
	testHandler.ListCRSignals(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-issue ListCRSignals status = %d, want 403 body=%s", w.Code, w.Body.String())
	}

	_, _ = testPool.Exec(ctx, `DELETE FROM issue WHERE id IN ($1, $2)`, issueID, otherIssueID)
}

func TestCRAttemptConstraints_RejectInvalidSignalKinds(t *testing.T) {
	issueID := seedCRAttemptIssue(t, testWorkspaceID)
	attemptID := seedCRAttempt(t, issueID, testWorkspaceID, 1)
	if _, err := testPool.Exec(context.Background(), `
		UPDATE cr_review_attempt SET first_signal_kind = 'bad_kind' WHERE id = $1
	`, attemptID); err == nil {
		t.Fatalf("expected invalid first_signal_kind to fail")
	}
	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO cr_review_signal (attempt_id, signal_kind, signal_action, payload_summary)
		VALUES ($1, 'bad_kind', 'created', '{}'::jsonb)
	`, attemptID); err == nil {
		t.Fatalf("expected invalid cr_review_signal.signal_kind to fail")
	}
	_, _ = testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, issueID)
}

func seedCRAttemptIssue(t *testing.T, workspaceID string) string {
	t.Helper()
	var issueID string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO issue (workspace_id, title, status, priority, creator_type, creator_id, number)
		VALUES ($1, 'CR attempt test', 'coderabbit', 'medium', 'member', $2,
		        (SELECT COALESCE(MAX(number), 0) + 1 FROM issue WHERE workspace_id = $1))
		RETURNING id
	`, workspaceID, testUserID).Scan(&issueID); err != nil {
		t.Fatalf("seed issue: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, issueID)
	})
	return issueID
}

func seedCRAttempt(t *testing.T, issueID, workspaceID string, round int) string {
	t.Helper()
	var attemptID string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO cr_review_attempt (issue_id, workspace_id, cr_round, pr_url, head_sha)
		VALUES ($1, $2, $3, 'https://github.com/acme/repo/pull/7', 'abc123')
		RETURNING id
	`, issueID, workspaceID, round).Scan(&attemptID); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
	return attemptID
}

func seedCRSignal(t *testing.T, attemptID, kind string) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO cr_review_signal (attempt_id, signal_kind, signal_action, payload_summary)
		VALUES ($1, $2, 'created', '{"name":"CodeRabbit"}'::jsonb)
	`, attemptID, kind); err != nil {
		t.Fatalf("seed signal: %v", err)
	}
}
