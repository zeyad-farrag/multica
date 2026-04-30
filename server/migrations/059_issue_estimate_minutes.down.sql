BEGIN;

DROP INDEX IF EXISTS idx_issue_assignee_open;

ALTER TABLE issue
    DROP COLUMN IF EXISTS estimate_minutes;

COMMIT;
