-- BMAD infrastructure for story-driven dev workflow:
--   1. Add 'ready_for_dev' to issue.status CHECK (between planning and in_progress)
--   2. Add BMAD comment kinds to comment.type CHECK:
--      debug, impl_plan, completion_note, change_log, review
--
-- Idempotent — re-running is safe.

-- (1) Extend issue.status with ready_for_dev
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'issue_status_check'
          AND pg_get_constraintdef(oid) LIKE '%ready_for_dev%'
    ) THEN
        ALTER TABLE issue DROP CONSTRAINT IF EXISTS issue_status_check;
        ALTER TABLE issue ADD CONSTRAINT issue_status_check CHECK (
            status = ANY (ARRAY[
                'backlog','todo','in_progress','in_review','done','blocked','cancelled',
                'planning','ready_for_dev','code_review','fixing','testing','checkpoint','staged'
            ])
        );
    END IF;
END $$;

-- (2) Extend comment.type with BMAD kinds
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'comment_type_check'
          AND pg_get_constraintdef(oid) LIKE '%impl_plan%'
    ) THEN
        ALTER TABLE comment DROP CONSTRAINT IF EXISTS comment_type_check;
        ALTER TABLE comment ADD CONSTRAINT comment_type_check CHECK (
            type = ANY (ARRAY[
                'comment','status_change','progress_update','system',
                'debug','impl_plan','completion_note','change_log','review'
            ])
        );
    END IF;
END $$;
