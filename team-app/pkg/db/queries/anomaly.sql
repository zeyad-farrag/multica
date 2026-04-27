-- TIM-6 sqlc query stub covering workload_anomaly. Production queries land in later stories.

-- name: GetWorkloadAnomaly :one
SELECT * FROM workload_anomaly WHERE id = $1 LIMIT 1;
