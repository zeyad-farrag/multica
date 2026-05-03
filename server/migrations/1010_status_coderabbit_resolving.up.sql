-- 1010_status_coderabbit_resolving.up.sql
--
-- Splits the overloaded `in_review` status into three distinct columns:
--   coderabbit  — Marcus publishes the PR; CodeRabbit reviews. (NEW)
--   resolving   — Rosa addresses CR comments per-thread; drafts replies. (NEW)
--   in_review   — Marcus posts queued fixer_replies verbatim and resolves
--                 threads. (Semantics narrowed; was overloaded before.)
--
-- Flow:  testing -> coderabbit (Marcus publish, CR reviews)
--        coderabbit -> resolving (CR posted comments)
--        coderabbit -> staged (CR clean on first pass; skips in_review)
--        resolving  -> code_review (Rosa done, Quinn re-reviews)
--        code_review -> testing (Quinn approved)
--        testing -> in_review  (Murat GREEN AND previous_loop=cr)
--        in_review -> staged   (Marcus replied + resolved every thread)
--
-- Backfill: any rows currently sitting in `in_review` are migrated to
-- `coderabbit` if they have unresolved CR threads, otherwise left alone
-- (the existing predicate will flip them to `staged` on the next event).
-- Per the user, no live data on the dev server needs preservation, but
-- the backfill is conservative and safe to run on populated DBs too.

BEGIN;

-- (1) Extend the CHECK constraint to permit the new statuses.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'issue_status_check'
          AND pg_get_constraintdef(oid) LIKE '%coderabbit%'
    ) THEN
        ALTER TABLE issue DROP CONSTRAINT IF EXISTS issue_status_check;
        ALTER TABLE issue ADD CONSTRAINT issue_status_check CHECK (
            status = ANY (ARRAY[
                'backlog','todo','in_progress','in_review','done','blocked','cancelled',
                'planning','ready_for_dev','code_review','fixing','testing','staged',
                'coderabbit','resolving'
            ])
        );
    END IF;
END $$;

-- (2) Backfill: rows in `in_review` with unresolved CR threads move to
-- `coderabbit` (the new "waiting for CR" lane). Rows with no unresolved
-- threads stay in `in_review` — they are either already-clean PRs awaiting
-- the staged predicate or rows that have already passed through resolving
-- and are awaiting Marcus's reply+resolve pass.
UPDATE issue i
   SET status = 'coderabbit',
       updated_at = now()
 WHERE i.status = 'in_review'
   AND EXISTS (
       SELECT 1 FROM issue_review_thread t
        WHERE t.issue_id = i.id
          AND t.state = 'unresolved'
   );

COMMIT;
