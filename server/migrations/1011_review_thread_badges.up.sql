-- 1011_review_thread_badges.up.sql
--
-- CodeRabbit's review-comment markdown carries three badges on the first
-- line and an AI-prompt block inside a `<details>` near the end. The
-- existing parser only captures `severity` (Issue / Refactor / Nitpick /
-- Suggestion) and a one-line title. To drive Rosa's resolution loop and
-- the per-thread UI rendering, we also need:
--
--   - severity_badge: Critical / Major / Minor / Trivial / Blocker
--     (CR uses 🔴 / 🟠 / 🟡 / 🔵 / others)
--   - effort_badge:   Quick win / Heavy lift / Poor tradeoff / Low value
--     (CR uses ⚡ / 🐢 / ⚖️ / 💤; optional — not all comments carry it)
--   - ai_prompt:      the verbatim fenced block under
--     `<details><summary>🤖 Prompt for AI Agents</summary>` — Rosa feeds
--     this directly into her patch loop.
--
-- Columns are NOT NULL with a sentinel default of 'unknown' / '' so the
-- backfill is a no-op. New ingest paths populate them via parseCRBody.

ALTER TABLE issue_review_thread
    ADD COLUMN IF NOT EXISTS severity_badge TEXT NOT NULL DEFAULT 'unknown',
    ADD COLUMN IF NOT EXISTS effort_badge   TEXT NOT NULL DEFAULT 'unknown',
    ADD COLUMN IF NOT EXISTS ai_prompt      TEXT NOT NULL DEFAULT '';
