package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type crAttemptResponse struct {
	ID                string  `json:"id"`
	IssueID           string  `json:"issue_id"`
	WorkspaceID       string  `json:"workspace_id"`
	CrRound           int32   `json:"cr_round"`
	PrUrl             string  `json:"pr_url"`
	HeadSha           string  `json:"head_sha"`
	StartedAt         string  `json:"started_at"`
	ReviewSubmittedAt *string `json:"review_submitted_at"`
	ReviewState       *string `json:"review_state"`
	FindingsCount     int32   `json:"findings_count"`
	Outcome           *string `json:"outcome"`
	OutcomeReason     *string `json:"outcome_reason"`
	ClosedAt          *string `json:"closed_at"`
	FirstSignalAt     *string `json:"first_signal_at"`
	FirstSignalKind   *string `json:"first_signal_kind"`
}

type crSignalResponse struct {
	ID             string         `json:"id"`
	AttemptID      string         `json:"attempt_id"`
	SignalKind     string         `json:"signal_kind"`
	SignalAction   *string        `json:"signal_action"`
	ReceivedAt     string         `json:"received_at"`
	PayloadSummary map[string]any `json:"payload_summary"`
}

func (h *Handler) ListCRAttempts(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	rows, err := h.Queries.ListCRAttemptsForIssue(r.Context(), issue.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := make([]crAttemptResponse, 0, len(rows))
	for _, row := range rows {
		resp = append(resp, crAttemptToResponse(row))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) ListCRSignals(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	attemptID := parseUUID(chi.URLParam(r, "attemptID"))
	if !attemptID.Valid {
		writeError(w, http.StatusBadRequest, "invalid attempt id")
		return
	}
	attempt, err := h.Queries.GetCRReviewAttemptByID(r.Context(), attemptID)
	if err != nil {
		writeError(w, http.StatusNotFound, "attempt not found")
		return
	}
	if uuidToString(attempt.IssueID) != uuidToString(issue.ID) {
		writeError(w, http.StatusForbidden, "attempt does not belong to this issue")
		return
	}
	rows, err := h.Queries.ListCRSignalsForAttempt(r.Context(), attemptID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := make([]crSignalResponse, 0, len(rows))
	for _, row := range rows {
		resp = append(resp, crSignalToResponse(row))
	}
	writeJSON(w, http.StatusOK, resp)
}

func crAttemptToResponse(row db.CrReviewAttempt) crAttemptResponse {
	return crAttemptResponse{
		ID:                uuidToString(row.ID),
		IssueID:           uuidToString(row.IssueID),
		WorkspaceID:       uuidToString(row.WorkspaceID),
		CrRound:           row.CrRound,
		PrUrl:             row.PrUrl,
		HeadSha:           row.HeadSha,
		StartedAt:         row.StartedAt.Time.Format("2006-01-02T15:04:05.000Z"),
		ReviewSubmittedAt: timestampToPtr(row.ReviewSubmittedAt),
		ReviewState:       textToPtr(row.ReviewState),
		FindingsCount:     row.FindingsCount,
		Outcome:           textToPtr(row.Outcome),
		OutcomeReason:     textToPtr(row.OutcomeReason),
		ClosedAt:          timestampToPtr(row.ClosedAt),
		FirstSignalAt:     timestampToPtr(row.FirstSignalAt),
		FirstSignalKind:   textToPtr(row.FirstSignalKind),
	}
}

func crSignalToResponse(row db.CrReviewSignal) crSignalResponse {
	var summary map[string]any
	if len(row.PayloadSummary) > 0 {
		_ = json.Unmarshal(row.PayloadSummary, &summary)
	}
	return crSignalResponse{
		ID:             uuidToString(row.ID),
		AttemptID:      uuidToString(row.AttemptID),
		SignalKind:     row.SignalKind,
		SignalAction:   textToPtr(row.SignalAction),
		ReceivedAt:     row.ReceivedAt.Time.Format("2006-01-02T15:04:05.000Z"),
		PayloadSummary: summary,
	}
}
