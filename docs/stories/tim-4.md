# TIM-4 — Story 1.4: Multica fork — read endpoints for reconciler & autofill backfill (M-PR#3 read portion)

**Status:** ready-for-dev
**Branch:** `multica/tim-4` (single shared issue branch; Marcus pushes to origin at `in_review`)
**Base:** `bmad-self-host`
**Repos:** `zeyad-farrag/multica` only — TimeTrack is **not** touched in this story.

The canonical specification (Story narrative, full AC text, Tasks/Subtasks, Dev Notes, References) lives on the Multica issue description. This Story File is the architect's working summary — it captures the impl-plan delta against current code and the decisions Amelia needs to act on, without re-typing 10k characters of canonical content.

---

## Story

As a standalone team-app, I want REST endpoints on Multica that return issues / comments / activity filtered by cursor and date, so that my reconciler can sync mirror tables and my autofill can backfill historical status changes.

## Scope (CRITICAL)

THIS STORY ships only the M-PR#3 **read-endpoint portion**:

- `GET /api/workspaces/{id}/issues?updated_since=...` (cursor-paginated)
- `GET /api/workspaces/{id}/comments?author_id=...&type=...&date=...`
- `GET /api/workspaces/{id}/activity?since=...&action=...&actor_id=...`
- `GET /api/workspaces/{id}` `settings.work_week` pass-through (regression test only — `WorkspaceResponse.Settings` is already `any` JSON, no code change needed)

DO NOT implement gate hooks, `inbox.go` POST, or `TEAM_APP_URL` wiring — those belong to Stories 5.7 and 6.1.

## Acceptance Criteria summary

1. Issues — incremental cursor sync with `(updated_at, id)` ordering; default 200 / max 1000.
2. Issues — backwards compatibility: bare `GET /api/issues` (no `updated_since`) is byte-for-byte unchanged.
3. Issues — cursor decoding strict-after; malformed cursor → 400 `{"error":"invalid_cursor"}`.
4. Comments — filter by `author_id`, `type`, `date` (workspace-tz day window); pagination identical.
5. Comments — agent-authored comments are returned (standalone app filters).
6. Activity — filter by `since`, optional `action`, optional `actor_id`; pagination identical.
7. Auth — PAT required (existing `middleware.Auth(queries)`); 401 when missing/invalid.
8. Membership — non-member → 403 (existing `RequireWorkspaceMemberFromURL`).
9. No DB schema changes — code-only.
10. `make sqlc` regenerates cleanly.
11. `make check` is green; new unit and integration tests cover the cases listed in the issue description AC #11.

Full AC text is on the issue. AC numbering matches.

## Resolved ambiguities (architect decisions Amelia should not re-litigate)

1. **Workspace timezone source.** `workspace.timezone` does NOT exist as a column. Settings is a `JSONB` column on `workspace`. Read the timezone from `workspace.settings->>'timezone'` (string IANA name, e.g. `Europe/Berlin`); on missing/empty/invalid (`time.LoadLocation` returns error), fall back to `time.UTC`. Compute the `[start, end)` window as the calendar day in that location, then convert to UTC for the SQL bound. Document this two-line policy at the top of the comments handler.

2. **`author_type` policy on the comments endpoint.** The endpoint MUST return both `member` and `agent` rows. The standalone app filters on its side (per epic AC + spec §14). Spec §6.1 #6 line 994 says the opposite — disregard it; AC #5 + §14 + §22 are canonical. Add the explanatory `// NOTE: ...` comment in `comment.go` per the issue's Subtask 2.6.

3. **Issues handler split.** Do NOT widen the existing `ListIssues` handler at `/api/issues` (header-driven workspace). Add a separate `ListIssuesUpdatedSinceForWorkspace` handler that reads `chi.URLParam(r, "id")` and is mounted on the workspace-scoped path. The legacy `/api/issues` route stays untouched, satisfying AC #2 byte-for-byte. The two handlers may share a private helper that runs the cursor body once `workspaceID` is resolved.

4. **`next_cursor` semantics — pick option (a).** Request `limit+1` from SQL, return `limit` items, set `next_cursor` only if SQL returned `limit+1`. Document the choice as a comment in `cursor.go`. This avoids the trailing empty-page request.

5. **`total` is included.** Match the existing `/api/issues` shape: response includes `total` (count of rows matching the filter, ignoring cursor). Add `Count*` queries alongside the list queries (`CountIssuesUpdatedSince`, `CountCommentsByAuthorTypeDate`, `CountActivityByWorkspace`).

6. **`enrichIssuesWithLabels`.** The cursor-path response includes labels — call `h.enrichIssuesWithLabels(ctx, resp)` on the new handler too, mirroring the legacy path. (Existing code calls it three times across `issue.go` at lines 793, 856, 917.)

7. **`make sqlc` rule.** Run `make sqlc` and commit the regenerated files. Do not hand-edit anything under `server/pkg/db/generated/`.

## Tasks / Subtasks (execution order — top-down)

The full subtask body is on the issue description. Below is the ordered task list with **corrected file/line references against the live tree** (the issue description's line numbers are off by ~150 lines in `router.go` and ~165 in `issue.go` — they were written against an older snapshot).

| # | Task | Live file references | AC |
|---|------|----------------------|----|
| 1 | Add `cursor.go` helpers (`parseCursor`, `encodeCursor`, `parseLimit(default, max)`) | NEW `server/internal/handler/cursor.go` | 1, 3, 4, 6 |
| 2 | Issues read endpoint with cursor | `server/pkg/db/queries/issue.sql` (existing `ListIssues` at line 1; append `ListIssuesUpdatedSince` + `CountIssuesUpdatedSince`); `server/internal/handler/issue.go` (`ListIssues` at **line 741**, `enrichIssuesWithLabels` calls at lines 793/856/917) | 1, 2, 3, 11 |
| 3 | Comments read endpoint | `server/pkg/db/queries/comment.sql` (pattern: `ListCommentsSincePaginated` line 17); `server/internal/handler/comment.go` (existing per-issue `ListComments` at line 56 — DO NOT MODIFY); workspace-tz day window per ambiguity #1 | 4, 5, 11 |
| 4 | Activity read endpoint | `server/pkg/db/queries/activity.sql` (existing `ListActivities` at line 1); `server/internal/handler/activity.go` (`ListTimeline` at line 38, `GetAssigneeFrequency` at line 128 — DO NOT MODIFY) | 6, 11 |
| 5 | Wire routes inside the existing member-gated workspace group | `server/cmd/server/router.go`: outer `Auth` group at **line 220**; `r.Route("/api/workspaces", ...)` at **line 235**; per-workspace `r.Route("/{id}", ...)` at **line 238**; `RequireWorkspaceMemberFromURL(queries, "id")` at **line 241**. Insert the three new `r.Get(...)` routes inside that inner member-gated block. The bare `/api/issues` route (line 286 in the same file) stays as-is. | 1, 4, 6, 7, 8 |
| 6 | Tests | New `*_test.go` files next to handlers; integration test in `server/cmd/server/integration_test.go`; coverage list per the issue's AC #11 | 11 |
| 7 | `settings.work_week` pass-through regression | `server/internal/handler/workspace.go` `WorkspaceResponse.Settings` is already `any` (raw JSON unmarshal at line 48–55). Add a regression test only — confirm an arbitrary `settings.work_week` JSON survives the round-trip unchanged. | scope-table row 4 |
| 8 | `make sqlc && make check` | from repo root | 10, 11 |

## Affected repos

| Repo | Branch | Default base |
|---|---|---|
| `zeyad-farrag/multica` | `multica/tim-4` | `bmad-self-host` |

## Files to touch

| Path | Change | Why |
|---|---|---|
| `server/pkg/db/queries/issue.sql` | edit (append) | AC-1, AC-3 — `ListIssuesUpdatedSince`, `CountIssuesUpdatedSince` |
| `server/pkg/db/queries/comment.sql` | edit (append) | AC-4 — `ListCommentsByAuthorTypeDate`, `CountCommentsByAuthorTypeDate` |
| `server/pkg/db/queries/activity.sql` | edit (append) | AC-6 — `ListActivityByWorkspace`, `CountActivityByWorkspace` |
| `server/pkg/db/generated/*.go` | regenerated by `make sqlc` | AC-10 |
| `server/internal/handler/cursor.go` | new | shared cursor + limit helpers, AC-1/3/4/6 |
| `server/internal/handler/issue.go` | edit | AC-1, AC-2, AC-3 — add `ListIssuesUpdatedSinceForWorkspace`; legacy `ListIssues` untouched |
| `server/internal/handler/comment.go` | edit | AC-4, AC-5 — add `ListCommentsForBackfill`; legacy `ListComments` untouched |
| `server/internal/handler/activity.go` | edit | AC-6 — add `ListWorkspaceActivity`; legacy `ListTimeline` and `GetAssigneeFrequency` untouched |
| `server/cmd/server/router.go` | edit | AC-1, AC-4, AC-6, AC-7, AC-8 — three new `r.Get` calls inside the existing member-gated group |
| `server/internal/handler/issue_updated_since_test.go` (or extend `issue_test.go`) | new | AC-11 issue tests |
| `server/internal/handler/comment_backfill_test.go` (or extend `comment_test.go`) | new | AC-11 comment tests |
| `server/internal/handler/activity_workspace_test.go` (or extend `activity_test.go`) | new | AC-11 activity tests |
| `server/cmd/server/integration_test.go` | edit (extend) | AC-7, AC-8 integration coverage |

NOT touched in this story: `server/internal/handler/inbox.go`, `server/internal/multica/gate_client.go` (does not exist yet), any `migrations/*.sql`, `server/pkg/protocol/events.go`.

## Test strategy

- **Unit:** `server/internal/handler/cursor_test.go` covers encode/decode round-trip and the strict-after tie-break. Per-handler test files cover the AC-#11 list (PAT-less → 401, non-member → 403, malformed `limit` → 400, agent-comment inclusion, cursor advance under tied timestamps, bare `GET /api/issues` regression).
- **Integration:** `server/cmd/server/integration_test.go` mints a PAT for a workspace member, hits each new endpoint via `httptest.NewServer` over the configured router, asserts 200 + JSON shape, and asserts 401/403 for the negative paths.
- **Frontend:** none.
- **Manual:** none beyond `make check`.

## Conventions to match (Multica fork patterns)

- Handlers call `h.Queries.<Generated>` directly (sqlc) — there is no service layer in Multica's existing code. Match `issue.go` / `comment.go` / `activity.go`.
- UUID parsing via `parseUUID` (`server/internal/handler/handler.go:118`).
- RFC3339Nano on the wire; convert at the handler boundary using `time.Parse(time.RFC3339, v)` then `pgtype.Timestamptz{Time: t, Valid: true}` — same shape as `comment.go` line ~88.
- Activity action strings are `<noun>_<verb_past>` (`gate_bypassed`, `assignee_changed`, ...). Accept any string; do not enumerate.
- New routes inherit `Auth` and `RequireWorkspaceMemberFromURL` automatically by living inside the existing member-gated workspace group. Do NOT add a parallel auth path.

## Out of scope

- Do NOT modify `server/internal/handler/inbox.go` (Story 6.1).
- Do NOT create `server/internal/multica/gate_client.go` (Story 5.7).
- Do NOT add or modify any `server/migrations/*.sql` (AC-9).
- Do NOT widen the legacy `ListIssues` handler at `/api/issues`. Add a parallel handler.
- Do NOT enable `author_type='member'` filtering at the SQL or handler layer (AC-5; standalone app filters).
- Do NOT rename or remove constants in `server/pkg/protocol/events.go` — the M-PR#2 CI guard (`scripts/check_team_app_events.sh`) will reject the PR if you do.

## References (canonical sources)

- Issue description on `[TIM-4](mention://issue/653ba68f-34e5-4986-9d82-d1b79eae07fe)` — full AC text, full Subtasks, full Dev Notes.
- `_bmad-output/planning-artifacts/epics.md` lines 649–672 — canonical AC.
- `multica-team-management-spec.v3.md` §6.1 #3 / #5 / #6 (lines 985–996) — integration surface.
- `multica-team-management-spec.v3.md` §22 M-PR#3 (lines 4252–4268) — deliverables + AC.
- `multica-team-management-spec.v3.md` §14 — autofill input contract; defines the `author_type` policy.
- `_bmad-output/planning-artifacts/architecture.md` lines 247–254 (§12.1 integration table) and lines 732–746 (M-PR#3 file inventory).

---

## Dev Agent Record

(Owned by Amelia. Architect leaves blank.)

### Agent Model Used

### Debug Log References

### Completion Notes List

### File List
