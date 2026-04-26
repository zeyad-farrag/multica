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

-- name: CreateComment :one
INSERT INTO comment (issue_id, workspace_id, author_type, author_id, content, type, parent_id)
VALUES ($1, $2, $3, $4, $5, $6, sqlc.narg(parent_id))
RETURNING *;

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

-- name: ListCommentsByAuthorTypeDate :many
-- SPEC: §6.1 #6 — M-PR#3 read portion (Story 1.4 / TIM-9).
-- Workspace-scoped backfill source for the standalone autofill agent.
-- The [created_at_start, created_at_end) window is computed by the handler
-- in the workspace timezone (UTC default). NOTE: deliberately does NOT
-- filter by author_type — the standalone caller filters per spec §14 and
-- the epic's AC #5. See ListCommentsForBackfill in comment.go.
SELECT * FROM comment
WHERE workspace_id = $1
  AND author_id = $2
  AND type = $3
  AND created_at >= $4
  AND created_at < $5
  AND (sqlc.narg('cursor_created_at')::timestamptz IS NULL
       OR (created_at, id) > (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid))
ORDER BY created_at ASC, id ASC
LIMIT $6;

-- name: CountCommentsByAuthorTypeDate :one
-- SPEC: §6.1 #6 — M-PR#3 read portion (Story 1.4 / TIM-9).
SELECT count(*) FROM comment
WHERE workspace_id = $1
  AND author_id = $2
  AND type = $3
  AND created_at >= $4
  AND created_at < $5;
