-- TIM-6 sqlc query stub covering member_leave. Production queries land in story 1.9.

-- name: GetMemberLeave :one
SELECT * FROM member_leave WHERE id = $1 LIMIT 1;
