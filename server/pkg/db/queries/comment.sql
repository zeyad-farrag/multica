-- name: ListComments :many
SELECT * FROM comment
WHERE issue_id = $1 AND workspace_id = $2
ORDER BY created_at ASC;

-- name: ListCommentsPaginated :many
SELECT * FROM comment
WHERE issue_id = $1 AND workspace_id = $2
ORDER BY created_at ASC
LIMIT $3 OFFSET $4;

-- name: ListCommentsSince :many
SELECT * FROM comment
WHERE issue_id = $1 AND workspace_id = $2 AND created_at > $3
ORDER BY created_at ASC;

-- name: ListCommentsSincePaginated :many
SELECT * FROM comment
WHERE issue_id = $1 AND workspace_id = $2 AND created_at > $3
ORDER BY created_at ASC
LIMIT $4 OFFSET $5;

-- name: CountComments :one
SELECT count(*) FROM comment
WHERE issue_id = $1 AND workspace_id = $2;

-- name: GetComment :one
SELECT * FROM comment
WHERE id = $1;

-- name: GetCommentInWorkspace :one
SELECT * FROM comment
WHERE id = $1 AND workspace_id = $2;

-- name: SystemListCommentsByWorkspace :many
SELECT c.*
FROM comment c
JOIN issue i ON i.id = c.issue_id AND i.workspace_id = c.workspace_id
WHERE c.workspace_id = @workspace_id
  AND (sqlc.narg('author_id')::uuid IS NULL OR c.author_id = sqlc.narg('author_id')::uuid)
  AND (sqlc.narg('comment_type')::text IS NULL OR c.type = sqlc.narg('comment_type')::text)
  AND (sqlc.narg('comment_date')::date IS NULL OR c.created_at::date = sqlc.narg('comment_date')::date)
ORDER BY c.created_at ASC, c.id ASC
LIMIT @limit_count;

-- name: CreateComment :one
INSERT INTO comment (issue_id, workspace_id, author_type, author_id, content, type, parent_id)
VALUES ($1, $2, $3, $4, $5, $6, sqlc.narg(parent_id))
RETURNING *;

-- name: UpsertCRReviewComment :one
-- Idempotent insert/update of a CodeRabbit review comment as a first-class
-- timeline entry. Keyed on review_thread_id so re-deliveries of the same
-- pull_request_review_comment.created/edited webhook update content in
-- place rather than creating duplicates.
INSERT INTO comment (
    issue_id, workspace_id, author_type, author_id, content, type, review_thread_id
) VALUES (
    $1, $2, 'system', NULL, $3, 'cr_review_comment', $4
)
ON CONFLICT (review_thread_id)
WHERE review_thread_id IS NOT NULL
DO UPDATE SET
    content    = EXCLUDED.content,
    updated_at = now()
RETURNING *;

-- name: MarkFixerReplyPosted :one
-- Stamp posted_to_github_at on a fixer_reply row after Marcus successfully
-- mirrors it to GitHub via the review-threads/{id}/reply endpoint. Idempotent:
-- a second call returns the row with its existing timestamp unchanged.
UPDATE comment SET
    posted_to_github_at = COALESCE(posted_to_github_at, now()),
    updated_at          = now()
WHERE id = $1 AND type = 'fixer_reply'
RETURNING *;

-- name: ListFixerRepliesForThread :many
-- Returns Rosa's queued fixer_reply rows for a single review thread, oldest
-- first. Joins through the parent cr_review_comment row so we resolve via
-- parent_id (the UI's nesting key) rather than duplicating review_thread_id
-- onto every reply. Marcus's bmad-pr-resolve skill walks these rows,
-- posts each content verbatim to GitHub, and resolves the thread.
SELECT c.id, c.issue_id, c.author_type, c.author_id, c.content, c.type, c.created_at, c.updated_at, c.parent_id, c.workspace_id, c.review_thread_id
FROM comment c
JOIN comment p ON c.parent_id = p.id
WHERE p.review_thread_id = $1
  AND p.type = 'cr_review_comment'
  AND c.type = 'fixer_reply'
ORDER BY c.created_at ASC;

-- name: UpdateComment :one
UPDATE comment SET
    content = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: HasAgentCommentedSince :one
SELECT EXISTS (
    SELECT 1 FROM comment
    WHERE issue_id = @issue_id
      AND author_type = 'agent'
      AND author_id = @author_id
      AND created_at >= @since
) AS commented;

-- name: HasAgentRepliedInThread :one
-- Returns true if the given agent has posted a reply in the thread rooted at
-- the specified parent comment. Used to detect agent participation in a
-- member-started thread so that follow-up member replies still trigger the agent.
SELECT count(*) > 0 AS has_replied FROM comment
WHERE parent_id = @parent_id AND author_type = 'agent' AND author_id = @agent_id;

-- name: DeleteComment :exec
DELETE FROM comment WHERE id = $1;
