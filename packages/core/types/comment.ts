export type CommentType =
  | "comment"
  | "status_change"
  | "progress_update"
  | "system"
  | "debug"
  | "impl_plan"
  | "completion_note"
  | "change_log"
  | "review"
  | "cr_review_comment"
  | "fixer_reply";

export type CommentAuthorType = "member" | "agent" | "system";

export interface Reaction {
  id: string;
  comment_id: string;
  actor_type: string;
  actor_id: string;
  emoji: string;
  created_at: string;
}

export interface Comment {
  id: string;
  issue_id: string;
  author_type: CommentAuthorType;
  author_id: string;
  content: string;
  type: CommentType;
  parent_id: string | null;
  /** Set on `cr_review_comment` rows only; the link from a CR thread to its timeline entry. */
  review_thread_id?: string | null;
  /** Set on `fixer_reply` rows after Marcus mirrors them to GitHub via the review-threads/reply endpoint. NULL or undefined = pending. */
  posted_to_github_at?: string | null;
  reactions: Reaction[];
  attachments: import("./attachment").Attachment[];
  created_at: string;
  updated_at: string;
}
