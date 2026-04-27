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

-- SPEC: §6.1 #6, §22 M-PR#3 — Story 1.4 read portion. Backfill query for the
-- standalone autofill. Filter is (workspace, author_id, type, [start,end))
-- with tuple cursor on (created_at, id). Note: NO author_type='member'
-- filter — agent-authored comments must be returned (epic 1.4 AC + spec §14
-- override the §6.1 #6 line that says otherwise; the standalone caller
-- filters on its side).

-- name: ListCommentsByAuthorTypeDate :many
SELECT id, issue_id, author_type, author_id, content, type,
       created_at, updated_at, parent_id, workspace_id
FROM comment
WHERE workspace_id = $1
  AND author_id = $2
  AND type = $3
  AND created_at >= $4
  AND created_at < $5
  AND (
        sqlc.narg('cursor_created_at')::timestamptz IS NULL
     OR (created_at, id) > (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
  )
ORDER BY created_at ASC, id ASC
LIMIT $6;

-- name: CountCommentsByAuthorTypeDate :one
SELECT count(*) FROM comment
WHERE workspace_id = $1
  AND author_id = $2
  AND type = $3
  AND created_at >= $4
  AND created_at < $5;
