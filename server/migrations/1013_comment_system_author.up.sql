-- 1013_comment_system_author.up.sql
--
-- CR-authored review comments stored as type='cr_review_comment' have no
-- Multica user backing them. Extend the comment table to permit a
-- system-authored row:
--
--   author_type IN ('member','agent','system')
--   author_id may be NULL when author_type='system'
--
-- The generated Go code already uses pgtype.UUID for AuthorID (i.e.
-- supports NULL); only the schema currently disallows it.

ALTER TABLE comment ALTER COLUMN author_id DROP NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'comment_author_type_check'
          AND pg_get_constraintdef(oid) LIKE '%system%'
    ) THEN
        ALTER TABLE comment DROP CONSTRAINT IF EXISTS comment_author_type_check;
        ALTER TABLE comment ADD CONSTRAINT comment_author_type_check CHECK (
            author_type = ANY (ARRAY['member','agent','system'])
        );
    END IF;
END $$;
