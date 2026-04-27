-- TIM-6 sqlc query stub covering member_schedule. Production queries land in story 1.9.

-- name: GetMemberSchedule :one
SELECT * FROM member_schedule WHERE member_id = $1 LIMIT 1;
