package handler

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// TimelineEntry represents a single entry in the issue timeline, which can be
// either an activity log record or a comment.
type TimelineEntry struct {
	Type string `json:"type"` // "activity" or "comment"
	ID   string `json:"id"`

	ActorType string `json:"actor_type"`
	ActorID   string `json:"actor_id"`
	CreatedAt string `json:"created_at"`

	// Activity-only fields
	Action  *string         `json:"action,omitempty"`
	Details json.RawMessage `json:"details,omitempty"`

	// Comment-only fields
	Content     *string              `json:"content,omitempty"`
	ParentID    *string              `json:"parent_id,omitempty"`
	UpdatedAt   *string              `json:"updated_at,omitempty"`
	CommentType *string              `json:"comment_type,omitempty"`
	Reactions   []ReactionResponse   `json:"reactions,omitempty"`
	Attachments []AttachmentResponse `json:"attachments,omitempty"`
}

type WorkspaceActivityResponse struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	IssueID     *string         `json:"issue_id"`
	ActorType   string          `json:"actor_type"`
	ActorID     string          `json:"actor_id"`
	Action      string          `json:"action"`
	Details     json.RawMessage `json:"details"`
	CreatedAt   string          `json:"created_at"`
}

func activityToWorkspaceResponse(a db.ActivityLog) WorkspaceActivityResponse {
	actorType := ""
	if a.ActorType.Valid {
		actorType = a.ActorType.String
	}
	return WorkspaceActivityResponse{
		ID:          uuidToString(a.ID),
		WorkspaceID: uuidToString(a.WorkspaceID),
		IssueID:     uuidToPtr(a.IssueID),
		ActorType:   actorType,
		ActorID:     uuidToString(a.ActorID),
		Action:      a.Action,
		Details:     a.Details,
		CreatedAt:   timestampToString(a.CreatedAt),
	}
}

// ListTimeline returns a merged, chronologically-sorted timeline of activities
// and comments for a given issue.
func (h *Handler) ListTimeline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	issue, ok := h.loadIssueForUser(w, r, id)
	if !ok {
		return
	}

	activities, err := h.Queries.ListActivities(r.Context(), db.ListActivitiesParams{
		IssueID: issue.ID,
		Limit:   200,
		Offset:  0,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list activities")
		return
	}

	comments, err := h.Queries.ListComments(r.Context(), db.ListCommentsParams{
		IssueID:     issue.ID,
		WorkspaceID: issue.WorkspaceID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list comments")
		return
	}

	timeline := make([]TimelineEntry, 0, len(activities)+len(comments))

	for _, a := range activities {
		action := a.Action
		actorType := ""
		if a.ActorType.Valid {
			actorType = a.ActorType.String
		}
		timeline = append(timeline, TimelineEntry{
			Type:      "activity",
			ID:        uuidToString(a.ID),
			ActorType: actorType,
			ActorID:   uuidToString(a.ActorID),
			Action:    &action,
			Details:   a.Details,
			CreatedAt: timestampToString(a.CreatedAt),
		})
	}

	// Fetch reactions and attachments for all comments in one batch.
	commentIDs := make([]pgtype.UUID, len(comments))
	for i, c := range comments {
		commentIDs[i] = c.ID
	}
	grouped := h.groupReactions(r, commentIDs)
	groupedAtt := h.groupAttachments(r, commentIDs)

	for _, c := range comments {
		content := c.Content
		commentType := c.Type
		updatedAt := timestampToString(c.UpdatedAt)
		cid := uuidToString(c.ID)
		timeline = append(timeline, TimelineEntry{
			Type:        "comment",
			ID:          cid,
			ActorType:   c.AuthorType,
			ActorID:     uuidToString(c.AuthorID),
			Content:     &content,
			CommentType: &commentType,
			ParentID:    uuidToPtr(c.ParentID),
			CreatedAt:   timestampToString(c.CreatedAt),
			UpdatedAt:   &updatedAt,
			Reactions:   grouped[cid],
			Attachments: groupedAtt[cid],
		})
	}

	// Sort chronologically (ascending by created_at)
	sort.Slice(timeline, func(i, j int) bool {
		return timeline[i].CreatedAt < timeline[j].CreatedAt
	})

	writeJSON(w, http.StatusOK, timeline)
}

// SPEC: §6.1 #5 — M-PR#3 read portion (Story 1.4).
func (h *Handler) ListWorkspaceActivity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workspaceID := chi.URLParam(r, "id")
	wsUUID, ok := parseRequiredUUID(workspaceID)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid workspace id")
		return
	}

	q := r.URL.Query()
	sinceRaw := q.Get("since")
	if sinceRaw == "" {
		writeError(w, http.StatusBadRequest, "since is required")
		return
	}
	since, err := time.Parse(time.RFC3339, sinceRaw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid since")
		return
	}
	limit, ok := parseLimit(r, 200, 1000)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_limit")
		return
	}

	var action pgtype.Text
	if raw := q.Get("action"); raw != "" {
		action = pgtype.Text{String: raw, Valid: true}
	}
	var actorID pgtype.UUID
	if raw := q.Get("actor_id"); raw != "" {
		var parsed bool
		actorID, parsed = parseRequiredUUID(raw)
		if !parsed {
			writeError(w, http.StatusBadRequest, "invalid actor_id")
			return
		}
	}
	var cursorTS pgtype.Timestamptz
	var cursorID pgtype.UUID
	if raw := q.Get("cursor"); raw != "" {
		cursorTime, parsedID, err := parseCursor(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_cursor")
			return
		}
		cursorTS = cursorTimestamptz(cursorTime)
		cursorID = parsedID
	}

	rows, err := h.Queries.ListActivityByWorkspace(ctx, db.ListActivityByWorkspaceParams{
		WorkspaceID: wsUUID,
		CreatedAt:   pgtype.Timestamptz{Time: since, Valid: true},
		Limit:       limit + 1,
		Action:      action,
		ActorID:     actorID,
		CursorTs:    cursorTS,
		CursorID:    cursorID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list activity")
		return
	}
	hasMore := len(rows) > int(limit)
	if hasMore {
		rows = rows[:int(limit)]
	}

	total, err := h.Queries.CountActivityByWorkspace(ctx, db.CountActivityByWorkspaceParams{
		WorkspaceID: wsUUID,
		CreatedAt:   pgtype.Timestamptz{Time: since, Valid: true},
		Action:      action,
		ActorID:     actorID,
	})
	if err != nil {
		total = int64(len(rows))
	}

	resp := make([]WorkspaceActivityResponse, len(rows))
	for i, row := range rows {
		resp[i] = activityToWorkspaceResponse(row)
	}
	var nextCursor *string
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		cursor := encodeCursor(last.CreatedAt.Time, last.ID)
		nextCursor = &cursor
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"activity":    resp,
		"next_cursor": nextCursor,
		"total":       total,
	})
}

// AssigneeFrequencyEntry represents how often a user assigns to a specific target.
type AssigneeFrequencyEntry struct {
	AssigneeType string `json:"assignee_type"`
	AssigneeID   string `json:"assignee_id"`
	Frequency    int64  `json:"frequency"`
}

// GetAssigneeFrequency returns assignee usage frequency for the current user,
// combining data from assignee change activities and initial issue assignments.
func (h *Handler) GetAssigneeFrequency(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := h.resolveWorkspaceID(r)

	// Aggregate frequency from both data sources.
	freq := map[string]int64{} // key: "type:id"

	// Source 1: assignee_changed activities by this user.
	activityCounts, err := h.Queries.CountAssigneeChangesByActor(r.Context(), db.CountAssigneeChangesByActorParams{
		WorkspaceID: parseUUID(workspaceID),
		ActorID:     parseUUID(userID),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get assignee frequency")
		return
	}
	for _, row := range activityCounts {
		aType, _ := row.AssigneeType.(string)
		aID, _ := row.AssigneeID.(string)
		if aType != "" && aID != "" {
			freq[aType+":"+aID] += row.Frequency
		}
	}

	// Source 2: issues created by this user with an assignee.
	issueCounts, err := h.Queries.CountCreatedIssueAssignees(r.Context(), db.CountCreatedIssueAssigneesParams{
		WorkspaceID: parseUUID(workspaceID),
		CreatorID:   parseUUID(userID),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get assignee frequency")
		return
	}
	for _, row := range issueCounts {
		if !row.AssigneeType.Valid || !row.AssigneeID.Valid {
			continue
		}
		key := row.AssigneeType.String + ":" + uuidToString(row.AssigneeID)
		freq[key] += row.Frequency
	}

	// Build sorted response.
	result := make([]AssigneeFrequencyEntry, 0, len(freq))
	for key, count := range freq {
		// Split "type:id" — type is always "member" or "agent" (no colons).
		var aType, aID string
		for i := 0; i < len(key); i++ {
			if key[i] == ':' {
				aType = key[:i]
				aID = key[i+1:]
				break
			}
		}
		result = append(result, AssigneeFrequencyEntry{
			AssigneeType: aType,
			AssigneeID:   aID,
			Frequency:    count,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Frequency > result[j].Frequency
	})

	writeJSON(w, http.StatusOK, result)
}
