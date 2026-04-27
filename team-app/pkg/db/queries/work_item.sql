-- TIM-6 sqlc query stub covering work_item. Production queries land in later stories.

-- name: GetWorkItem :one
SELECT * FROM work_item WHERE id = $1 LIMIT 1;
