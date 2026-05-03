-- 1015_comment_posted_to_github.down.sql

DROP INDEX IF EXISTS idx_comment_fixer_reply_pending;

ALTER TABLE comment DROP COLUMN IF EXISTS posted_to_github_at;
