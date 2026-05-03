-- 1011_review_thread_badges.down.sql

ALTER TABLE issue_review_thread
    DROP COLUMN IF EXISTS severity_badge,
    DROP COLUMN IF EXISTS effort_badge,
    DROP COLUMN IF EXISTS ai_prompt;
