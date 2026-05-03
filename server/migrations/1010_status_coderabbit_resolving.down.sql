-- 1010_status_coderabbit_resolving.down.sql
--
-- Reverses 1010. Coalesces any rows currently in the new statuses back to
-- `in_review` (the closest neighbor in the previous flow), then removes
-- the new values from the CHECK constraint.

BEGIN;

UPDATE issue
   SET status = 'in_review',
       updated_at = now()
 WHERE status IN ('coderabbit','resolving');

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'issue_status_check'
          AND pg_get_constraintdef(oid) LIKE '%coderabbit%'
    ) THEN
        ALTER TABLE issue DROP CONSTRAINT issue_status_check;
        ALTER TABLE issue ADD CONSTRAINT issue_status_check CHECK (
            status = ANY (ARRAY[
                'backlog','todo','in_progress','in_review','done','blocked','cancelled',
                'planning','ready_for_dev','code_review','fixing','testing','staged'
            ])
        );
    END IF;
END $$;

COMMIT;
