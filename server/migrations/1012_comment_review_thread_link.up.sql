-- 1012_comment_review_thread_link.up.sql
--
-- First-class rendering of CodeRabbit comments + Rosa's per-thread fix
-- replies in the issue comments timeline. Two changes:
--
-- 1. Extend comment.type CHECK with 'cr_review_comment' and 'fixer_reply'.
--    The webhook handler upserts one cr_review_comment row per CR thread
--    (linked via review_thread_id), and Rosa POSTs fixer_reply child
--    comments under each cr_review_comment via the existing /comments API
--    (parent_id reuses the existing comment.parent_id column from
--    migration 017).
--
-- 2. Add comment.review_thread_id (nullable FK to issue_review_thread).
--    Only populated for cr_review_comment rows. ON DELETE SET NULL so
--    that purging a thread doesn't cascade into the comments timeline —
--    we keep the audit trail; the row just unlinks.
--
-- The partial index makes "find the comment for thread X" O(log n) for
-- the ingest path's idempotent upsert.

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'comment_type_check'
          AND pg_get_constraintdef(oid) LIKE '%cr_review_comment%'
    ) THEN
        ALTER TABLE comment DROP CONSTRAINT IF EXISTS comment_type_check;
        ALTER TABLE comment ADD CONSTRAINT comment_type_check CHECK (
            type = ANY (ARRAY[
                'comment','status_change','progress_update','system',
                'debug','impl_plan','completion_note','change_log','review',
                'cr_review_comment','fixer_reply'
            ])
        );
    END IF;
END $$;

ALTER TABLE comment
    ADD COLUMN IF NOT EXISTS review_thread_id UUID
    REFERENCES issue_review_thread(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_comment_review_thread
    ON comment (review_thread_id)
    WHERE review_thread_id IS NOT NULL;
