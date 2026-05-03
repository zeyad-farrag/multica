-- name: UpsertReviewThread :one
INSERT INTO issue_review_thread (
    workspace_id, issue_id, pr_repo, pr_number,
    gh_comment_id, gh_thread_node_id,
    file_path, line, side, severity, title, body, url, author_login,
    severity_badge, effort_badge, ai_prompt
) VALUES (
    $1, $2, $3, $4,
    $5, sqlc.narg('gh_thread_node_id')::text,
    $6, sqlc.narg('line')::int, sqlc.narg('side')::text, $7, $8, $9, $10, $11,
    $12, $13, $14
)
ON CONFLICT (gh_comment_id) DO UPDATE SET
    body           = EXCLUDED.body,
    title          = EXCLUDED.title,
    severity       = EXCLUDED.severity,
    severity_badge = EXCLUDED.severity_badge,
    effort_badge   = EXCLUDED.effort_badge,
    ai_prompt      = EXCLUDED.ai_prompt,
    url            = EXCLUDED.url,
    file_path      = EXCLUDED.file_path,
    line           = EXCLUDED.line,
    side           = EXCLUDED.side,
    author_login   = EXCLUDED.author_login,
    gh_thread_node_id = COALESCE(EXCLUDED.gh_thread_node_id, issue_review_thread.gh_thread_node_id),
    updated_at     = now()
RETURNING *;

-- name: GetReviewThreadByCommentID :one
SELECT * FROM issue_review_thread
WHERE gh_comment_id = $1;

-- name: ListReviewThreadsByIssue :many
SELECT * FROM issue_review_thread
WHERE issue_id = $1
ORDER BY created_at ASC;

-- name: ListUnresolvedReviewThreadsByIssue :many
SELECT * FROM issue_review_thread
WHERE issue_id = $1 AND state = 'unresolved'
ORDER BY created_at ASC;

-- name: CountUnresolvedReviewThreadsByIssue :one
SELECT COUNT(*) FROM issue_review_thread
WHERE issue_id = $1 AND state = 'unresolved';

-- name: SetReviewThreadStateByThreadNodeID :execrows
UPDATE issue_review_thread SET
    state             = $2,
    resolved_at       = CASE WHEN $2 = 'resolved' THEN now() ELSE NULL END,
    resolved_by_agent = CASE WHEN $2 = 'resolved' THEN sqlc.narg('agent_id')::uuid ELSE NULL END,
    updated_at        = now()
WHERE gh_thread_node_id = $1;

-- name: SetReviewThreadStateByPR :execrows
UPDATE issue_review_thread SET
    state      = $3,
    updated_at = now()
WHERE pr_repo = $1 AND pr_number = $2;

-- name: SetReviewThreadStateByCommentID :execrows
UPDATE issue_review_thread SET
    state             = $2,
    gh_thread_node_id = COALESCE(sqlc.narg('gh_thread_node_id')::text, gh_thread_node_id),
    resolved_at       = CASE WHEN $2 = 'resolved' THEN now() ELSE NULL END,
    resolved_by_agent = CASE WHEN $2 = 'resolved' THEN sqlc.narg('agent_id')::uuid ELSE NULL END,
    updated_at        = now()
WHERE gh_comment_id = $1;

-- name: ListStuckCoderabbitIssues :many
-- Issues that have been parked in `coderabbit` long enough that we suspect
-- CodeRabbit streamed inline comments without ever submitting a wrapping
-- pull_request_review (rare, but observed on long PRs). The settle-window
-- sweeper runs this on a cadence and forces the coderabbit → resolving
-- transition for any matching issue.
--
-- Match conditions:
--   1. issue is currently in `coderabbit`
--   2. issue has at least one unresolved CR thread
--   3. the most recent CR thread is at least @settle_seconds old
--      (we hold off on issues whose CR comments are still landing — the
--      normal pull_request_review.submitted will fire shortly)
SELECT
    i.id          AS issue_id,
    i.workspace_id,
    i.title,
    i.pr_repo,
    i.pr_number,
    COUNT(t.*)::int AS unresolved_count,
    MAX(t.created_at) AS last_cr_comment_at
FROM issue i
JOIN issue_review_thread t ON t.issue_id = i.id
WHERE i.status = 'coderabbit'
  AND t.state = 'unresolved'
GROUP BY i.id
HAVING MAX(t.created_at) < now() - make_interval(secs => sqlc.arg(settle_seconds)::int)
ORDER BY i.id;

-- name: BackfillReviewThreadNodeID :execrows
UPDATE issue_review_thread SET
    gh_thread_node_id = $2,
    updated_at        = now()
WHERE gh_comment_id = $1
  AND (gh_thread_node_id IS NULL OR gh_thread_node_id = '');
