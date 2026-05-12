CREATE TABLE IF NOT EXISTS cr_review_attempt (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_id             UUID NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    workspace_id         UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    cr_round             INT NOT NULL CHECK (cr_round >= 0),
    pr_url               TEXT NOT NULL,
    head_sha             TEXT NOT NULL,
    started_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    review_submitted_at  TIMESTAMPTZ,
    review_state         TEXT CHECK (review_state IS NULL OR review_state IN
        ('approved','changes_requested','commented','dismissed')),
    findings_count       INT NOT NULL DEFAULT 0 CHECK (findings_count >= 0),
    outcome              TEXT CHECK (outcome IS NULL OR outcome IN
        ('completed_clean','completed_with_findings','silent_partial','silent_total','failed','skipped')),
    outcome_reason       TEXT,
    closed_at            TIMESTAMPTZ,
    CONSTRAINT cr_review_attempt_round_per_issue UNIQUE (issue_id, cr_round),
    CONSTRAINT cr_review_attempt_outcome_requires_close
        CHECK ((outcome IS NULL AND closed_at IS NULL) OR (outcome IS NOT NULL AND closed_at IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS idx_cr_review_attempt_pending_settle
    ON cr_review_attempt (review_submitted_at)
    WHERE review_state = 'commented' AND outcome IS NULL;

CREATE INDEX IF NOT EXISTS idx_cr_review_attempt_issue
    ON cr_review_attempt (issue_id, cr_round DESC);
