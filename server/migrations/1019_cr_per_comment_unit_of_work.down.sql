DROP INDEX IF EXISTS idx_issue_review_thread_claimable;

ALTER TABLE issue_review_thread
    DROP COLUMN IF EXISTS processed_by_resolver_at,
    DROP COLUMN IF EXISTS processed_by_agent,
    DROP COLUMN IF EXISTS claimed_by_agent,
    DROP COLUMN IF EXISTS claim_expires_at;
