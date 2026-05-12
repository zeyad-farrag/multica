package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	githubintegration "github.com/multica-ai/multica/server/internal/integrations/github"
)

var reviewThreadCommentSeq int64 = 9999100000

type crThreadFixture struct {
	IssueID     string
	ThreadID    string
	CRCommentID string
}

func seedPR2Issue(t *testing.T, ctx context.Context, title string) string {
	t.Helper()
	var issueID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO issue (workspace_id, title, status, creator_type, creator_id, position, number)
		VALUES ($1, $2, 'backlog', 'member', $3, 0,
		        (SELECT COALESCE(MAX(number), 0) + 1 FROM issue WHERE workspace_id = $1))
		RETURNING id::text
	`, testWorkspaceID, title, testUserID).Scan(&issueID); err != nil {
		t.Fatalf("seed issue: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, issueID) })
	return issueID
}

func seedPR2Thread(t *testing.T, ctx context.Context, issueID, severity, state string, withParent bool) crThreadFixture {
	t.Helper()
	ghID := atomic.AddInt64(&reviewThreadCommentSeq, 1)
	var threadID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO issue_review_thread
		    (workspace_id, issue_id, pr_repo, pr_number, gh_comment_id,
		     file_path, line, side, severity, title, body, url, author_login, state)
		VALUES
		    ($1, $2, 'zeyad-farrag/multica', 19, $3,
		     $4, 42, 'RIGHT', $5, $6, 'body', 'https://example.test/thread', 'coderabbitai[bot]', $7)
		RETURNING id::text
	`, testWorkspaceID, issueID, ghID, fmt.Sprintf("%s.go", severity), severity, "title "+severity, state).Scan(&threadID); err != nil {
		t.Fatalf("seed thread: %v", err)
	}
	var crCommentID string
	if withParent {
		if err := testPool.QueryRow(ctx, `
			INSERT INTO comment
			    (issue_id, workspace_id, author_type, author_id, content, type, review_thread_id)
			VALUES
			    ($1, $2, 'system', NULL, 'cr finding', 'cr_review_comment', $3)
			RETURNING id::text
		`, issueID, testWorkspaceID, threadID).Scan(&crCommentID); err != nil {
			t.Fatalf("seed cr_review_comment: %v", err)
		}
	}
	return crThreadFixture{IssueID: issueID, ThreadID: threadID, CRCommentID: crCommentID}
}

func pr2Request(method, path string, body any, issueID string, agentID string) (*httptest.ResponseRecorder, *http.Request) {
	req := newRequest(method, path, body)
	if agentID != "" {
		req.Header.Set("X-Agent-ID", agentID)
	}
	req = withURLParam(req, "id", issueID)
	return httptest.NewRecorder(), req
}

func pr2ThreadRequest(method, path string, body any, issueID, threadID string, agentID string) (*httptest.ResponseRecorder, *http.Request) {
	w, req := pr2Request(method, path, body, issueID, agentID)
	req = withURLParams(req, "threadID", threadID)
	return w, req
}

func TestNextResolverThread_OrderingBySeverity(t *testing.T) {
	ctx := context.Background()
	agentID := lookupFixtureAgentID(t, ctx)
	issueID := seedPR2Issue(t, ctx, "PR2 ordering")
	seedPR2Thread(t, ctx, issueID, "nitpick", "unresolved", true)
	want := seedPR2Thread(t, ctx, issueID, "issue", "unresolved", true)
	seedPR2Thread(t, ctx, issueID, "refactor", "unresolved", true)

	w, req := pr2Request("POST", "/api/issues/"+issueID+"/review-threads/next", nil, issueID, agentID)
	testHandler.ClaimNextResolverThread(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("claim next: got %d: %s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["id"] != want.ThreadID {
		t.Fatalf("claim ordering: got %v, want issue-severity thread %s", got["id"], want.ThreadID)
	}
}

func TestClaimNextResolverThread_PersistentClaim(t *testing.T) {
	ctx := context.Background()
	agentA := lookupFixtureAgentID(t, ctx)
	agentB := createHandlerTestAgent(t, "PR2 Claim Agent B", []byte(`{}`))
	issueID := seedPR2Issue(t, ctx, "PR2 persistent claim")
	thread := seedPR2Thread(t, ctx, issueID, "issue", "unresolved", true)

	w, req := pr2Request("POST", "/api/issues/"+issueID+"/review-threads/next", map[string]any{"claim_ttl_secs": 900}, issueID, agentA)
	testHandler.ClaimNextResolverThread(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("agent A claim: got %d: %s", w.Code, w.Body.String())
	}

	w, req = pr2Request("POST", "/api/issues/"+issueID+"/review-threads/next", nil, issueID, agentB)
	testHandler.ClaimNextResolverThread(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("agent B should not see active claim; got %d: %s", w.Code, w.Body.String())
	}

	w, req = pr2ThreadRequest("POST", "/api/issues/"+issueID+"/review-threads/"+thread.ThreadID+"/release-claim", nil, issueID, thread.ThreadID, agentA)
	testHandler.ReleaseThreadClaim(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("release claim: got %d: %s", w.Code, w.Body.String())
	}

	w, req = pr2Request("POST", "/api/issues/"+issueID+"/review-threads/next", nil, issueID, agentB)
	testHandler.ClaimNextResolverThread(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("agent B reclaim: got %d: %s", w.Code, w.Body.String())
	}
}

func TestClaimNextResolverThread_ConcurrentClaimsDifferent(t *testing.T) {
	ctx := context.Background()
	agentA := lookupFixtureAgentID(t, ctx)
	agentB := createHandlerTestAgent(t, "PR2 Concurrent Agent B", []byte(`{}`))
	issueID := seedPR2Issue(t, ctx, "PR2 concurrent claims")
	seedPR2Thread(t, ctx, issueID, "issue", "unresolved", true)
	seedPR2Thread(t, ctx, issueID, "refactor", "unresolved", true)

	var wg sync.WaitGroup
	results := make(chan string, 2)
	claim := func(agentID string) {
		defer wg.Done()
		w, req := pr2Request("POST", "/api/issues/"+issueID+"/review-threads/next", nil, issueID, agentID)
		testHandler.ClaimNextResolverThread(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("claim got %d: %s", w.Code, w.Body.String())
			return
		}
		var got map[string]any
		_ = json.NewDecoder(w.Body).Decode(&got)
		results <- got["id"].(string)
	}
	wg.Add(2)
	go claim(agentA)
	go claim(agentB)
	wg.Wait()
	close(results)
	seen := map[string]bool{}
	for id := range results {
		seen[id] = true
	}
	if len(seen) != 2 {
		t.Fatalf("concurrent claims should return two distinct threads; got %v", seen)
	}
}

func TestProcessReviewThread_Atomic(t *testing.T) {
	ctx := context.Background()
	agentID := lookupFixtureAgentID(t, ctx)
	issueID := seedPR2Issue(t, ctx, "PR2 process atomic")
	thread := seedPR2Thread(t, ctx, issueID, "issue", "unresolved", true)
	claimThread(t, issueID, agentID)

	w, req := pr2ThreadRequest("POST", "/api/issues/"+issueID+"/review-threads/"+thread.ThreadID+"/process", map[string]any{"content": "fixed"}, issueID, thread.ThreadID, agentID)
	testHandler.ProcessReviewThread(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("process: got %d: %s", w.Code, w.Body.String())
	}
	var processed bool
	if err := testPool.QueryRow(ctx, `
		SELECT processed_by_resolver_at IS NOT NULL
		  AND processed_by_agent = $2
		  AND claimed_by_agent IS NULL
		  AND claim_expires_at IS NULL
		FROM issue_review_thread WHERE id = $1
	`, thread.ThreadID, agentID).Scan(&processed); err != nil {
		t.Fatalf("verify processed: %v", err)
	}
	if !processed {
		t.Fatal("thread was not processed and claim-cleared atomically")
	}
	var replies int
	if err := testPool.QueryRow(ctx, `
		SELECT count(*)
		FROM comment reply
		JOIN comment parent ON parent.id = reply.parent_id
		WHERE reply.type = 'fixer_reply' AND parent.review_thread_id = $1
	`, thread.ThreadID).Scan(&replies); err != nil {
		t.Fatalf("count replies: %v", err)
	}
	if replies != 1 {
		t.Fatalf("fixer_reply count = %d, want 1", replies)
	}
}

func TestProcessReviewThread_409OnRetry(t *testing.T) {
	ctx := context.Background()
	agentID := lookupFixtureAgentID(t, ctx)
	issueID := seedPR2Issue(t, ctx, "PR2 process retry")
	thread := seedPR2Thread(t, ctx, issueID, "issue", "unresolved", true)
	claimThread(t, issueID, agentID)

	for i := 0; i < 2; i++ {
		w, req := pr2ThreadRequest("POST", "/api/issues/"+issueID+"/review-threads/"+thread.ThreadID+"/process", map[string]any{"content": "fixed"}, issueID, thread.ThreadID, agentID)
		testHandler.ProcessReviewThread(w, req)
		if i == 0 && w.Code != http.StatusOK {
			t.Fatalf("first process: got %d: %s", w.Code, w.Body.String())
		}
		if i == 1 && w.Code != http.StatusConflict {
			t.Fatalf("retry process: got %d: %s", w.Code, w.Body.String())
		}
	}
	var replies int
	_ = testPool.QueryRow(ctx, `SELECT count(*) FROM comment WHERE issue_id = $1 AND type = 'fixer_reply'`, issueID).Scan(&replies)
	if replies != 1 {
		t.Fatalf("retry created duplicate fixer_reply count=%d", replies)
	}
}

func TestProcessReviewThread_404IfNoParent(t *testing.T) {
	ctx := context.Background()
	agentID := lookupFixtureAgentID(t, ctx)
	issueID := seedPR2Issue(t, ctx, "PR2 no parent")
	thread := seedPR2Thread(t, ctx, issueID, "issue", "unresolved", false)
	claimThread(t, issueID, agentID)

	w, req := pr2ThreadRequest("POST", "/api/issues/"+issueID+"/review-threads/"+thread.ThreadID+"/process", map[string]any{"content": "fixed"}, issueID, thread.ThreadID, agentID)
	testHandler.ProcessReviewThread(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("process without parent: got %d: %s", w.Code, w.Body.String())
	}
}

func TestProcessReviewThread_StaleAndOtherAgentClaimsRejected(t *testing.T) {
	ctx := context.Background()
	agentA := lookupFixtureAgentID(t, ctx)
	agentB := createHandlerTestAgent(t, "PR2 Stale Agent B", []byte(`{}`))
	issueID := seedPR2Issue(t, ctx, "PR2 stale claim")
	thread := seedPR2Thread(t, ctx, issueID, "issue", "unresolved", true)

	claimThreadWithTTL(t, issueID, agentA, 1)
	w, req := pr2ThreadRequest("POST", "/api/issues/"+issueID+"/review-threads/"+thread.ThreadID+"/process", map[string]any{"content": "wrong agent"}, issueID, thread.ThreadID, agentB)
	testHandler.ProcessReviewThread(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("other agent process: got %d: %s", w.Code, w.Body.String())
	}

	time.Sleep(1100 * time.Millisecond)
	claimThread(t, issueID, agentB)
	w, req = pr2ThreadRequest("POST", "/api/issues/"+issueID+"/review-threads/"+thread.ThreadID+"/process", map[string]any{"content": "stale"}, issueID, thread.ThreadID, agentA)
	testHandler.ProcessReviewThread(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("stale agent process: got %d: %s", w.Code, w.Body.String())
	}
	w, req = pr2ThreadRequest("POST", "/api/issues/"+issueID+"/review-threads/"+thread.ThreadID+"/process", map[string]any{"content": "fixed"}, issueID, thread.ThreadID, agentB)
	testHandler.ProcessReviewThread(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("fresh agent process: got %d: %s", w.Code, w.Body.String())
	}
}

func TestNextUnpostedThread_FollowsParentLink(t *testing.T) {
	ctx := context.Background()
	agentID := lookupFixtureAgentID(t, ctx)
	issueID := seedPR2Issue(t, ctx, "PR2 next unposted")
	thread := seedPR2Thread(t, ctx, issueID, "issue", "unresolved", true)
	claimThread(t, issueID, agentID)
	w, req := pr2ThreadRequest("POST", "/api/issues/"+issueID+"/review-threads/"+thread.ThreadID+"/process", map[string]any{"content": "reply body"}, issueID, thread.ThreadID, agentID)
	testHandler.ProcessReviewThread(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("process: got %d: %s", w.Code, w.Body.String())
	}

	w, req = pr2Request("GET", "/api/issues/"+issueID+"/review-threads/next-unposted", nil, issueID, "")
	testHandler.GetNextUnpostedThread(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("next-unposted: got %d: %s", w.Code, w.Body.String())
	}
	var got map[string]any
	_ = json.NewDecoder(w.Body).Decode(&got)
	if got["thread_id"] != thread.ThreadID || got["reply_body"] != "reply body" {
		t.Fatalf("unexpected next-unposted response: %#v", got)
	}
}

func TestMarkThreadReplyPosted_StampOnly_NoGitHubCall(t *testing.T) {
	ctx := context.Background()
	agentID := lookupFixtureAgentID(t, ctx)
	issueID, threadID, _, fixerReplyID := seedFixerReplyChain(t, ctx)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM comment WHERE issue_id = $1`, issueID)
		testPool.Exec(ctx, `DELETE FROM issue_review_thread WHERE id = $1`, threadID)
		testPool.Exec(ctx, `DELETE FROM issue WHERE id = $1`, issueID)
	})

	w, req := pr2ThreadRequest("POST", "/api/issues/"+issueID+"/review-threads/"+threadID+"/mark-replied", map[string]any{"fixer_reply_comment_id": fixerReplyID}, issueID, threadID, agentID)
	testHandler.MarkThreadReplyPosted(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("mark-replied: got %d: %s", w.Code, w.Body.String())
	}
	var stamped bool
	if err := testPool.QueryRow(ctx, `SELECT posted_to_github_at IS NOT NULL FROM comment WHERE id = $1`, fixerReplyID).Scan(&stamped); err != nil {
		t.Fatalf("verify stamp: %v", err)
	}
	if !stamped {
		t.Fatal("posted_to_github_at was not stamped")
	}
}

func TestReviewThreadMutatingEndpoints_NonAgent401(t *testing.T) {
	ctx := context.Background()
	issueID := seedPR2Issue(t, ctx, "PR2 non-agent")
	thread := seedPR2Thread(t, ctx, issueID, "issue", "unresolved", true)
	oldReviewActions := testHandler.ReviewActions
	testHandler.ReviewActions = &githubintegration.ReviewActions{}
	t.Cleanup(func() { testHandler.ReviewActions = oldReviewActions })

	cases := []struct {
		name string
		call func(http.ResponseWriter, *http.Request)
		req  *http.Request
	}{
		{"next", testHandler.ClaimNextResolverThread, withURLParam(newRequest("POST", "/api/issues/"+issueID+"/review-threads/next", nil), "id", issueID)},
		{"release", testHandler.ReleaseThreadClaim, withURLParams(withURLParam(newRequest("POST", "/api/issues/"+issueID+"/review-threads/"+thread.ThreadID+"/release-claim", nil), "id", issueID), "threadID", thread.ThreadID)},
		{"process", testHandler.ProcessReviewThread, withURLParams(withURLParam(newRequest("POST", "/api/issues/"+issueID+"/review-threads/"+thread.ThreadID+"/process", map[string]any{"content": "x"}), "id", issueID), "threadID", thread.ThreadID)},
		{"mark-replied", testHandler.MarkThreadReplyPosted, withURLParams(withURLParam(newRequest("POST", "/api/issues/"+issueID+"/review-threads/"+thread.ThreadID+"/mark-replied", map[string]any{"fixer_reply_comment_id": thread.CRCommentID}), "id", issueID), "threadID", thread.ThreadID)},
		{"dismiss", testHandler.DismissPriorCRChangesRequested, withURLParam(newRequest("POST", "/api/issues/"+issueID+"/cr-review/dismiss-prior-changes-requested", nil), "id", issueID)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.call(w, tc.req)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("got %d, want 401: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestClaimNextResolverThread_DecodesChunkedTTL(t *testing.T) {
	ctx := context.Background()
	agentID := lookupFixtureAgentID(t, ctx)
	issueID := seedPR2Issue(t, ctx, "PR2 chunked TTL")
	seedPR2Thread(t, ctx, issueID, "issue", "unresolved", true)

	req := newRequest("POST", "/api/issues/"+issueID+"/review-threads/next", nil)
	req.Body = ioNopCloser{bytes.NewBufferString(`{"claim_ttl_secs":1}`)}
	req.ContentLength = -1
	req.Header.Set("X-Agent-ID", agentID)
	req = withURLParam(req, "id", issueID)
	w := httptest.NewRecorder()
	testHandler.ClaimNextResolverThread(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("chunked claim: got %d: %s", w.Code, w.Body.String())
	}
	var seconds float64
	if err := testPool.QueryRow(ctx, `SELECT EXTRACT(EPOCH FROM (claim_expires_at - now())) FROM issue_review_thread WHERE issue_id = $1`, issueID).Scan(&seconds); err != nil {
		t.Fatalf("read ttl: %v", err)
	}
	if seconds > 5 {
		t.Fatalf("chunked claim_ttl_secs ignored; expiry is %.2fs away", seconds)
	}
}

type ioNopCloser struct {
	*bytes.Buffer
}

func (ioNopCloser) Close() error { return nil }

func claimThread(t *testing.T, issueID, agentID string) {
	t.Helper()
	claimThreadWithTTL(t, issueID, agentID, 900)
}

func claimThreadWithTTL(t *testing.T, issueID, agentID string, ttl int) {
	t.Helper()
	w, req := pr2Request("POST", "/api/issues/"+issueID+"/review-threads/next", map[string]any{"claim_ttl_secs": ttl}, issueID, agentID)
	testHandler.ClaimNextResolverThread(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("claim thread: got %d: %s", w.Code, strings.TrimSpace(w.Body.String()))
	}
}
