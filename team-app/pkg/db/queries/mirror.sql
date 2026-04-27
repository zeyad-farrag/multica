-- TIM-6 sqlc query stubs covering mirror_workspace, mirror_user, mirror_member, mirror_issue.
-- Production queries land in stories 1.7 (WS subscriber) and 1.8 (reconciler). These
-- stubs exist so sqlc can compile every column type against the 001 schema.

-- name: GetMirrorWorkspace :one
SELECT * FROM mirror_workspace WHERE id = $1 LIMIT 1;

-- name: GetMirrorUser :one
SELECT * FROM mirror_user WHERE id = $1 LIMIT 1;

-- name: GetMirrorMember :one
SELECT * FROM mirror_member WHERE id = $1 LIMIT 1;

-- name: GetMirrorIssue :one
SELECT * FROM mirror_issue WHERE id = $1 LIMIT 1;
