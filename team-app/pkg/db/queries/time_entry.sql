-- TIM-6 sqlc query stub covering time_entry. Production queries land in later stories.

-- name: GetTimeEntry :one
SELECT * FROM time_entry WHERE id = $1 LIMIT 1;
