-- name: ListActivities :many
SELECT * FROM activity_log
WHERE issue_id = $1
ORDER BY created_at ASC
LIMIT $2 OFFSET $3;

-- name: CreateActivity :one
INSERT INTO activity_log (
    workspace_id, issue_id, actor_type, actor_id, action, details
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: CountAssigneeChangesByActor :many
-- Count how many times a user assigned each target via assignee_changed activities.
SELECT
  details->>'to_type' as assignee_type,
  details->>'to_id' as assignee_id,
  COUNT(*)::bigint as frequency
FROM activity_log
WHERE workspace_id = $1
  AND actor_id = $2
  AND actor_type = 'member'
  AND action = 'assignee_changed'
  AND details->>'to_type' IS NOT NULL
  AND details->>'to_id' IS NOT NULL
GROUP BY details->>'to_type', details->>'to_id';

-- SPEC: §6.1 #5, §19.17, §22 M-PR#3 — Story 1.4 read portion. Workspace-
-- scoped scan for the team-app reconciler's gate-bypass detection.
-- Optional action/actor_id filters; tuple cursor on (created_at, id).

-- name: ListActivityByWorkspace :many
SELECT id, workspace_id, issue_id, actor_type, actor_id, action, details, created_at
FROM activity_log
WHERE workspace_id = $1
  AND created_at >= $2
  AND (sqlc.narg('action')::text IS NULL OR action = sqlc.narg('action'))
  AND (sqlc.narg('actor_id')::uuid IS NULL OR actor_id = sqlc.narg('actor_id'))
  AND (
        sqlc.narg('cursor_created_at')::timestamptz IS NULL
     OR (created_at, id) > (sqlc.narg('cursor_created_at')::timestamptz, sqlc.narg('cursor_id')::uuid)
  )
ORDER BY created_at ASC, id ASC
LIMIT $3;

-- name: CountActivityByWorkspace :one
SELECT count(*) FROM activity_log
WHERE workspace_id = $1
  AND created_at >= $2
  AND (sqlc.narg('action')::text IS NULL OR action = sqlc.narg('action'))
  AND (sqlc.narg('actor_id')::uuid IS NULL OR actor_id = sqlc.narg('actor_id'));
