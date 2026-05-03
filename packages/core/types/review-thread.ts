/**
 * Mirror of the Multica `issue_review_thread` row, returned by
 * `GET /api/issues/{id}/review-threads`.
 *
 * One row per CodeRabbit inline comment on the issue's PR. The webhook
 * upserts these on every `pull_request_review_comment.created/edited`
 * event; the row is the canonical record of what CR found.
 */
export interface ReviewThread {
  id: string;
  issue_id: string;
  pr_repo: string;
  pr_number: number;
  gh_comment_id: number;
  gh_thread_node_id: string;
  file_path: string;
  line?: number | null;
  side?: string;
  /** Normalized severity: issue / refactor / nitpick / suggestion / unknown. */
  severity: string;
  /** CR's coloured tag: Critical / Major / Minor / Trivial / Blocker / unknown. */
  severity_badge: string;
  /** CR's effort tag: Quick win / Heavy lift / Poor tradeoff / Low value / unknown. */
  effort_badge: string;
  title: string;
  body: string;
  /** Verbatim contents of CR's "🤖 Prompt for AI Agents" fenced block. */
  ai_prompt: string;
  url: string;
  author_login: string;
  /** unresolved / resolved / outdated / wont_fix. */
  state: string;
  resolved_by_agent?: string | null;
  resolved_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface ListReviewThreadsResponse {
  issue_id: string;
  state_filter: string;
  threads: ReviewThread[];
  total: number;
}
