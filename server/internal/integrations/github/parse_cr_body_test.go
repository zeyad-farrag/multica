package github

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseCRBody_Fixtures runs against real CR comment bodies captured
// from production deliveries (PR #19 family). The fixtures are the
// authoritative format CodeRabbit currently emits; if CR changes its
// markup, regenerate by selecting fresh rows from issue_review_thread.
func TestParseCRBody_Fixtures(t *testing.T) {
	cases := []struct {
		fixture           string
		wantSeverity      string
		wantSeverityBadge string
		wantEffortBadge   string
		wantTitle         string
		wantPromptPrefix  string // first ~40 chars of the AI prompt block
	}{
		{
			fixture:           "01_two_badges_no_effort.md",
			wantSeverity:      "issue",
			wantSeverityBadge: "Major",
			wantEffortBadge:   "unknown",
			wantTitle:         "Run the frontend runtime stage as a non-root user.",
			wantPromptPrefix:  "Verify each finding against the current code",
		},
		{
			fixture:           "02_three_badges_quick_win_compact.md",
			wantSeverity:      "nitpick",
			wantSeverityBadge: "Trivial",
			wantEffortBadge:   "Quick win",
			wantTitle:         "Exclude unrelated files from the API build context.",
			wantPromptPrefix:  "Verify each finding against the current code",
		},
		{
			fixture:           "03_three_badges_quick_win.md",
			wantSeverity:      "issue",
			wantSeverityBadge: "Minor",
			wantEffortBadge:   "Quick win",
			wantTitle:         "Differentiate missing vs invalid env failures in logs.",
		},
		{
			fixture:           "04_poor_tradeoff.md",
			wantSeverity:      "nitpick",
			wantSeverityBadge: "Trivial",
			wantEffortBadge:   "Poor tradeoff",
			wantTitle:         "Docker helpers duplicated across scripts.",
			wantPromptPrefix:  "Verify each finding against the current code",
		},
		{
			fixture:           "05_low_value.md",
			wantSeverity:      "nitpick",
			wantSeverityBadge: "Trivial",
			wantEffortBadge:   "Low value",
		},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join("testdata", "cr_comments", tc.fixture))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			got := parseCRBody(string(body))

			if got.Severity != tc.wantSeverity {
				t.Errorf("severity: got %q want %q", got.Severity, tc.wantSeverity)
			}
			if got.SeverityBadge != tc.wantSeverityBadge {
				t.Errorf("severity_badge: got %q want %q", got.SeverityBadge, tc.wantSeverityBadge)
			}
			if got.EffortBadge != tc.wantEffortBadge {
				t.Errorf("effort_badge: got %q want %q", got.EffortBadge, tc.wantEffortBadge)
			}
			if tc.wantTitle != "" && got.Title != tc.wantTitle {
				t.Errorf("title: got %q want %q", got.Title, tc.wantTitle)
			}
			if tc.wantPromptPrefix != "" && !strings.HasPrefix(got.AIPrompt, tc.wantPromptPrefix) {
				t.Errorf("ai_prompt prefix: got %q want prefix %q", got.AIPrompt[:min(len(got.AIPrompt), 60)], tc.wantPromptPrefix)
			}
		})
	}
}

// TestParseCRBody_EmptyBody covers the trivial path.
func TestParseCRBody_EmptyBody(t *testing.T) {
	got := parseCRBody("")
	if got.Severity != "unknown" || got.SeverityBadge != "unknown" || got.EffortBadge != "unknown" || got.Title != "" || got.AIPrompt != "" {
		t.Fatalf("expected all-default for empty input; got %+v", got)
	}
}

// TestParseCRBody_NonCRComment ensures a free-text comment (e.g. a human
// inline review) doesn't accidentally produce false-positive badges.
func TestParseCRBody_NonCRComment(t *testing.T) {
	got := parseCRBody("This is a regular comment, not from CodeRabbit.\n\nMaybe a major issue here.")
	if got.SeverityBadge != "unknown" {
		t.Errorf("severity_badge: got %q want unknown (no badge line)", got.SeverityBadge)
	}
	if got.AIPrompt != "" {
		t.Errorf("ai_prompt: got %q want empty", got.AIPrompt)
	}
}

// TestParseCRBody_TitleCap protects against runaway titles.
func TestParseCRBody_TitleCap(t *testing.T) {
	long := strings.Repeat("x", 500)
	body := "_⚠️ Potential issue_\n\n**" + long + "**\n"
	got := parseCRBody(body)
	if len(got.Title) != 140 {
		t.Errorf("title len: got %d want 140", len(got.Title))
	}
}
