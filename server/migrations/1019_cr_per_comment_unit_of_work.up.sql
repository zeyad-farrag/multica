ALTER TABLE issue_review_thread
    ADD COLUMN IF NOT EXISTS processed_by_resolver_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS processed_by_agent       UUID REFERENCES agent(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS claimed_by_agent         UUID REFERENCES agent(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS claim_expires_at         TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_issue_review_thread_claimable
    ON issue_review_thread (issue_id, file_path, line)
    WHERE state = 'unresolved' AND processed_by_resolver_at IS NULL;
