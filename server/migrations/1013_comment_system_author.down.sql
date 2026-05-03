-- 1013_comment_system_author.down.sql
--
-- Best-effort revert. If any system-authored rows exist, this would
-- fail on re-imposing NOT NULL — caller must purge them first:
--   DELETE FROM comment WHERE author_type = 'system';

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'comment_author_type_check'
          AND pg_get_constraintdef(oid) LIKE '%system%'
    ) THEN
        ALTER TABLE comment DROP CONSTRAINT comment_author_type_check;
        ALTER TABLE comment ADD CONSTRAINT comment_author_type_check CHECK (
            author_type = ANY (ARRAY['member','agent'])
        );
    END IF;
END $$;

ALTER TABLE comment ALTER COLUMN author_id SET NOT NULL;
