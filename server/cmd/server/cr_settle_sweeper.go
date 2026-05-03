package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/events"
	githubintegration "github.com/multica-ai/multica/server/internal/integrations/github"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// CR settle-window sweeper.
//
// Background. The state machine's `coderabbit → resolving` transition is
// driven by CodeRabbit's `pull_request_review.submitted` webhook event,
// which CR fires once at the end of a review batch. That works in the
// happy path, but a long PR or a misconfigured CR can stream individual
// `pull_request_review_comment.created` events without ever wrapping them
// in a `review.submitted` — the issue then sits in `coderabbit` forever
// because no transition fires.
//
// This sweeper is the safety net. Every `crSweepInterval`, it queries for
// issues that have been in `coderabbit` for at least `crSettleSeconds`
// past their last unresolved CR comment, and forces the
// `coderabbit → resolving` transition the same way the webhook would have.
// Idempotent: the second pass only catches issues whose unresolved count
// hasn't been zero'd out yet.
//
// Tunable via env:
//   MULTICA_CR_SWEEP_INTERVAL_SECS  default 60
//   MULTICA_CR_SETTLE_SECS          default 300 (5 minutes)
//
// Set CR_SETTLE_SECS to a small value (e.g. 30) for development.

var (
	crSweepInterval  = envSeconds("MULTICA_CR_SWEEP_INTERVAL_SECS", 60)
	crSettleSeconds  = envSeconds("MULTICA_CR_SETTLE_SECS", 300)
)

func envSeconds(name string, fallback int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

// runCRSettleSweeper periodically scans for issues parked in `coderabbit`
// whose CR comment stream has gone quiet, and forces them through to
// `resolving` — closing the gap when CodeRabbit doesn't emit a wrapping
// `pull_request_review.submitted` event.
func runCRSettleSweeper(ctx context.Context, queries *db.Queries, bus *events.Bus) {
	if crSweepInterval <= 0 {
		slog.Info("cr-settle sweeper: disabled (MULTICA_CR_SWEEP_INTERVAL_SECS=0)")
		return
	}
	slog.Info("cr-settle sweeper: starting",
		"interval_secs", crSweepInterval,
		"settle_secs", crSettleSeconds,
	)
	ticker := time.NewTicker(time.Duration(crSweepInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweepStuckCoderabbitIssues(ctx, queries, bus)
		}
	}
}

func sweepStuckCoderabbitIssues(ctx context.Context, queries *db.Queries, bus *events.Bus) {
	rows, err := queries.ListStuckCoderabbitIssues(ctx, int32(crSettleSeconds))
	if err != nil {
		slog.Warn("cr-settle sweeper: query failed", "error", err)
		return
	}
	if len(rows) == 0 {
		return
	}
	slog.Info("cr-settle sweeper: forcing coderabbit → resolving",
		"count", len(rows),
		"settle_secs", crSettleSeconds,
	)
	for _, row := range rows {
		forceCoderabbitToResolving(ctx, queries, bus, row)
	}
}

// forceCoderabbitToResolving applies the coderabbit → resolving transition
// directly via UpdateIssueStatus instead of going through the
// github.Decide() state-machine function.
//
// This is intentional. The sweeper's pre-conditions (queried via
// ListStuckCoderabbitIssues) already enforce the only invariant Decide
// would check: the issue is in `coderabbit` AND has at least one
// unresolved CR thread AND the comment stream has settled. Running the
// rows through Decide would either be a no-op (if Decide had a matching
// branch) or wouldn't fire (because Decide is keyed off webhook event
// kinds — EventKindReview / EventKindReviewThread — and we have neither
// here, just an SQL-driven cron tick).
//
// **Maintenance note:** if the coderabbit → resolving transition logic
// in state_machine.go's decideReview / decideReviewThread changes
// (e.g. adds a new precondition or an additional side-effect), update
// this function in lockstep. The two paths are not mechanically linked.
func forceCoderabbitToResolving(ctx context.Context, queries *db.Queries, bus *events.Bus, row db.ListStuckCoderabbitIssuesRow) {
	const newStatus = githubintegration.StatusResolving
	updated, err := queries.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{
		ID:     row.IssueID,
		Status: newStatus,
	})
	if err != nil {
		slog.Warn("cr-settle sweeper: status update failed",
			"issue", util.UUIDToString(row.IssueID),
			"error", err,
		)
		return
	}

	// Activity row matching the webhook's "soft changes requested" path.
	details, _ := json.Marshal(map[string]any{
		"from":             "coderabbit",
		"to":               newStatus,
		"unresolved":       row.UnresolvedCount,
		"settle_secs":      crSettleSeconds,
		"reason":           "cr_settle_window_expired",
	})
	if _, aerr := queries.CreateActivity(ctx, db.CreateActivityParams{
		WorkspaceID: updated.WorkspaceID,
		IssueID:     pgtype.UUID{Bytes: updated.ID.Bytes, Valid: true},
		ActorType:   pgtype.Text{String: "system", Valid: true},
		Action:      "review_comments_unresolved",
		Details:     details,
	}); aerr != nil {
		slog.Warn("cr-settle sweeper: activity write failed",
			"issue", util.UUIDToString(row.IssueID),
			"error", aerr,
		)
	}

	bus.Publish(events.Event{
		Type:        protocol.EventIssueUpdated,
		WorkspaceID: util.UUIDToString(updated.WorkspaceID),
		ActorType:   "system",
		Payload: map[string]any{
			"id":             util.UUIDToString(updated.ID),
			"status":         updated.Status,
			"prev":           "coderabbit",
			"prev_status":    "coderabbit",
			"status_changed": true,
			"source":         "cr_settle_sweeper",
			"src_event":      "settle_window_expired",
			"unresolved":     row.UnresolvedCount,
		},
	})
}
