-- Revert: remove ready_for_dev from issue.status CHECK,
-- and remove BMAD kinds from comment.type CHECK.
-- Only runs if rows using those values have been cleaned up first.

DO $$
BEGIN
    ALTER TABLE issue DROP CONSTRAINT IF EXISTS issue_status_check;
    ALTER TABLE issue ADD CONSTRAINT issue_status_check CHECK (
        status = ANY (ARRAY[
            'backlog','todo','in_progress','in_review','done','blocked','cancelled',
            'planning','code_review','fixing','testing','checkpoint','staged'
        ])
    );
END $$;

DO $$
BEGIN
    ALTER TABLE comment DROP CONSTRAINT IF EXISTS comment_type_check;
    ALTER TABLE comment ADD CONSTRAINT comment_type_check CHECK (
        type = ANY (ARRAY[
            'comment','status_change','progress_update','system'
        ])
    );
END $$;
