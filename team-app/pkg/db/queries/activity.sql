-- TIM-6 sqlc query stub covering activity_log. Production queries land in later stories.

-- name: GetActivityLog :one
SELECT * FROM activity_log WHERE id = $1 LIMIT 1;
