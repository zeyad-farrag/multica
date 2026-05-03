package github

import "testing"

func TestDecide_PROpened(t *testing.T) {
	cases := []struct {
		name   string
		status string
		want   Decision
	}{
		{
			name:   "from todo links + sets coderabbit",
			status: "todo",
			want:   Decision{Action: ActionLinkPR, NewStatus: StatusCoderabbit, ActivityKind: "pr_opened"},
		},
		{
			name:   "from in_progress links + sets coderabbit",
			status: "in_progress",
			want:   Decision{Action: ActionLinkPR, NewStatus: StatusCoderabbit, ActivityKind: "pr_opened"},
		},
		{
			name:   "from testing links + sets coderabbit",
			status: StatusTesting,
			want:   Decision{Action: ActionLinkPR, NewStatus: StatusCoderabbit, ActivityKind: "pr_opened"},
		},
		{
			name:   "from coderabbit preserves status (re-delivery)",
			status: StatusCoderabbit,
			want:   Decision{Action: ActionLinkPR, NewStatus: StatusCoderabbit, ActivityKind: "pr_opened"},
		},
		{
			name:   "from resolving preserves status",
			status: StatusResolving,
			want:   Decision{Action: ActionLinkPR, NewStatus: StatusResolving, ActivityKind: "pr_opened"},
		},
		{
			name:   "from in_review preserves status",
			status: StatusInReview,
			want:   Decision{Action: ActionLinkPR, NewStatus: StatusInReview, ActivityKind: "pr_opened"},
		},
		{
			name:   "from staged preserves status",
			status: StatusStaged,
			want:   Decision{Action: ActionLinkPR, NewStatus: StatusStaged, ActivityKind: "pr_opened"},
		},
		{
			name:   "from done preserves status",
			status: StatusDone,
			want:   Decision{Action: ActionLinkPR, NewStatus: StatusDone, ActivityKind: "pr_opened"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Decide(Input{
				Kind:        EventKindPR,
				IssueStatus: tc.status,
				PRAction:    PRActionOpened,
			})
			if got != tc.want {
				t.Fatalf("got %+v; want %+v", got, tc.want)
			}
		})
	}
}

// PRActionSynchronize is intentionally noop in the new CR-resolution flow.
// Bounce-backs are driven by review events and sidecar markers, not by raw
// push events.
func TestDecide_PRSynchronize_AlwaysNoop(t *testing.T) {
	statuses := []string{
		StatusInProgress,
		StatusCodeReview,
		StatusFixing,
		StatusTesting,
		StatusCoderabbit,
		StatusResolving,
		StatusInReview,
		StatusStaged,
	}
	for _, s := range statuses {
		t.Run("from "+s+" is noop", func(t *testing.T) {
			got := Decide(Input{
				Kind: EventKindPR, IssueStatus: s, PRAction: PRActionSynchronize,
			})
			if got.Action != ActionNoop {
				t.Fatalf("from %s: got %+v; want noop", s, got)
			}
		})
	}
}

func TestIsAgentPusher(t *testing.T) {
	cases := []struct {
		login string
		want  bool
	}{
		{"bmad-amelia", true},
		{"BMAD-Amelia", true},
		{"BMAD-WINSTON", true},
		{"bmad-quinn", true},
		{"bmad-murat", true},
		{"some-human", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsAgentPusher(tc.login); got != tc.want {
			t.Errorf("IsAgentPusher(%q) = %v; want %v", tc.login, got, tc.want)
		}
	}
}

func TestDecide_PRClosed(t *testing.T) {
	t.Run("merged from staged → done", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindPR, IssueStatus: StatusStaged, PRAction: PRActionClosed, Merged: true,
		})
		want := Decision{Action: ActionSetStatus, NewStatus: StatusDone, ActivityKind: "pr_merged"}
		if got != want {
			t.Fatalf("got %+v; want %+v", got, want)
		}
	})
	t.Run("merged from coderabbit → staged (preserve audit)", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindPR, IssueStatus: StatusCoderabbit, PRAction: PRActionClosed, Merged: true,
		})
		want := Decision{Action: ActionSetStatus, NewStatus: StatusStaged, ActivityKind: "pr_merged_from_coderabbit"}
		if got != want {
			t.Fatalf("got %+v; want %+v", got, want)
		}
	})
	t.Run("merged from in_review → staged (preserve audit)", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindPR, IssueStatus: StatusInReview, PRAction: PRActionClosed, Merged: true,
		})
		want := Decision{Action: ActionSetStatus, NewStatus: StatusStaged, ActivityKind: "pr_merged_from_in_review"}
		if got != want {
			t.Fatalf("got %+v; want %+v", got, want)
		}
	})
	t.Run("merged from done is noop", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindPR, IssueStatus: StatusDone, PRAction: PRActionClosed, Merged: true,
		})
		if got.Action != ActionNoop {
			t.Fatalf("got %+v; want noop", got)
		}
	})
	t.Run("unmerged flips to blocked", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindPR, IssueStatus: StatusCoderabbit, PRAction: PRActionClosed, Merged: false,
		})
		want := Decision{Action: ActionSetStatus, NewStatus: StatusBlocked, ActivityKind: "pr_closed_unmerged"}
		if got != want {
			t.Fatalf("got %+v; want %+v", got, want)
		}
	})
	t.Run("unmerged from blocked is noop", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindPR, IssueStatus: StatusBlocked, PRAction: PRActionClosed, Merged: false,
		})
		if got.Action != ActionNoop {
			t.Fatalf("got %+v; want noop", got)
		}
	})
}

func TestDecide_PRReopened(t *testing.T) {
	t.Run("from blocked → coderabbit", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindPR, IssueStatus: StatusBlocked, PRAction: PRActionReopened,
		})
		want := Decision{Action: ActionSetStatus, NewStatus: StatusCoderabbit, ActivityKind: "pr_reopened"}
		if got != want {
			t.Fatalf("got %+v; want %+v", got, want)
		}
	})
	t.Run("from done → coderabbit", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindPR, IssueStatus: StatusDone, PRAction: PRActionReopened,
		})
		want := Decision{Action: ActionSetStatus, NewStatus: StatusCoderabbit, ActivityKind: "pr_reopened"}
		if got != want {
			t.Fatalf("got %+v; want %+v", got, want)
		}
	})
	t.Run("from coderabbit is noop", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindPR, IssueStatus: StatusCoderabbit, PRAction: PRActionReopened,
		})
		if got.Action != ActionNoop {
			t.Fatalf("got %+v; want noop", got)
		}
	})
}

func TestDecide_ReviewChangesRequested(t *testing.T) {
	t.Run("from coderabbit by CR → resolving", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindReview, IssueStatus: StatusCoderabbit,
			ReviewState: ReviewChangesRequested, ReviewByCR: true,
		})
		want := Decision{Action: ActionSetStatus, NewStatus: StatusResolving, ActivityKind: "review_changes_requested"}
		if got != want {
			t.Fatalf("got %+v; want %+v", got, want)
		}
	})
	t.Run("from in_review by CR → resolving (re-round)", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindReview, IssueStatus: StatusInReview,
			ReviewState: ReviewChangesRequested, ReviewByCR: true,
		})
		if got.NewStatus != StatusResolving {
			t.Fatalf("got %+v; want NewStatus=resolving", got)
		}
	})
	t.Run("from staged by CR is noop (CR can't reopen staged)", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindReview, IssueStatus: StatusStaged,
			ReviewState: ReviewChangesRequested, ReviewByCR: true,
		})
		if got.Action != ActionNoop {
			t.Fatalf("got %+v; want noop", got)
		}
	})
	t.Run("non-CR reviewer is ignored", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindReview, IssueStatus: StatusCoderabbit,
			ReviewState: ReviewChangesRequested, ReviewByCR: false,
		})
		if got.Action != ActionNoop {
			t.Fatalf("got %+v; want noop", got)
		}
	})
	t.Run("already resolving is noop", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindReview, IssueStatus: StatusResolving,
			ReviewState: ReviewChangesRequested, ReviewByCR: true,
		})
		if got.Action != ActionNoop {
			t.Fatalf("got %+v; want noop", got)
		}
	})
}

func TestDecide_ReviewCommentedWithUnresolved(t *testing.T) {
	t.Run("from coderabbit + unresolved → resolving (soft-changes)", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindReview, IssueStatus: StatusCoderabbit,
			ReviewState: ReviewCommented, ReviewByCR: true,
			LocalUnresolvedThreadCount: 3,
		})
		want := Decision{Action: ActionSetStatus, NewStatus: StatusResolving, ActivityKind: "review_comments_unresolved"}
		if got != want {
			t.Fatalf("got %+v; want %+v", got, want)
		}
	})
	t.Run("from in_review + unresolved → resolving (re-round)", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindReview, IssueStatus: StatusInReview,
			ReviewState: ReviewCommented, ReviewByCR: true,
			LocalUnresolvedThreadCount: 1,
		})
		if got.NewStatus != StatusResolving {
			t.Fatalf("got %+v; want resolving", got)
		}
	})
	t.Run("from resolving + unresolved is noop (already there)", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindReview, IssueStatus: StatusResolving,
			ReviewState: ReviewCommented, ReviewByCR: true,
			LocalUnresolvedThreadCount: 3,
		})
		if got.Action != ActionNoop {
			t.Fatalf("got %+v; want noop", got)
		}
	})
}

func TestDecide_ReviewClean(t *testing.T) {
	// Regression: a COMMENTED review with predicate-clean is the racing
	// shape. GitHub delivers `pull_request_review` before per-finding
	// `pull_request_review_comment` webhooks, so on a COMMENTED review with
	// inline findings the local mirror is still empty here. We must NOT
	// promote — the inline-comment ingest re-evaluation drives → resolving
	// once the rows land.
	t.Run("commented first pass from coderabbit waits for inline events (noop)", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindReview, IssueStatus: StatusCoderabbit,
			ReviewState: ReviewCommented, ReviewByCR: true,
			NoOpenCRChangesRequest: true, NoUnresolvedCRThreads: true,
		})
		if got.Action != ActionNoop {
			t.Fatalf("commented predicate-clean must noop on coderabbit (race guard); got %+v", got)
		}
	})
	t.Run("approved review from coderabbit + predicate → staged", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindReview, IssueStatus: StatusCoderabbit,
			ReviewState: ReviewApproved, ReviewByCR: true,
			NoOpenCRChangesRequest: true, NoUnresolvedCRThreads: true,
		})
		want := Decision{Action: ActionSetStatus, NewStatus: StatusStaged, ActivityKind: "review_passed"}
		if got != want {
			t.Fatalf("got %+v; want %+v", got, want)
		}
	})
	t.Run("commented from in_review waits for inline events (noop)", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindReview, IssueStatus: StatusInReview,
			ReviewState: ReviewCommented, ReviewByCR: true,
			NoOpenCRChangesRequest: true, NoUnresolvedCRThreads: true,
		})
		if got.Action != ActionNoop {
			t.Fatalf("commented predicate-clean must noop on in_review (race guard); got %+v", got)
		}
	})
	t.Run("approved from in_review (Marcus done) → staged", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindReview, IssueStatus: StatusInReview,
			ReviewState: ReviewApproved, ReviewByCR: true,
			NoOpenCRChangesRequest: true, NoUnresolvedCRThreads: true,
		})
		if got.NewStatus != StatusStaged {
			t.Fatalf("got %+v; want NewStatus=staged", got)
		}
	})
	t.Run("predicate fails (open changes) → noop", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindReview, IssueStatus: StatusCoderabbit,
			ReviewState: ReviewApproved, ReviewByCR: true,
			NoOpenCRChangesRequest: false, NoUnresolvedCRThreads: true,
		})
		if got.Action != ActionNoop {
			t.Fatalf("got %+v; want noop", got)
		}
	})
	t.Run("from resolving with clean predicate is noop (sidecar drives resolving)", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindReview, IssueStatus: StatusResolving,
			ReviewState: ReviewApproved, ReviewByCR: true,
			NoOpenCRChangesRequest: true, NoUnresolvedCRThreads: true,
		})
		if got.Action != ActionNoop {
			t.Fatalf("got %+v; want noop", got)
		}
	})
}

func TestDecide_ReviewThread(t *testing.T) {
	// Inline findings present on coderabbit drive → resolving even without
	// a wrapping review event. This branch is what handleReviewComment's
	// per-comment re-evaluation reaches; it closes the COMMENTED-review
	// race that decideReview's APPROVED gate leaves to the inline path.
	t.Run("unresolved threads from coderabbit → resolving", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindReviewThread, IssueStatus: StatusCoderabbit,
			NoOpenCRChangesRequest: true, NoUnresolvedCRThreads: false,
			LocalUnresolvedThreadCount: 5,
		})
		want := Decision{Action: ActionSetStatus, NewStatus: StatusResolving, ActivityKind: "review_comments_unresolved"}
		if got != want {
			t.Fatalf("got %+v; want %+v", got, want)
		}
	})
	// Phase 1 invariant: thread events on in_review do NOT drive any
	// state-machine transition. Marcus's /resolve calls in the
	// bmad-pr-resolve loop fire pull_request_review_thread.resolved events;
	// reacting to them would (a) bounce in_review → resolving on each
	// resolve and (b) violate the "ONLY APPROVED → staged" rule when the
	// last one drains the count to zero. The in_review exit is sidecar-driven
	// (Marcus emits a marker; sidecar flips in_review → coderabbit).
	t.Run("unresolved threads from in_review is noop (sidecar drives in_review)", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindReviewThread, IssueStatus: StatusInReview,
			NoOpenCRChangesRequest: true, NoUnresolvedCRThreads: false,
			LocalUnresolvedThreadCount: 1,
		})
		if got.Action != ActionNoop {
			t.Fatalf("got %+v; want noop", got)
		}
	})
	t.Run("unresolved threads from resolving is noop (already there)", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindReviewThread, IssueStatus: StatusResolving,
			NoOpenCRChangesRequest: true, NoUnresolvedCRThreads: false,
			LocalUnresolvedThreadCount: 2,
		})
		if got.Action != ActionNoop {
			t.Fatalf("got %+v; want noop", got)
		}
	})
	// Phase 1 invariant: predicate-clear thread events do NOT promote any
	// column to staged. Only an explicit APPROVED CR review (handled in
	// decideReview) can drive `→ staged` autonomously.
	t.Run("predicate clear from coderabbit is noop (req: APPROVED only → staged)", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindReviewThread, IssueStatus: StatusCoderabbit,
			NoOpenCRChangesRequest: true, NoUnresolvedCRThreads: true,
		})
		if got.Action != ActionNoop {
			t.Fatalf("got %+v; want noop", got)
		}
	})
	t.Run("predicate clear from in_review is noop (sidecar drives this column)", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindReviewThread, IssueStatus: StatusInReview,
			NoOpenCRChangesRequest: true, NoUnresolvedCRThreads: true,
		})
		if got.Action != ActionNoop {
			t.Fatalf("got %+v; want noop", got)
		}
	})
	t.Run("from resolving is noop (sidecar drives this column)", func(t *testing.T) {
		got := Decide(Input{
			Kind: EventKindReviewThread, IssueStatus: StatusResolving,
			NoOpenCRChangesRequest: true, NoUnresolvedCRThreads: true,
		})
		if got.Action != ActionNoop {
			t.Fatalf("got %+v; want noop", got)
		}
	})
}
