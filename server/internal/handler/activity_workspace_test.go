package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestListWorkspaceActivityFiltersAndPaginates(t *testing.T) {
	ctx := context.Background()
	ts := time.Date(2026, 4, 30, 9, 0, 0, 0, time.UTC)
	ids := []string{
		"00000000-0000-0000-0000-000000000051",
		"00000000-0000-0000-0000-000000000052",
	}
	for _, id := range ids {
		_, err := testPool.Exec(ctx, `
			INSERT INTO activity_log (id, workspace_id, actor_type, actor_id, action, details, created_at)
			VALUES ($1, $2, 'member', $3, 'gate_bypassed', '{"outcome":"blocked"}'::jsonb, $4)
		`, id, testWorkspaceID, testUserID, ts)
		if err != nil {
			t.Fatalf("seed activity: %v", err)
		}
		t.Cleanup(func() {
			testPool.Exec(ctx, `DELETE FROM activity_log WHERE id = $1`, id)
		})
	}

	w := httptest.NewRecorder()
	req := newRequest("GET", "/api/workspaces/"+testWorkspaceID+"/activity?since="+ts.Format(time.RFC3339)+"&action=gate_bypassed&limit=1", nil)
	req = withURLParam(req, "id", testWorkspaceID)
	testHandler.ListWorkspaceActivity(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first page: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var first struct {
		Activity   []WorkspaceActivityResponse `json:"activity"`
		NextCursor *string                     `json:"next_cursor"`
		Total      int64                       `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&first); err != nil {
		t.Fatalf("decode first page: %v", err)
	}
	if len(first.Activity) != 1 || first.NextCursor == nil || first.Total != 2 {
		t.Fatalf("unexpected first page: %+v", first)
	}
	if string(first.Activity[0].Details) != `{"outcome": "blocked"}` && string(first.Activity[0].Details) != `{"outcome":"blocked"}` {
		t.Fatalf("details not returned: %s", string(first.Activity[0].Details))
	}

	w = httptest.NewRecorder()
	req = newRequest("GET", "/api/workspaces/"+testWorkspaceID+"/activity?since="+ts.Format(time.RFC3339)+"&action=gate_bypassed&limit=1&cursor="+*first.NextCursor, nil)
	req = withURLParam(req, "id", testWorkspaceID)
	testHandler.ListWorkspaceActivity(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("second page: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var second struct {
		Activity []WorkspaceActivityResponse `json:"activity"`
	}
	if err := json.NewDecoder(w.Body).Decode(&second); err != nil {
		t.Fatalf("decode second page: %v", err)
	}
	if len(second.Activity) != 1 || second.Activity[0].ID != ids[1] {
		t.Fatalf("unexpected second page: %+v", second.Activity)
	}
}
