-- 1012_comment_review_thread_link.down.sql

DROP INDEX IF EXISTS idx_comment_review_thread;

ALTER TABLE comment DROP COLUMN IF EXISTS review_thread_id;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'comment_type_check'
          AND pg_get_constraintdef(oid) LIKE '%cr_review_comment%'
    ) THEN
        ALTER TABLE comment DROP CONSTRAINT comment_type_check;
        ALTER TABLE comment ADD CONSTRAINT comment_type_check CHECK (
            type = ANY (ARRAY[
                'comment','status_change','progress_update','system',
                'debug','impl_plan','completion_note','change_log','review'
            ])
        );
    END IF;
END $$;
