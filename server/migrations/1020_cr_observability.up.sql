ALTER TABLE cr_review_attempt
    ADD COLUMN IF NOT EXISTS first_signal_at   TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS first_signal_kind TEXT
      CHECK (first_signal_kind IS NULL OR first_signal_kind IN
        ('check_run','issue_comment','review_comment','review','thread'));

CREATE INDEX IF NOT EXISTS idx_cr_review_attempt_silent_check
    ON cr_review_attempt (started_at)
    WHERE first_signal_at IS NULL AND closed_at IS NULL;

CREATE TABLE IF NOT EXISTS cr_review_signal (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attempt_id      UUID NOT NULL REFERENCES cr_review_attempt(id) ON DELETE CASCADE,
    signal_kind     TEXT NOT NULL CHECK (signal_kind IN
        ('check_run','issue_comment','review_comment','review','thread')),
    signal_action   TEXT,
    received_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    payload_summary JSONB
);

CREATE INDEX IF NOT EXISTS idx_cr_review_signal_attempt
    ON cr_review_signal (attempt_id, received_at);

ALTER TABLE workspace_repo_binding
    ADD COLUMN IF NOT EXISTS cr_required BOOLEAN NOT NULL DEFAULT true;
