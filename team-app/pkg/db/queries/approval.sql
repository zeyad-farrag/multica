-- TIM-6 sqlc query stubs covering time_confirm and time_confirm_history. Production
-- queries (approval workflow, rollups) land in later stories.

-- name: GetTimeConfirm :one
SELECT * FROM time_confirm WHERE id = $1 LIMIT 1;

-- name: GetTimeConfirmHistory :one
SELECT * FROM time_confirm_history WHERE id = $1 LIMIT 1;
