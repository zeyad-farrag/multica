-- TIM-6 Strict inverse of 001_init.up.sql — drops the 12 tables in reverse creation
-- order so any FKs resolve cleanly. Indexes drop with their tables; no separate
-- DROP INDEX statements are needed.

BEGIN;

DROP TABLE IF EXISTS activity_log;
DROP TABLE IF EXISTS workload_anomaly;
DROP TABLE IF EXISTS time_confirm_history;
DROP TABLE IF EXISTS time_confirm;
DROP TABLE IF EXISTS time_entry;
DROP TABLE IF EXISTS work_item;
DROP TABLE IF EXISTS member_leave;
DROP TABLE IF EXISTS member_schedule;
DROP TABLE IF EXISTS mirror_issue;
DROP TABLE IF EXISTS mirror_member;
DROP TABLE IF EXISTS mirror_user;
DROP TABLE IF EXISTS mirror_workspace;

COMMIT;
