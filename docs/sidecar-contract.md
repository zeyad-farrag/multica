# Multica ↔ BMAD Sidecar Contract

This document is the source of truth for what the BMAD sidecar
(`/opt/multica-bmad/sidecar/`) reads from and writes to the Multica
backend. The sidecar is a separate Python process; Multica only stores
the `issue.phase_state` JSONB column it consumes.

The contract is in two parts:
1. **`issue.phase_state` shape** — the sidecar's persistent counters and
   per-issue memo, written via `PATCH /api/issues/{id}` with a
   `phase_state` field.
2. **Comment-marker contract** — the comment-content markers that drive
   sidecar routing. These are parsed by `bmad_sidecar/contract_parser.py`
   and the comment.type column is intentionally `comment` (the markers
   live in the Markdown body, not the type).

If you change either part, update this doc and the sidecar parser
together. Drift between Multica writes and sidecar reads will silently
mis-route issues.

## `issue.phase_state` shape

The column is `jsonb NULL`. Multica makes no assumptions about the inner
fields — they're entirely owned by the sidecar. This document describes
what the sidecar writes today (2026-05) so other services know how to
read consistent state.

```jsonc
{
  // ---- canonical counters (reset on terminal status per shared §5) -----
  "planning_loop": 0,        // bumps on plan-issue / sidecar bounce_to_planning
  "decision_loop": 0,        // bumps inside in_progress per-cycle (decision-needed)
  "review_loop": 0,          // bumps on code_review → fixing (Quinn requests changes)
  "test_loop": 0,            // bumps on testing → fixing (Murat RED)
  "cr_round": 0,             // bumps on coderabbit → resolving (CR review round)
                             // (replaces the legacy `pr_loop`, retired 2026-05)

  // ---- per-column retry counter (separate lifecycle: clears on column exit) ----
  "stall_retry": 0,          // bumps on each stall_recovery for the current column

  // ---- routing memo (read by markers downstream) -----------------------
  "previous_loop": "review", // "review" | "test" | null
                             //   set by Quinn's review-findings approval (test)
                             //   set by Quinn's review-findings changes_requested (review)
                             // Read by Felix's fix-note router. The legacy
                             // "cr" value (set by Rosa's resolution-note,
                             // read by Murat) is retired in the 2026-05-03
                             // CR-loop redesign — Murat is no longer in the
                             // post-CR loop.

  // ---- blocked-origin memo (set on terminal flip to `blocked`) --------
  "blocked_origin": "code_review",   // status active when sidecar flipped to blocked
                                     // Cleared on unblock (contract 10 §4).

  // ---- audit metadata (purely informational) ---------------------------
  "last_marker": "review-findings (changes_requested) → fixing[review]",
  "updated_at": "2026-05-02T10:00:00Z",
  "from_comment_id": "9b8c7d6e-…",

  // ---- legacy phase-progress UI fields (preserved verbatim, never authoritative) ----
  "phase": "build",
  "status": "in_progress",
  "branches": ["multica/MUL-117"],
  "prs": ["zeyad-farrag/multica#19"],
  "last_pr_url": "https://github.com/zeyad-farrag/multica/pull/19"
}
```

### Counter reset rules

All counters in `LOOP_COUNTER_KEYS = ("planning_loop", "decision_loop",
"review_loop", "test_loop", "cr_round")` reset to zero when the issue
reaches a terminal status (`done`, `staged`, `cancelled`, `blocked`). The
sidecar performs the reset by writing a `phase_state` patch with those
keys removed. `stall_retry` resets when status leaves a column (separate
lifecycle).

### CR-resolution loop (2026-05-03 redesign)

The CR loop is a tight three-column cycle. Quinn (`code_review`) and Murat
(`testing`) are intentionally not in the post-CR loop — CR re-reviewing the
push IS the quality gate, so re-running them every iteration is wasted
work.

1. Sidecar bumps `cr_round` and writes it into `phase_state` on every
   `coderabbit → resolving` transition. `cr_round` is the persistent
   indicator that "the issue has been through resolving at least once."
2. Rosa's `<!-- resolution-note -->` marker routes `resolving → in_review`.
   `previous_loop` is intentionally NOT set — nothing downstream needs the
   cr-lineage signal in the new flow.
3. Marcus runs `bmad-pr-resolve` on `in_review`: pushes Rosa's accumulated
   patches, posts each thread's verbatim `fixer_reply` to GitHub, calls
   `/resolve` on each thread, then emits `<!-- pr-republished -->` to exit.
   The sidecar routes `in_review → coderabbit`.
4. CR re-reviews on the push. The state machine handles the next move:
   - APPROVED → `coderabbit → staged` (the SOLE path to staged).
   - COMMENTED with new findings → `coderabbit → resolving` (loop, `cr_round` bumps again).
   - CHANGES_REQUESTED → `coderabbit → resolving` (loop).
5. `staged → done` fires on `pull_request.closed merged=true`. Both
   terminal flips reset all loop counters.

Rationale: predicate-clear → staged was removed in Phase 1 (state machine,
2026-05-03) so threads draining to zero (Marcus's own `/resolve` calls)
cannot promote the issue. Only an explicit CR APPROVED review can.

## Comment-marker contract

Sidecar markers live in `comment.content` as HTML comments
(`<!-- marker-name -->`) on rows of `comment.type = "comment"`. The
parser ignores all other types. The full vocabulary lives in
`bmad_sidecar/contract_parser.py::_MARKER_RE`. Selected entries:

| Marker | Author column | Routes to | Notes |
|---|---|---|---|
| `<!-- claim -->` | ready_for_dev | (no route; claim only) | Required body: `role: dev`, `issue: <id>`. |
| `<!-- impl-plan -->` | planning | ready_for_dev | Resets all loop counters. |
| `<!-- plan-issue -->` | ready_for_dev / in_progress | planning | Bumps `planning_loop`. |
| `<!-- decision-needed -->` | in_progress | planning | No loop bump (targeted touch-up). |
| `<!-- decision-resolved -->` | planning | ready_for_dev | No loop bump. |
| `<!-- arch-blocked -->` | planning | blocked | Suppresses default sidecar audit. |
| `<!-- review-findings -->` | code_review | testing (approved) / fixing (changes_requested) | Sets `previous_loop=test` or `=review`; bumps `review_loop` on changes_requested. |
| `<!-- fix-note -->` | fixing | code_review (review) / testing (test) | Routes by `phase_state.previous_loop`. The legacy `pr` lane was retired 2026-05. |
| `<!-- resolution-note -->` | resolving | in_review | Rosa's exit. `previous_loop` is NOT set (cr-lineage signal unused post-redesign). (UPDATED 2026-05-03.) |
| `<!-- completion-note -->` (role: dev) | in_progress | code_review (GREEN) / blocked (RED) | Sets `previous_loop=review` on GREEN. |
| `<!-- completion-note -->` (role: tea) | testing | coderabbit (GREEN) / fixing (RED) | The pre-CR `testing → coderabbit` path. The CR loop no longer re-enters testing, so the legacy `cr_round > 0 → in_review` branch is unreachable in the new flow. |
| `<!-- pr-opened -->` | coderabbit / in_review | (non-routing) | Records `phase_state.last_pr_url`. Marcus emits this on the initial publish on coderabbit. |
| `<!-- pr-republished -->` | in_review | coderabbit | Marcus's exit after push + reply + resolve. (NEW 2026-05-03.) |
| `<!-- post-merge-noop -->` | ready_for_dev / in_progress | staged | Step 1.1.0 short-circuit when PR has already been merged externally. |

### Sidecar audit comments

The sidecar writes its own audit trail under the reserved `<!-- sidecar-* -->`
namespace (e.g. `<!-- sidecar-loop-bump -->`, `<!-- sidecar-bridge -->`,
`<!-- sidecar-block -->`). The parser intentionally REJECTS these as
routing decisions — they are records, not triggers. Multica writes none
of these; only the sidecar does.

## Status enum (2026-05-03 redesign)

The CR-resolution loop is a tight three-column cycle; the pre-CR pipeline
is unchanged from the original BMAD spec:

```
backlog → todo → planning → ready_for_dev → in_progress
       ↓
     code_review ⇄ fixing (review) ⇄ testing
                   ↑                  ↑
                 fixing (test) ⟵──────┘
                                      ↓
                                   coderabbit  ◄────────────────┐
                                      ↓ (CR found issues)       │
                                   resolving   (Rosa fixes; cr_round bumps)
                                      ↓ (resolution-note)        │
                                   in_review   (Marcus pushes + posts replies + resolves)
                                      ↓ (pr-republished)         │
                                      └──────────────────────────┘
                                      ↓
                                   coderabbit  → staged   (CR APPROVED — sole path)
                                                  ↓
                                                done   (PR merged)
```

`fixing` is the inner-loop column for review/test verdict rework only;
the CR-resolution loop is a separate column (`resolving`). Quinn
(`code_review`) and Murat (`testing`) are not in the CR loop — CR
re-reviews on every push and is the quality gate.

## Contract change protocol

If you need to change the `phase_state` shape OR the comment-marker
vocabulary:

1. Update this doc first.
2. Update the sidecar parser/writer in lockstep.
3. Update the corresponding contract file under
   `/opt/multica-bmad/contracts/` (the source-of-truth narrative spec).
4. If the change is backwards-incompatible (renaming a counter, dropping
   a marker), bump a new sidecar contract version (e.g. `v6`) and add a
   migration note in `column-routing.yaml`.

## Authority

- Multica owns: status enum, comment.type CHECK, comment-API write
  guards, the GitHub webhook → state-machine glue, the
  in_review/coderabbit predicate (`noUnresolvedCRThreads`), the
  cr-settle sweeper.
- Sidecar owns: `phase_state` schema, comment-marker vocabulary,
  predecessor validation, loop caps, watchdog/dispatch verification,
  blocked-origin tracking, mention-router unblock.
- Neither side may write to the other's domain without going through
  the published API surface.
