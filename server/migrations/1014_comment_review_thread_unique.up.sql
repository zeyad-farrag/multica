-- 1014_comment_review_thread_unique.up.sql
--
-- Tighten the review_thread_id index to UNIQUE WHERE NOT NULL so that
-- the upsert query can target it via ON CONFLICT. Each CR review thread
-- maps to exactly one cr_review_comment row in the timeline; the index
-- enforces that invariant and gives the webhook handler an idempotent
-- ingest path.

DROP INDEX IF EXISTS idx_comment_review_thread;

CREATE UNIQUE INDEX IF NOT EXISTS idx_comment_review_thread_unique
    ON comment (review_thread_id)
    WHERE review_thread_id IS NOT NULL;
