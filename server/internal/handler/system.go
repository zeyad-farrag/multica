package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type ActivityResponse struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	IssueID     *string         `json:"issue_id"`
	ActorType   *string         `json:"actor_type"`
	ActorID     *string         `json:"actor_id"`
	Action      string          `json:"action"`
	Details     json.RawMessage `json:"details"`
	CreatedAt   string          `json:"created_at"`
}

func activityToResponse(a db.ActivityLog) ActivityResponse {
	return ActivityResponse{
		ID:          uuidToString(a.ID),
		WorkspaceID: uuidToString(a.WorkspaceID),
		IssueID:     uuidToPtr(a.IssueID),
		ActorType:   textToPtr(a.ActorType),
		ActorID:     uuidToPtr(a.ActorID),
		Action:      a.Action,
		Details:     bytesToRawJSON(a.Details),
		CreatedAt:   timestampToString(a.CreatedAt),
	}
}

func systemLimit(r *http.Request) (int32, bool) {
	limit := int32(200)
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return limit, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, false
	}
	if n > 1000 {
		n = 1000
	}
	return int32(n), true
}

func parseOptionalTimestamp(raw string) (pgtype.Timestamptz, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return pgtype.Timestamptz{}, true
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return pgtype.Timestamptz{}, false
	}
	return pgtype.Timestamptz{Time: t, Valid: true}, true
}

func parseOptionalDate(raw string) (pgtype.Date, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return pgtype.Date{}, true
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return pgtype.Date{}, false
	}
	return pgtype.Date{Time: t, Valid: true}, true
}

func parseIssueCursor(raw string) (pgtype.Timestamptz, pgtype.UUID, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return pgtype.Timestamptz{}, pgtype.UUID{}, true
	}
	idx := strings.LastIndex(raw, ":")
	if idx < 0 {
		return pgtype.Timestamptz{}, pgtype.UUID{}, false
	}
	t, ok := parseOptionalTimestamp(raw[:idx])
	if !ok || !t.Valid {
		return pgtype.Timestamptz{}, pgtype.UUID{}, false
	}
	id := parseUUID(raw[idx+1:])
	if !id.Valid {
		return pgtype.Timestamptz{}, pgtype.UUID{}, false
	}
	return t, id, true
}

func (h *Handler) SystemGetWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID := chi.URLParam(r, "workspaceID")
	ws, err := h.Queries.GetWorkspace(r.Context(), parseUUID(workspaceID))
	if err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "workspace not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get workspace")
		return
	}
	writeJSON(w, http.StatusOK, workspaceToResponse(ws))
}

func (h *Handler) SystemListIssuesByWorkspace(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, ok := systemLimit(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid limit parameter")
		return
	}
	updatedSince, ok := parseOptionalTimestamp(q.Get("updated_since"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid updated_since parameter")
		return
	}
	cursorUpdatedAt, cursorID, ok := parseIssueCursor(q.Get("cursor"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid cursor parameter")
		return
	}

	workspaceID := chi.URLParam(r, "workspaceID")
	issues, err := h.Queries.SystemListIssuesByWorkspace(r.Context(), db.SystemListIssuesByWorkspaceParams{
		WorkspaceID:     parseUUID(workspaceID),
		UpdatedSince:    updatedSince,
		CursorUpdatedAt: cursorUpdatedAt,
		CursorID:        cursorID,
		LimitCount:      limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list issues")
		return
	}
	prefix := h.getIssuePrefix(r.Context(), parseUUID(workspaceID))
	resp := make([]IssueResponse, len(issues))
	for i, issue := range issues {
		resp[i] = issueToResponse(issue, prefix)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) SystemListCommentsByWorkspace(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, ok := systemLimit(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid limit parameter")
		return
	}
	commentDate, ok := parseOptionalDate(q.Get("date"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid date parameter")
		return
	}
	authorID := pgtype.UUID{}
	if raw := strings.TrimSpace(q.Get("author_id")); raw != "" {
		authorID = parseUUID(raw)
		if !authorID.Valid {
			writeError(w, http.StatusBadRequest, "invalid author_id parameter")
			return
		}
	}
	commentType := pgtype.Text{}
	if raw := strings.TrimSpace(q.Get("type")); raw != "" {
		commentType = pgtype.Text{String: raw, Valid: true}
	}

	comments, err := h.Queries.SystemListCommentsByWorkspace(r.Context(), db.SystemListCommentsByWorkspaceParams{
		WorkspaceID: parseUUID(chi.URLParam(r, "workspaceID")),
		AuthorID:    authorID,
		CommentType: commentType,
		CommentDate: commentDate,
		LimitCount:  limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list comments")
		return
	}
	resp := make([]CommentResponse, len(comments))
	for i, comment := range comments {
		resp[i] = commentToResponse(comment, nil, nil)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) SystemListActivityByWorkspace(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, ok := systemLimit(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid limit parameter")
		return
	}
	since, ok := parseOptionalTimestamp(q.Get("since"))
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid since parameter")
		return
	}
	actorID := pgtype.UUID{}
	if raw := strings.TrimSpace(q.Get("actor_id")); raw != "" {
		actorID = parseUUID(raw)
		if !actorID.Valid {
			writeError(w, http.StatusBadRequest, "invalid actor_id parameter")
			return
		}
	}
	action := pgtype.Text{}
	if raw := strings.TrimSpace(q.Get("action")); raw != "" {
		action = pgtype.Text{String: raw, Valid: true}
	}

	items, err := h.Queries.SystemListActivityByWorkspace(r.Context(), db.SystemListActivityByWorkspaceParams{
		WorkspaceID: parseUUID(chi.URLParam(r, "workspaceID")),
		Since:       since,
		Action:      action,
		ActorID:     actorID,
		LimitCount:  limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list activity")
		return
	}
	resp := make([]ActivityResponse, len(items))
	for i, item := range items {
		resp[i] = activityToResponse(item)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) SystemListMembersByWorkspace(w http.ResponseWriter, r *http.Request) {
	members, err := h.Queries.ListMembersWithUser(r.Context(), parseUUID(chi.URLParam(r, "workspaceID")))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list members")
		return
	}
	resp := make([]MemberWithUserResponse, len(members))
	for i, m := range members {
		resp[i] = MemberWithUserResponse{
			ID:          uuidToString(m.ID),
			WorkspaceID: uuidToString(m.WorkspaceID),
			UserID:      uuidToString(m.UserID),
			Role:        m.Role,
			CreatedAt:   timestampToString(m.CreatedAt),
			Name:        m.UserName,
			Email:       m.UserEmail,
			AvatarURL:   textToPtr(m.UserAvatarUrl),
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) SystemCreateInboxItem(w http.ResponseWriter, r *http.Request) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	required := []string{"workspace_id", "recipient_type", "recipient_id", "type", "severity", "issue_id", "title", "body", "actor_type", "actor_id", "details"}
	for _, key := range required {
		if _, ok := raw[key]; !ok {
			writeError(w, http.StatusBadRequest, "missing required key: "+key)
			return
		}
	}

	var req struct {
		WorkspaceID   string          `json:"workspace_id"`
		RecipientType string          `json:"recipient_type"`
		RecipientID   string          `json:"recipient_id"`
		Type          string          `json:"type"`
		Severity      string          `json:"severity"`
		IssueID       *string         `json:"issue_id"`
		Title         string          `json:"title"`
		Body          *string         `json:"body"`
		ActorType     string          `json:"actor_type"`
		ActorID       *string         `json:"actor_id"`
		Details       json.RawMessage `json:"details"`
	}
	body, _ := json.Marshal(raw)
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ActorType != "system" && req.ActorID == nil {
		writeError(w, http.StatusBadRequest, "actor_id is required for non-system actors")
		return
	}
	details := req.Details
	if len(details) == 0 || string(details) == "null" {
		details = []byte(`{}`)
	}
	item, err := h.Queries.CreateInboxItem(r.Context(), db.CreateInboxItemParams{
		WorkspaceID:   parseUUID(req.WorkspaceID),
		RecipientType: req.RecipientType,
		RecipientID:   parseUUID(req.RecipientID),
		Type:          req.Type,
		Severity:      req.Severity,
		IssueID:       nullableUUID(req.IssueID),
		Title:         req.Title,
		Body:          ptrToText(req.Body),
		ActorType:     pgtype.Text{String: req.ActorType, Valid: req.ActorType != ""},
		ActorID:       nullableUUID(req.ActorID),
		Details:       details,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to create inbox item")
		return
	}
	writeJSON(w, http.StatusOK, inboxToResponse(item))
}
