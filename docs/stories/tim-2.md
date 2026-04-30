# TIM-2 — Story 1.2: Multica fork — add `estimate_minutes` to issues (M-PR#1)

Status: ready-for-dev

## Story

As a **Multica maintainer**,
I want to add an `estimate_minutes INT` column to Multica's `issue` table and expose it through the issue REST endpoints,
so that the standalone team-app can read estimates from the mirror and the gate can enforce estimate-required (V-I2/D3).

## Acceptance Criteria

1. **AC1 — Migration prefix discipline (the easiest step to get wrong).** The migration filename uses the next free prefix discovered at PR-open time via `git log --oneline -- server/migrations/` against the **PR base** (`bmad-self-host` — see Architect Note 1). Never hard-code a prefix. The current branch already cuts from `bmad-self-host`, where the highest sequential 0xx prefix is `058_drop_autopilot_priority_and_project_id` and the 1xx range starts at `101_issue_label_polish`. At PR-open time, re-check and pick the next free 0xx (currently `059`) or the next free 1xx (currently `102`); the PR description must call out the prefix used and the basis for choosing it.
2. **AC2 — `<NNN>_issue_estimate_minutes.up.sql` schema.** Wraps `BEGIN; ... COMMIT;`. Adds `estimate_minutes INT NULL` to `issue` with `CHECK (estimate_minutes IS NULL OR estimate_minutes > 0)`. Creates partial index `idx_issue_assignee_open` on `(assignee_type, assignee_id, status) WHERE status IN ('todo','in_progress','planning','ready_for_dev','fixing','testing')`. NO `UNIQUE (id, workspace_id)` constraint.
3. **AC3 — `<NNN>_issue_estimate_minutes.down.sql` is the strict inverse.** Wraps `BEGIN; ... COMMIT;`. `DROP INDEX IF EXISTS idx_issue_assignee_open;` then `ALTER TABLE issue DROP COLUMN IF EXISTS estimate_minutes;`. Up/down round-trip cleanly on a populated database.
4. **AC4 — sqlc code regenerated cleanly.** `server/pkg/db/queries/issue.sql` is updated so `estimate_minutes` is selected by every existing read query (`ListIssues`, `GetIssue`, `GetIssueInWorkspace`, `GetIssueByNumber`, `ListOpenIssues`, `ListChildIssues`) and writable through `CreateIssue`, `CreateIssueWithOrigin`, and `UpdateIssue` using **bare `sqlc.narg('estimate_minutes')`** (NOT `COALESCE` — see Architect Note 2). After edits, `make sqlc` regenerates `server/pkg/db/generated/*.go` with no manual edits.
5. **AC5 — `PUT /api/issues/{id}` accepts `estimate_minutes`.** Body field is `int | null`. When the JSON key is present and value is a positive int, set the column; when present and `null`, clear it; when the key is absent, preserve the existing value (use the same `rawFields` "explicit-null vs absent" detection pattern that `assignee_id`, `due_date`, and `parent_issue_id` use in `handler/issue.go:UpdateIssue`). Values `<= 0` (or non-integer) return HTTP 400 with a clear message — the DB CHECK is the last line of defence; the handler validates before sqlc invocation.
6. **AC6 — `GET /api/issues/{id}` response includes both fields.** Response includes `estimate_minutes` (nullable int — pointer in JSON, so `null` when unset) and `computed_estimate_minutes` (int, parent rollup; `0` when the issue has no descendants). The rollup is `Σ leaf descendants WHERE assignee_type IS NULL OR assignee_type = 'member'` per D22/B4 — agent-assigned leaves are excluded; the parent's own `estimate_minutes` is ignored when the issue has children (D37). Compute via a recursive CTE against `issue` (Multica's own table — NOT `mirror_issue`).
7. **AC7 — `ListIssues` and other read paths surface `estimate_minutes`.** `IssueResponse`, `issueToResponse`, `issueListRowToResponse`, and `openIssueRowToResponse` in `handler/issue.go` all expose the field. Existing list/open/child endpoints round-trip the value.
8. **AC8 — Zero behavioural changes outside the new field.** Issue CRUD, search, status updates, priority updates, assignment, parent-child cycling, BatchUpdateIssues, daemon GC checks, and label enrichment all behave identically to the PR base. No regression in `make check` (`go test ./...`, `pnpm test`, `pnpm typecheck`, `pnpm exec playwright test`).
9. **AC9 — Test coverage.** Adds Go handler tests asserting (a) PUT with `estimate_minutes: 60` persists and round-trips on GET, (b) PUT with `estimate_minutes: null` clears the field, (c) PUT with `estimate_minutes: 0` returns 400, (d) PUT with `estimate_minutes: -10` returns 400, (e) GET returns `computed_estimate_minutes = sum of member/null-assignee leaf descendants` for a parent with two member-assigned leaves and one agent-assigned leaf (the agent leaf is excluded), (f) GET returns `computed_estimate_minutes = 0` for a leaf with no children. Place tests in `server/internal/handler/handler_test.go` next to `TestIssueCRUD`, following the same `httptest`/`testHandler` pattern.
10. **AC10 — `idx_issue_assignee_open` is correctly partial.** A test or `EXPLAIN ANALYZE` verification confirms the index fires only for queries restricted to the open-bucket statuses.

## Implementation Plan (Architect-owned)

### Affected repos

| Repo | Branch | PR base |
|---|---|---|
| `zeyad-farrag/multica` | `multica/tim-2` | `bmad-self-host` |

### Files to touch

| Path | Change | Why |
|---|---|---|
| `server/migrations/<NNN>_issue_estimate_minutes.up.sql` | new | AC2 — schema additions |
| `server/migrations/<NNN>_issue_estimate_minutes.down.sql` | new | AC3 — strict inverse |
| `server/pkg/db/queries/issue.sql` | edit | AC4 — surface column in reads, accept it in writes, add `ComputeIssueEstimateRollup` |
| `server/pkg/db/generated/issue.sql.go` | regenerated | AC4 — `make sqlc` regenerates from `issue.sql` |
| `server/pkg/db/generated/models.go` | regenerated | AC4 — `Issue` struct gets `EstimateMinutes pgtype.Int4` |
| `server/internal/handler/issue.go` | edit | AC5 / AC6 / AC7 — request/response shape, `rawFields` PATCH semantics, rollup wiring |
| `server/internal/handler/handler_test.go` | edit | AC9 — six new test cases next to `TestIssueCRUD` |

### Approach (ordered)

1. **Validate prefix at PR-open time, not now.** When opening the PR, run `git fetch origin bmad-self-host` then `git log --oneline -- server/migrations/ | head` and pick the first free 0xx (currently `059`) or first free 1xx (currently `102`). Document the basis in the PR description (AC1).
2. Write `server/migrations/<NNN>_issue_estimate_minutes.up.sql`: `BEGIN; ALTER TABLE issue ADD COLUMN estimate_minutes INT NULL CHECK (estimate_minutes IS NULL OR estimate_minutes > 0); CREATE INDEX idx_issue_assignee_open ON issue (assignee_type, assignee_id, status) WHERE status IN ('todo','in_progress','planning','ready_for_dev','fixing','testing'); COMMIT;` (AC2).
3. Write the matching `<NNN>_issue_estimate_minutes.down.sql`: `BEGIN; DROP INDEX IF EXISTS idx_issue_assignee_open; ALTER TABLE issue DROP COLUMN IF EXISTS estimate_minutes; COMMIT;` (AC3). Verify round-trip with `make migrate-up && make migrate-down && make migrate-up`.
4. Edit `server/pkg/db/queries/issue.sql` (AC4):
   - `ListIssues` and `ListOpenIssues` use explicit column lists — append `estimate_minutes` to both. `GetIssue`, `GetIssueInWorkspace`, `GetIssueByNumber`, `ListChildIssues`, and the `RETURNING *` in `CreateIssue` / `CreateIssueWithOrigin` / `UpdateIssue` already pick up the new column from the schema, so no edit needed there beyond verifying the regen output.
   - In `CreateIssue` and `CreateIssueWithOrigin`, append `estimate_minutes` to the `INSERT (...)` column list and `sqlc.narg('estimate_minutes')` to `VALUES (...)`.
   - In `UpdateIssue`, add a new line `estimate_minutes = sqlc.narg('estimate_minutes'),` — **bare narg, no COALESCE** (Architect Note 2). Place it next to `assignee_id`, `due_date`, `parent_issue_id`, `project_id`.
   - Add a new query at the bottom of `issue.sql`:
     ```
     -- name: ComputeIssueEstimateRollup :one
     WITH RECURSIVE descendants AS (
       SELECT id, parent_issue_id, estimate_minutes, assignee_type
         FROM issue
        WHERE parent_issue_id = sqlc.arg('issue_id')
          AND workspace_id = sqlc.arg('workspace_id')
       UNION ALL
       SELECT i.id, i.parent_issue_id, i.estimate_minutes, i.assignee_type
         FROM issue i
         JOIN descendants d ON i.parent_issue_id = d.id
        WHERE i.workspace_id = sqlc.arg('workspace_id')
     ),
     leaves AS (
       SELECT d.id, d.estimate_minutes, d.assignee_type
         FROM descendants d
        WHERE NOT EXISTS (
          SELECT 1 FROM descendants c WHERE c.parent_issue_id = d.id
        )
     )
     SELECT COALESCE(SUM(estimate_minutes), 0)::int AS total
       FROM leaves
      WHERE estimate_minutes IS NOT NULL
        AND (assignee_type IS NULL OR assignee_type = 'member');
     ```
   - Run `make sqlc` from project root. Verify `Issue.EstimateMinutes pgtype.Int4` lands in `models.go` and the new `ComputeIssueEstimateRollup` Go method lands in `issue.sql.go`.
5. Edit `server/internal/handler/issue.go` (AC5 / AC6 / AC7):
   - Add `EstimateMinutes *int32 \`json:"estimate_minutes"\`` and `ComputedEstimateMinutes int32 \`json:"computed_estimate_minutes"\`` (no pointer — always present, default `0`) to `IssueResponse`.
   - Add `int4ToPtr(v pgtype.Int4) *int32` helper next to the existing `textToPtr`/`uuidToPtr`/`timestampToPtr` (search the handler package — `agent.go:90` shows the existing helper style; place the new one wherever the existing pointer helpers live).
   - Populate `EstimateMinutes` in `issueToResponse`, `issueListRowToResponse`, and `openIssueRowToResponse` from `i.EstimateMinutes`.
   - In `GetIssue`, after building `resp` via `issueToResponse`, call `h.Queries.ComputeIssueEstimateRollup(ctx, ComputeIssueEstimateRollupParams{IssueID: ..., WorkspaceID: ...})` and assign the result to `resp.ComputedEstimateMinutes`. On query error, `slog.Warn` with the issue ID and default to `0` — do NOT fail the GET because of a rollup computation issue. (`ListIssues` and the open/child endpoints do NOT call the rollup — too expensive on a list path.)
   - Extend `UpdateIssueRequest` with `EstimateMinutes *int32 \`json:"estimate_minutes"\``.
   - In `UpdateIssue`, after the existing `rawFields` decode, add:
     ```
     params.EstimateMinutes = prevIssue.EstimateMinutes // preserve when key absent
     if _, ok := rawFields["estimate_minutes"]; ok {
         if req.EstimateMinutes == nil {
             params.EstimateMinutes = pgtype.Int4{Valid: false}
         } else if *req.EstimateMinutes <= 0 {
             writeError(w, http.StatusBadRequest, "estimate_minutes must be > 0")
             return
         } else {
             params.EstimateMinutes = pgtype.Int4{Int32: *req.EstimateMinutes, Valid: true}
         }
     }
     ```
     Place it next to the `phase_state` block (around line 1283). Read the existing `assignee_id` / `due_date` branches end-to-end first to lock in the exact shape — they're the canonical precedent (Architect Note 2).
   - For `CreateIssue` and `CreateIssueWithOrigin` handlers, leave `estimate_minutes` out of the request body and pass `pgtype.Int4{Valid: false}` to sqlc — estimates are set post-creation via `UpdateIssue`. (AC5 only requires PUT acceptance; if `git grep CreateIssueRequest` shows a clean place to add it, do so additively, but the v1 path does not require it.)
6. Add tests in `server/internal/handler/handler_test.go` next to `TestIssueCRUD` (AC9). Use the existing `testHandler` / `newRequest` / `withURLParam` scaffold. Six cases:
   - (a) PUT `{estimate_minutes: 60}` then GET → `estimate_minutes == 60`.
   - (b) PUT `{estimate_minutes: null}` after (a) then GET → `estimate_minutes == nil`.
   - (c) PUT `{estimate_minutes: 0}` → 400.
   - (d) PUT `{estimate_minutes: -10}` → 400.
   - (e) Create parent, two member-assigned children with estimates 30/45, one agent-assigned child with estimate 999. GET parent → `computed_estimate_minutes == 75`.
   - (f) Create leaf with no children, set `estimate_minutes: 60`. GET → `computed_estimate_minutes == 0`.
7. Run `make check` at the repo root. All green before opening the PR (AC8).
8. Verify partial-index behaviour (AC10): `EXPLAIN ANALYZE SELECT id FROM issue WHERE assignee_type = 'member' AND assignee_id = '<uuid>' AND status = 'in_progress';` should pick `idx_issue_assignee_open`. Same query with `status = 'done'` should NOT use it.

### Test strategy

- **Unit (Go):** six new cases in `server/internal/handler/handler_test.go` driving `TestUpdateIssueEstimateMinutes_RoundTrip` and `TestGetIssueComputedEstimateMinutes` through `httptest` (AC9 a–f).
- **Integration:** none specific to this story. `make migrate-up && make migrate-down && make migrate-up` against a populated dev DB confirms the schema round-trip (AC2 / AC3).
- **Manual verification:** `EXPLAIN ANALYZE` on dev DB to confirm `idx_issue_assignee_open` is partial (AC10). Capture both EXPLAIN outputs in the PR description.
- **Full pipeline:** `make check` from repo root must be green (AC8).

### Assumptions (Architect Notes)

1. **PR base is `bmad-self-host`, not `main`.** The canonical repo→base mapping in this BMAD workspace targets `bmad-self-host` (the fork's working trunk). The story description's AC1 references `origin/main`, but that's the upstream-Multica check — for THIS PR, the migrations need to be unique on the actual merge target. On `bmad-self-host` today: `057_feedback`, `058_drop_autopilot_priority_and_project_id`, `101_issue_label_polish`, `999_bmad_phase_state`, `1000-1003_bmad_*` are all present. The next free 0xx is `059`; the next free 1xx is `102`. Pick whichever range matches the prevailing convention at PR-open time and document the basis. **Do not hard-code `057` — it is already taken on `bmad-self-host`.**

2. **`UpdateIssue` uses bare `sqlc.narg`, NOT `COALESCE`.** AC4 says "mirror the existing `phase_state` precedent" but the actual `phase_state` line in `issue.sql:49` uses `COALESCE(sqlc.narg('phase_state')::jsonb, phase_state)` — that pattern cannot represent "explicit clear to NULL". The correct precedent for `estimate_minutes` is `assignee_id` / `due_date` / `parent_issue_id` / `project_id` (lines 44, 46, 47, 48), which all use bare `sqlc.narg(...)` and pair with handler-side preserve-on-absent. AC5 / AC6 / Anti-patterns make this clear; the in-text "phase_state precedent" reference in AC4 is a citation slip — follow the bare-narg pattern.

3. **`computed_estimate_minutes` is computed only on `GetIssue`, not on list paths.** The recursive CTE is O(descendants) per call; running it on every row of `ListIssues` would be a perf regression (AC8). The standalone team-app's gate (M-PR#3 territory) reads single-issue context, so single-issue computation is sufficient.

4. **`CreateIssue` does not currently accept `estimate_minutes`.** v1 product flow sets estimates post-creation via `UpdateIssue`. If `git grep CreateIssueRequest` shows a tidy way to add the field additively without changing existing behaviour, do so — but the AC list does not require it, and adding it expands surface area beyond what M-PR#1 commits to.

5. **Frontend may need a no-op type bump.** The Next.js type definitions for `Issue` may surface a `tsc` warning when the Go handler grows two new response fields. Run `pnpm typecheck` and only update the TS types if it complains; v1 UI does not surface estimates (AC8 — zero behavioural changes).

### Out of scope

- The `team-app/` repo, `mirror_*` tables, capacity service, gate handler — those belong to Epic 1's other stories (M-PR#2, M-PR#3, and beyond). This story stays inside the existing `multica` repo.
- WS event emissions on `estimate_minutes` change — AC8 forbids behavioural drift outside the new field.
- `BatchUpdateIssues`, daemon handlers, search, autopilot — left untouched (AC8).
- A separate `058_team_app_ws_events.up.sql` migration — explicitly retired in spec §6.1 line 983.
- A `UNIQUE (id, workspace_id)` constraint on `issue` — explicitly retired in spec §6.1 NOTE.
- Modifying the existing UI to surface `estimate_minutes` — Multica's v1 UI ignores the field; the team-app reads it through M-PR#3.

## Tasks / Subtasks

- [x] **Task 1 — Determine migration prefix at PR-open time (AC: #1).** Re-fetch `bmad-self-host`, run `git log --oneline -- server/migrations/`, pick the next free prefix in the 0xx or 1xx range, document the basis in the PR description. Do this immediately before `gh pr create`, NOT during implementation.
- [x] **Task 2 — Write the migration files (AC: #2, #3).** Create `<NNN>_issue_estimate_minutes.up.sql` with the column + CHECK + partial index in one transaction. Create the strict-inverse `<NNN>_issue_estimate_minutes.down.sql`. Verify `make migrate-up && make migrate-down && make migrate-up` round-trips cleanly.
- [x] **Task 3 — Update `pkg/db/queries/issue.sql` (AC: #4).** Append `estimate_minutes` to the explicit column lists in `ListIssues` / `ListOpenIssues`. Append `estimate_minutes` to `INSERT (...) VALUES (...)` in `CreateIssue` / `CreateIssueWithOrigin` (`sqlc.narg('estimate_minutes')`). Add `estimate_minutes = sqlc.narg('estimate_minutes')` to `UpdateIssue` (bare narg, NOT COALESCE — Architect Note 2). Add new query `ComputeIssueEstimateRollup` (recursive CTE per Approach step 4). Run `make sqlc`; verify `EstimateMinutes pgtype.Int4` lands on `Issue` and a `ComputeIssueEstimateRollup` method lands on `Queries`.
- [x] **Task 4 — Update `internal/handler/issue.go` (AC: #5, #6, #7).** Extend `IssueResponse` with `EstimateMinutes *int32` and `ComputedEstimateMinutes int32`. Add `int4ToPtr` helper if not present. Populate the new fields in `issueToResponse`, `issueListRowToResponse`, `openIssueRowToResponse`. Wire the rollup call into `GetIssue` (with `slog.Warn` + default 0 on error). Extend `UpdateIssueRequest` with `EstimateMinutes *int32`. In `UpdateIssue`, follow the existing `rawFields` "explicit-null vs absent" pattern to set / clear / preserve, with `<= 0` returning 400 before sqlc invocation.
- [x] **Task 5 — Add Go handler tests (AC: #9).** Add the six cases (a–f) next to `TestIssueCRUD` using the existing `testHandler` scaffold.
- [x] **Task 6 — Verify (AC: #8, #10).** Run `make check` green from repo root. Run the two `EXPLAIN ANALYZE` queries to confirm the partial index is selected for open-bucket statuses and not for `status = 'done'`. Capture EXPLAIN outputs for the PR description.
- [x] **Task 7 — PR description (when Marcus opens it).** Title: `feat(issue): add estimate_minutes column for team-app gate (M-PR#1)`. Body: prefix used + basis (`git log` snapshot), schema diff, REST contract diff, "no behavioural changes outside this field" statement, link to spec §6.1 / §22 M-PR#1 / §19.4.

## Dev Notes

### What this story is and is not

**Is:** the first of three Multica-fork PRs (M-PR#1 → M-PR#3) that lay the integration surface for the standalone team-app. M-PR#1 is the only fork PR that touches the Multica schema. M-PR#2 is verification-only, M-PR#3 adds the gate hook + read endpoints.

**Is not:** any standalone-app work. The `team-app/` repo, `mirror_*` tables, capacity service, and the gate handler all live in Epic 1's other stories and beyond. This story stays inside the existing Multica repo.

### Codebase patterns to follow (NOT reinvent)

- **Nullable-field PATCH semantics with explicit-null detection.** `handler/issue.go:UpdateIssue` already implements this for `assignee_type`, `assignee_id`, `due_date`, `parent_issue_id`, `project_id`, and `phase_state` using `bodyBytes` + `json.Unmarshal(bodyBytes, &rawFields)` (lines 1177–1297). Read those branches end-to-end before writing the `estimate_minutes` branch — they show exactly how "key absent vs key=null vs key=value" is distinguished and how the sqlc bare-`narg` pattern is paired with handler-side pre-fill from `prevIssue`.
- **`pgtype.Int4` for nullable INT.** Multica's `Issue` model uses `pgtype.Text`, `pgtype.UUID`, `pgtype.Timestamptz`. The equivalent for nullable INT is `pgtype.Int4`. Search existing usages: `grep -rn "pgtype.Int4" server/`.
- **JSON pointer for nullable int.** `IssueResponse.AssigneeID *string`, `DueDate *string`. Follow this pattern with `EstimateMinutes *int32`.
- **Recursive CTE for parent rollup.** Spec §8.6 line 1947 shows the canonical shape (against `mirror_issue` in the standalone app); this story transcribes it onto Multica's `issue` table. Key invariants: exclude the root from the SUM, exclude descendants that have children of their own (only leaves contribute), exclude descendants where `assignee_type = 'agent'`.
- **Test pattern.** `handler/handler_test.go:TestIssueCRUD` (line 237) is the canonical handler-test shape. Use `httptest.NewRecorder()`, `newRequest("PUT", path, body)`, `withURLParam`, `testHandler.UpdateIssue(w, req)`. Decode response into `IssueResponse` and assert.

### Behavioural guardrails

- The new column is **nullable**. Existing rows have `estimate_minutes = NULL` after the up-migration. No backfill required.
- Multica's existing UI does not surface estimates in v1. The team-app reads them via M-PR#3 (Story 1.4); for now the field is write-only from Multica's perspective.
- The CHECK constraint `estimate_minutes IS NULL OR estimate_minutes > 0` is intentionally permissive of `NULL` and strict on positive values. There is no upper bound.
- The partial index `idx_issue_assignee_open` exists exclusively to support the standalone app's commitment queries (FR42). It does not need to fire for any current Multica query path; the EXPLAIN check in AC10 confirms current Multica reads still pick their existing indexes.

### Anti-patterns to avoid

- **Do not** hard-code `057` (or any fixed prefix). On `bmad-self-host` `057` is already taken (`057_feedback`). See Architect Note 1.
- **Do not** add a `UNIQUE (id, workspace_id)` constraint on `issue`.
- **Do not** use `COALESCE(sqlc.narg('estimate_minutes'), estimate_minutes)` in `UpdateIssue`. See Architect Note 2.
- **Do not** modify Multica's WS event emissions or any handler outside `issue.go`. Behavioural surface area must be exactly the new field.
- **Do not** run the rollup CTE on `ListIssues` or `ListOpenIssues` — single-issue computation only (Architect Note 3).
- **Do not** split the column add and the index create into two migrations — both go in one file inside one transaction.
- **Do not** add a `058_team_app_ws_events.up.sql` placeholder.

### Project Structure Notes

- Migrations live at `server/migrations/<NNN>_<purpose>.up.sql` / `.down.sql` (bare prefix, not zero-padded above 100; existing repo has `056_…` and `101_…` side-by-side).
- sqlc config is at `server/sqlc.yaml`: queries dir is `pkg/db/queries/`, schema dir is `migrations/`, generated output goes to `pkg/db/generated/` (do not hand-edit). Run `make sqlc` from project root.
- Handler files stay flat under `server/internal/handler/`. Tests live in the same directory (`*_test.go`).

### References

- Spec §6.1 (lines 929–972) — canonical migration SQL and REST contract.
- Spec §22 M-PR#1 (lines 4225–4235) — pre-checks, deliverables, AC list (authoritative).
- Spec §19.4 (lines 3872–3877) — `computed_estimate_minutes` invariants (D22, D37, B4).
- Spec §8.6 ComputeIssueEstimateRollup (lines 1947–1968) — canonical recursive-CTE shape (translated from `mirror_issue` to `issue` for this story).
- Spec §0.3 (lines 97–117) — open-bucket statuses for the partial index.
- `server/internal/handler/issue.go:1177-1297` — existing nullable-field PATCH pattern to follow.
- `server/pkg/db/queries/issue.sql:39-49` — `UpdateIssue` precedent (bare-narg fields are 43, 44, 46, 47, 48; `phase_state` line 49 uses COALESCE — see Architect Note 2).
- `server/internal/handler/handler_test.go:237` `TestIssueCRUD` — handler-test scaffold pattern.

## Dev Agent Record

### Agent Model Used

GPT-5 Codex

### Debug Log References

- `git fetch origin bmad-self-host` confirmed the PR-base migration history; local migrations already include `057_feedback`, `058_drop_autopilot_priority_and_project_id`, and `101_issue_label_polish`, so this implementation uses prefix `059`.
- `make sqlc` regenerated `server/pkg/db/generated/issue.sql.go` and `server/pkg/db/generated/models.go`.
- `make migrate-up`, `make migrate-down`, DB-backed handler test execution, `make check`, and EXPLAIN verification were blocked by local Docker/Postgres access: Docker socket permission denied for `pgvector/pgvector:pg17`, and direct DB connection failed password auth for user `multica`.
- `GOCACHE=/tmp/go-build go test ./...` compiled changed packages but failed in pre-existing `server/internal/daemon/execenv` tests: `TestReuseWritesMissingCodexWorkspaceSkills` and `TestReuseUpdatesCodexWorkspaceSkills`.
- `COREPACK_HOME=/tmp/corepack XDG_CACHE_HOME=/tmp/xdg-cache XDG_DATA_HOME=/tmp/xdg-data XDG_STATE_HOME=/tmp/xdg-state pnpm typecheck` passed.
- `COREPACK_HOME=/tmp/corepack XDG_CACHE_HOME=/tmp/xdg-cache XDG_DATA_HOME=/tmp/xdg-data XDG_STATE_HOME=/tmp/xdg-state pnpm test` passed.

### Completion Notes List

- Ultimate context engine analysis completed — comprehensive developer guide created.
- Added nullable `issue.estimate_minutes` with positive-value CHECK and `idx_issue_assignee_open` partial index in migration `059`.
- Surfaced `estimate_minutes` through sqlc reads/writes and added `ComputeIssueEstimateRollup`.
- Added PUT set/clear/preserve semantics and GET `computed_estimate_minutes` rollup response.
- Added handler tests covering estimate set, clear, invalid values, parent rollup, and leaf rollup.
- PR description handoff: title `feat(issue): add estimate_minutes column for team-app gate (M-PR#1)`; prefix basis is `bmad-self-host` next free 0xx prefix `059` after `058_drop_autopilot_priority_and_project_id`.

### File List

- `docs/stories/tim-2.md`
- `server/migrations/059_issue_estimate_minutes.up.sql`
- `server/migrations/059_issue_estimate_minutes.down.sql`
- `server/pkg/db/queries/issue.sql`
- `server/pkg/db/generated/issue.sql.go`
- `server/pkg/db/generated/models.go`
- `server/internal/handler/issue.go`
- `server/internal/handler/handler_test.go`
