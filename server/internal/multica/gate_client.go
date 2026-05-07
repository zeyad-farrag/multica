package multica

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const GateTimeout = 500 * time.Millisecond

type GateAssignee struct {
	Type *string `json:"assignee_type,omitempty"`
	ID   *string `json:"assignee_id,omitempty"`
}

type GatePatch struct {
	Assignee        *GateAssignee `json:"assignee,omitempty"`
	EstimateMinutes *int          `json:"estimate_minutes,omitempty"`
	DueDate         *time.Time    `json:"due_date,omitempty"`
	Status          *string       `json:"status,omitempty"`
}

func (p GatePatch) IsTimeRelevant() bool {
	if p.Assignee != nil || p.EstimateMinutes != nil || p.DueDate != nil {
		return true
	}
	if p.Status == nil {
		return false
	}
	switch strings.TrimSpace(strings.ToLower(*p.Status)) {
	case "backlog", "done", "cancelled":
		return false
	default:
		return true
	}
}

type GateRequestBody struct {
	WorkspaceID string    `json:"workspace_id"`
	IssueID     string    `json:"issue_id"`
	ActorUserID string    `json:"actor_user_id"`
	Force       bool      `json:"force"`
	Patch       GatePatch `json:"patch"`
}

type GateOutcome string

const (
	GateOutcomeAllow    GateOutcome = "allow"
	GateOutcomeDeny     GateOutcome = "deny"
	GateOutcomeFailOpen GateOutcome = "fail-open"
)

type GateResult struct {
	Outcome    GateOutcome
	StatusCode int
	Body       []byte
	Err        error
}

type GateClient struct {
	baseURL    string
	secret     string
	httpClient *http.Client
}

func NewGateClient(baseURL, secret string) *GateClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	secret = strings.TrimSpace(secret)
	if baseURL == "" || secret == "" {
		return nil
	}
	return &GateClient{
		baseURL:    baseURL,
		secret:     secret,
		httpClient: &http.Client{Timeout: GateTimeout},
	}
}

func (c *GateClient) Enabled() bool {
	return c != nil && c.baseURL != "" && c.secret != ""
}

func (c *GateClient) CallGate(ctx context.Context, body GateRequestBody) GateResult {
	if !c.Enabled() || !body.Patch.IsTimeRelevant() {
		return GateResult{Outcome: GateOutcomeAllow, StatusCode: http.StatusOK}
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return GateResult{Outcome: GateOutcomeFailOpen, Err: err}
	}

	ctx, cancel := context.WithTimeout(ctx, GateTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/system/issue-update-gate", bytes.NewReader(payload))
	if err != nil {
		return GateResult{Outcome: GateOutcomeFailOpen, Err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Team-App-Secret", c.secret)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		slog.Warn("team-app gate transport failed", "error", err)
		return GateResult{Outcome: GateOutcomeFailOpen, Err: err}
	}
	defer resp.Body.Close()

	respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		slog.Warn("team-app gate response read failed", "error", readErr)
		return GateResult{Outcome: GateOutcomeFailOpen, StatusCode: resp.StatusCode, Err: readErr}
	}

	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		return GateResult{Outcome: GateOutcomeAllow, StatusCode: resp.StatusCode, Body: respBody}
	case http.StatusConflict:
		return GateResult{Outcome: GateOutcomeDeny, StatusCode: resp.StatusCode, Body: respBody}
	default:
		slog.Warn("team-app gate returned fail-open status", "status", resp.StatusCode)
		return GateResult{Outcome: GateOutcomeFailOpen, StatusCode: resp.StatusCode, Body: respBody}
	}
}
