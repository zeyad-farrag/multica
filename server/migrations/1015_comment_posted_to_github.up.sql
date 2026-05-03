-- 1015_comment_posted_to_github.up.sql
--
-- Track when Rosa's `fixer_reply` rows have been mirrored to GitHub by
-- Marcus's `bmad-pr-resolve` skill. The column is nullable and only
-- meaningful for type='fixer_reply' rows; any other type leaves it NULL.
--
-- The UI uses this to show a "Pending" / "Posted to GitHub" pill on each
-- fixer_reply rendered under its parent cr_review_comment. Marcus stamps
-- it after a successful POST /api/issues/{id}/review-threads/{threadID}/reply
-- (which calls GitHub's GraphQL addPullRequestReviewThreadReply mutation).
-- A NULL value means "not yet posted", which is the default state right
-- after Rosa drafts the reply.
--
-- Partial index speeds up Marcus's lookup of "fixer_reply rows still
-- pending" without scanning every comment row in the DB. Pending = type
-- is fixer_reply AND posted_to_github_at IS NULL.

ALTER TABLE comment
    ADD COLUMN IF NOT EXISTS posted_to_github_at TIMESTAMPTZ;

-- fixer_reply rows carry parent_id (pointing at the cr_review_comment) and
-- leave review_thread_id NULL — the parent's review_thread_id is the source
-- of truth. Index on parent_id so Marcus's "what's still pending under
-- this thread's cr_review_comment row?" lookup is constant-time.
CREATE INDEX IF NOT EXISTS idx_comment_fixer_reply_pending
    ON comment (parent_id)
    WHERE type = 'fixer_reply' AND posted_to_github_at IS NULL;
