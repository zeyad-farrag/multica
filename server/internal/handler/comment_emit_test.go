package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

func createEventTestIssue(t *testing.T) string {
	t.Helper()

	var issueID string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO issue (workspace_id, title, status, priority, creator_type, creator_id, position, number)
		VALUES ($1, $2, 'todo', 'medium', 'member', $3, 0,
			(SELECT COALESCE(MAX(number), 0) + 1 FROM issue WHERE workspace_id = $1))
		RETURNING id
	`, testWorkspaceID, "Event emit test issue", testUserID).Scan(&issueID); err != nil {
		t.Fatalf("create event test issue: %v", err)
	}

	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, issueID)
	})

	return issueID
}

func createEventTestUser(t *testing.T, prefix string) (string, string) {
	t.Helper()

	email := fmt.Sprintf("%s-%s@multica.ai", prefix, randomID())
	var userID string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO "user" (name, email)
		VALUES ($1, $2)
		RETURNING id
	`, prefix, email).Scan(&userID); err != nil {
		t.Fatalf("create event test user: %v", err)
	}

	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM "user" WHERE id = $1`, userID)
	})

	return userID, email
}

func createEventTestMember(t *testing.T, prefix, role string) (string, string) {
	t.Helper()

	userID, _ := createEventTestUser(t, prefix)

	var memberID string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO member (workspace_id, user_id, role)
		VALUES ($1, $2, $3)
		RETURNING id
	`, testWorkspaceID, userID, role).Scan(&memberID); err != nil {
		t.Fatalf("create event test member: %v", err)
	}

	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM member WHERE id = $1`, memberID)
	})

	return memberID, userID
}

func TestCommentEmit_StatusChangeFiresCommentCreated(t *testing.T) {
	var (
		mu       sync.Mutex
		captured []events.Event
	)

	bus := events.New()
	bus.Subscribe(protocol.EventCommentCreated, func(event events.Event) {
		mu.Lock()
		defer mu.Unlock()
		captured = append(captured, event)
	})

	originalBus := testHandler.Bus
	testHandler.Bus = bus
	t.Cleanup(func() {
		testHandler.Bus = originalBus
	})

	issueID := createEventTestIssue(t)

	req := newRequest(http.MethodPost, "/api/issues/"+issueID+"/comments", map[string]any{
		"content": "moved to in_progress",
		"type":    "status_change",
	})
	req = withURLParam(req, "id", issueID)

	recorder := httptest.NewRecorder()
	testHandler.CreateComment(recorder, req)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, recorder.Code, recorder.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()

	if len(captured) != 1 {
		t.Fatalf("expected 1 comment event, got %d", len(captured))
	}

	event := captured[0]
	if event.Type != protocol.EventCommentCreated {
		t.Fatalf("expected event type %q, got %q", protocol.EventCommentCreated, event.Type)
	}
	if event.WorkspaceID != testWorkspaceID {
		t.Fatalf("expected workspace %q, got %q", testWorkspaceID, event.WorkspaceID)
	}
	if event.ActorType != "member" {
		t.Fatalf("expected actor type %q, got %q", "member", event.ActorType)
	}
	if event.ActorID != testUserID {
		t.Fatalf("expected actor id %q, got %q", testUserID, event.ActorID)
	}

	payload, ok := event.Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected payload type map[string]any, got %T", event.Payload)
	}

	comment, ok := payload["comment"].(CommentResponse)
	if !ok {
		t.Fatalf("expected comment payload type %T, got %T", CommentResponse{}, payload["comment"])
	}
	if comment.IssueID != issueID {
		t.Fatalf("expected issue id %q, got %q", issueID, comment.IssueID)
	}
	if comment.Type != "status_change" {
		t.Fatalf("expected comment type %q, got %q", "status_change", comment.Type)
	}
}
