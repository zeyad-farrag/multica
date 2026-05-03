-- 1014_comment_review_thread_unique.down.sql

DROP INDEX IF EXISTS idx_comment_review_thread_unique;

CREATE INDEX IF NOT EXISTS idx_comment_review_thread
    ON comment (review_thread_id)
    WHERE review_thread_id IS NOT NULL;
