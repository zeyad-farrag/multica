BEGIN;

ALTER TABLE issue
    ADD COLUMN estimate_minutes INT NULL
    CHECK (estimate_minutes IS NULL OR estimate_minutes > 0);

CREATE INDEX idx_issue_assignee_open
    ON issue (assignee_type, assignee_id, status)
    WHERE status IN ('todo', 'in_progress', 'planning', 'ready_for_dev', 'fixing', 'testing');

COMMIT;
