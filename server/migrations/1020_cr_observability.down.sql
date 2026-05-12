ALTER TABLE workspace_repo_binding
    DROP COLUMN IF EXISTS cr_required;

DROP INDEX IF EXISTS idx_cr_review_signal_attempt;
DROP TABLE IF EXISTS cr_review_signal;

DROP INDEX IF EXISTS idx_cr_review_attempt_silent_check;
ALTER TABLE cr_review_attempt
    DROP COLUMN IF EXISTS first_signal_kind,
    DROP COLUMN IF EXISTS first_signal_at;
