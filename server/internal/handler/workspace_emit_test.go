package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

func withEventTestURLParams(req *http.Request, pairs ...string) *http.Request {
	rctx := chi.NewRouteContext()
	for i := 0; i+1 < len(pairs); i += 2 {
		rctx.URLParams.Add(pairs[i], pairs[i+1])
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestWorkspaceEmit(t *testing.T) {
	t.Run("MemberAdded", func(t *testing.T) {
		var (
			mu       sync.Mutex
			captured []events.Event
		)

		bus := events.New()
		bus.Subscribe(protocol.EventMemberAdded, func(event events.Event) {
			mu.Lock()
			defer mu.Unlock()
			captured = append(captured, event)
		})

		originalBus := testHandler.Bus
		testHandler.Bus = bus
		t.Cleanup(func() {
			testHandler.Bus = originalBus
		})

		email := fmt.Sprintf("member-added-%s@multica.ai", randomID())
		req := newRequest(http.MethodPost, "/api/workspaces/"+testWorkspaceID+"/members", map[string]any{
			"email": email,
			"role":  "member",
		})
		req = withURLParam(req, "id", testWorkspaceID)

		recorder := httptest.NewRecorder()
		testHandler.CreateMember(recorder, req)

		if recorder.Code != http.StatusCreated {
			t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, recorder.Code, recorder.Body.String())
		}

		var created MemberWithUserResponse
		if err := json.NewDecoder(recorder.Body).Decode(&created); err != nil {
			t.Fatalf("decode create member response: %v", err)
		}

		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM "user" WHERE id = $1`, created.UserID)
		})
		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM member WHERE id = $1`, created.ID)
		})

		mu.Lock()
		defer mu.Unlock()

		if len(captured) != 1 {
			t.Fatalf("expected 1 member added event, got %d", len(captured))
		}

		event := captured[0]
		if event.WorkspaceID != testWorkspaceID {
			t.Fatalf("expected workspace %q, got %q", testWorkspaceID, event.WorkspaceID)
		}

		payload, ok := event.Payload.(map[string]any)
		if !ok {
			t.Fatalf("expected payload type map[string]any, got %T", event.Payload)
		}

		member, ok := payload["member"].(MemberWithUserResponse)
		if !ok {
			t.Fatalf("expected member payload type %T, got %T", MemberWithUserResponse{}, payload["member"])
		}
		if member.ID != created.ID {
			t.Fatalf("expected member id %q, got %q", created.ID, member.ID)
		}
		if member.Role != "member" {
			t.Fatalf("expected role %q, got %q", "member", member.Role)
		}
	})

	t.Run("MemberUpdated", func(t *testing.T) {
		var (
			mu       sync.Mutex
			captured []events.Event
		)

		bus := events.New()
		bus.Subscribe(protocol.EventMemberUpdated, func(event events.Event) {
			mu.Lock()
			defer mu.Unlock()
			captured = append(captured, event)
		})

		originalBus := testHandler.Bus
		testHandler.Bus = bus
		t.Cleanup(func() {
			testHandler.Bus = originalBus
		})

		memberID, _ := createEventTestMember(t, "member-updated", "member")

		req := newRequest(http.MethodPatch, "/api/workspaces/"+testWorkspaceID+"/members/"+memberID, map[string]any{
			"role": "admin",
		})
		req = withEventTestURLParams(req, "id", testWorkspaceID, "memberId", memberID)

		recorder := httptest.NewRecorder()
		testHandler.UpdateMember(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
		}

		var updated MemberWithUserResponse
		if err := json.NewDecoder(recorder.Body).Decode(&updated); err != nil {
			t.Fatalf("decode update member response: %v", err)
		}

		mu.Lock()
		defer mu.Unlock()

		if len(captured) != 1 {
			t.Fatalf("expected 1 member updated event, got %d", len(captured))
		}

		payload, ok := captured[0].Payload.(map[string]any)
		if !ok {
			t.Fatalf("expected payload type map[string]any, got %T", captured[0].Payload)
		}

		member, ok := payload["member"].(MemberWithUserResponse)
		if !ok {
			t.Fatalf("expected member payload type %T, got %T", MemberWithUserResponse{}, payload["member"])
		}
		if member.ID != memberID {
			t.Fatalf("expected member id %q, got %q", memberID, member.ID)
		}
		if member.Role != "admin" {
			t.Fatalf("expected role %q, got %q", "admin", member.Role)
		}
		if updated.Role != "admin" {
			t.Fatalf("expected response role %q, got %q", "admin", updated.Role)
		}
	})

	t.Run("MemberRemoved", func(t *testing.T) {
		var (
			mu       sync.Mutex
			captured []events.Event
		)

		bus := events.New()
		bus.Subscribe(protocol.EventMemberRemoved, func(event events.Event) {
			mu.Lock()
			defer mu.Unlock()
			captured = append(captured, event)
		})

		originalBus := testHandler.Bus
		testHandler.Bus = bus
		t.Cleanup(func() {
			testHandler.Bus = originalBus
		})

		memberID, userID := createEventTestMember(t, "member-removed", "member")

		req := newRequest(http.MethodDelete, "/api/workspaces/"+testWorkspaceID+"/members/"+memberID, nil)
		req = withEventTestURLParams(req, "id", testWorkspaceID, "memberId", memberID)

		recorder := httptest.NewRecorder()
		testHandler.DeleteMember(recorder, req)

		if recorder.Code != http.StatusNoContent {
			t.Fatalf("expected status %d, got %d: %s", http.StatusNoContent, recorder.Code, recorder.Body.String())
		}

		mu.Lock()
		defer mu.Unlock()

		if len(captured) != 1 {
			t.Fatalf("expected 1 member removed event, got %d", len(captured))
		}

		payload, ok := captured[0].Payload.(map[string]any)
		if !ok {
			t.Fatalf("expected payload type map[string]any, got %T", captured[0].Payload)
		}

		memberEventID, ok := payload["member_id"].(string)
		if !ok {
			t.Fatalf("expected member_id payload to be string, got %T", payload["member_id"])
		}
		if memberEventID != memberID {
			t.Fatalf("expected member id %q, got %q", memberID, memberEventID)
		}

		userEventID, ok := payload["user_id"].(string)
		if !ok {
			t.Fatalf("expected user_id payload to be string, got %T", payload["user_id"])
		}
		if userEventID != userID {
			t.Fatalf("expected user id %q, got %q", userID, userEventID)
		}

		workspaceEventID, ok := payload["workspace_id"].(string)
		if !ok {
			t.Fatalf("expected workspace_id payload to be string, got %T", payload["workspace_id"])
		}
		if workspaceEventID != testWorkspaceID {
			t.Fatalf("expected workspace id %q, got %q", testWorkspaceID, workspaceEventID)
		}
	})

	t.Run("WorkspaceUpdated", func(t *testing.T) {
		var (
			mu       sync.Mutex
			captured []events.Event
		)

		bus := events.New()
		bus.Subscribe(protocol.EventWorkspaceUpdated, func(event events.Event) {
			mu.Lock()
			defer mu.Unlock()
			captured = append(captured, event)
		})

		originalBus := testHandler.Bus
		testHandler.Bus = bus
		t.Cleanup(func() {
			testHandler.Bus = originalBus
		})

		var originalName string
		var originalDescription string
		if err := testPool.QueryRow(context.Background(), `
			SELECT name, description
			FROM workspace
			WHERE id = $1
		`, testWorkspaceID).Scan(&originalName, &originalDescription); err != nil {
			t.Fatalf("load original workspace: %v", err)
		}

		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), `
				UPDATE workspace
				SET name = $2, description = $3
				WHERE id = $1
			`, testWorkspaceID, originalName, originalDescription)
		})

		updatedName := "Handler Tests Updated"
		updatedDescription := "Updated workspace description for emit assertions"
		req := newRequest(http.MethodPatch, "/api/workspaces/"+testWorkspaceID, map[string]any{
			"name":        updatedName,
			"description": updatedDescription,
		})
		req = withURLParam(req, "id", testWorkspaceID)

		recorder := httptest.NewRecorder()
		testHandler.UpdateWorkspace(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
		}

		var updated WorkspaceResponse
		if err := json.NewDecoder(recorder.Body).Decode(&updated); err != nil {
			t.Fatalf("decode update workspace response: %v", err)
		}

		mu.Lock()
		defer mu.Unlock()

		if len(captured) != 1 {
			t.Fatalf("expected 1 workspace updated event, got %d", len(captured))
		}

		event := captured[0]
		if event.WorkspaceID != testWorkspaceID {
			t.Fatalf("expected workspace %q, got %q", testWorkspaceID, event.WorkspaceID)
		}

		payload, ok := event.Payload.(map[string]any)
		if !ok {
			t.Fatalf("expected payload type map[string]any, got %T", event.Payload)
		}

		workspace, ok := payload["workspace"].(WorkspaceResponse)
		if !ok {
			t.Fatalf("expected workspace payload type %T, got %T", WorkspaceResponse{}, payload["workspace"])
		}
		if workspace.ID != testWorkspaceID {
			t.Fatalf("expected workspace id %q, got %q", testWorkspaceID, workspace.ID)
		}
		if workspace.Name != updatedName {
			t.Fatalf("expected workspace name %q, got %q", updatedName, workspace.Name)
		}
		if workspace.Description == nil || *workspace.Description != updatedDescription {
			t.Fatalf("expected workspace description %q, got %+v", updatedDescription, workspace.Description)
		}
		if updated.Name != updatedName {
			t.Fatalf("expected response name %q, got %q", updatedName, updated.Name)
		}
	})
}
