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

// ActivityResponse is the JSON shape returned by ListWorkspaceActivity. Field
// layout mirrors the activity slice of TimelineEntry so the standalone
// reconciler can parse both endpoints with one struct.
type ActivityResponse struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	IssueID     *string         `json:"issue_id"`
	ActorType   *string         `json:"actor_type"`
	ActorID     string          `json:"actor_id"`
	Action      string          `json:"action"`
	Details     json.RawMessage `json:"details,omitempty"`
	CreatedAt   string          `json:"created_at"`
}

func activityToResponse(a db.ActivityLog) ActivityResponse {
	resp := ActivityResponse{
		ID:          uuidToString(a.ID),
		WorkspaceID: uuidToString(a.WorkspaceID),
		IssueID:     uuidToPtr(a.IssueID),
		ActorType:   textToPtr(a.ActorType),
		ActorID:     uuidToString(a.ActorID),
		Action:      a.Action,
		CreatedAt:   timestampToString(a.CreatedAt),
	}
	if len(a.Details) > 0 {
		resp.Details = json.RawMessage(a.Details)
	}
	return resp
}

// SPEC: §6.1 #5, §19.17, §22 M-PR#3 — Story 1.4 read portion. Workspace-
// scoped activity scan for the team-app reconciler's gate-bypass detection.
// Cursor format <RFC3339Nano>:<UUID>; fetch limit+1 to set next_cursor only
// when more rows exist.
func (h *Handler) ListWorkspaceActivity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	workspaceID := chi.URLParam(r, "id")
	if workspaceID == "" {
		writeError(w, http.StatusBadRequest, "workspace id is required")
		return
	}
	wsUUID := parseUUID(workspaceID)
	if !wsUUID.Valid {
		writeError(w, http.StatusBadRequest, "invalid workspace id")
		return
	}

	q := r.URL.Query()

	rawSince := q.Get("since")
	if rawSince == "" {
		writeError(w, http.StatusBadRequest, "since is required")
		return
	}
	since, err := time.Parse(time.RFC3339, rawSince)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid since; expected RFC3339")
		return
	}

	var actionFilter pgtype.Text
	if a := q.Get("action"); a != "" {
		actionFilter = pgtype.Text{String: a, Valid: true}
	}

	var actorFilter pgtype.UUID
	if a := q.Get("actor_id"); a != "" {
		actorFilter = parseUUID(a)
		if !actorFilter.Valid {
			writeError(w, http.StatusBadRequest, "invalid actor_id")
			return
		}
	}

	cursorTS, cursorID, err := parseCursor(q.Get("cursor"))
	if err != nil {
		writeInvalidCursor(w)
		return
	}

	limit, err := parseLimit(q.Get("limit"), 200, 1000)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid limit; must be 1-1000")
		return
	}

	cursorTSCol := pgtype.Timestamptz{}
	if !cursorTS.IsZero() {
		cursorTSCol = pgtype.Timestamptz{Time: cursorTS, Valid: true}
	}

	rows, err := h.Queries.ListActivityByWorkspace(ctx, db.ListActivityByWorkspaceParams{
		WorkspaceID:     wsUUID,
		CreatedAt:       pgtype.Timestamptz{Time: since, Valid: true},
		Limit:           int32(limit + 1),
		Action:          actionFilter,
		ActorID:         actorFilter,
		CursorCreatedAt: cursorTSCol,
		CursorID:        cursorID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list activity")
		return
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	resp := make([]ActivityResponse, len(rows))
	for i, a := range rows {
		resp[i] = activityToResponse(a)
	}

	var nextCursor *string
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		c := encodeCursor(last.CreatedAt.Time, last.ID)
		nextCursor = &c
	}

	total, err := h.Queries.CountActivityByWorkspace(ctx, db.CountActivityByWorkspaceParams{
		WorkspaceID: wsUUID,
		CreatedAt:   pgtype.Timestamptz{Time: since, Valid: true},
		Action:      actionFilter,
		ActorID:     actorFilter,
	})
	if err != nil {
		total = int64(len(resp))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"activity":    resp,
		"next_cursor": nextCursor,
		"total":       total,
	})
}
