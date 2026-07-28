package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/msitarzewski/agent-room/internal/domain"
)

func TestHubScopesPublishesAndForcesResyncOnOverflow(t *testing.T) {
	t.Parallel()
	hub := NewHub()
	messages, cancel := hub.Subscribe("project-one")
	other, cancelOther := hub.Subscribe("project-two")
	defer cancelOther()
	at := time.Now().UTC()
	hub.Publish(domain.Event{Cursor: 1, ProjectID: "project-one", Type: "task.updated", OccurredAt: at})
	select {
	case message := <-messages:
		if message.Cursor != 1 || message.Type != "task.updated" || len(message.Data) == 0 {
			t.Fatalf("message=%+v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("project subscriber did not receive event")
	}
	select {
	case message := <-other:
		t.Fatalf("cross-project event leaked: %+v", message)
	default:
	}
	for cursor := int64(2); cursor <= 258; cursor++ {
		hub.Publish(domain.Event{Cursor: cursor, ProjectID: "project-one", Type: "run.progressed", OccurredAt: at})
	}
	foundResync := false
	for len(messages) > 0 {
		if message := <-messages; message.Type == "resync_required" {
			foundResync = true
			var payload map[string]int64
			if err := json.Unmarshal(message.Data, &payload); err != nil || payload["after"] == 0 {
				t.Fatalf("resync payload=%s err=%v", message.Data, err)
			}
		}
	}
	if !foundResync {
		t.Fatal("overflow did not request resynchronization")
	}
	cancel()
	cancel()
	if _, ok := <-messages; ok {
		t.Fatal("cancel did not close subscription")
	}
}

func TestWriteLoopReplaysStreamsAndCloses(t *testing.T) {
	t.Parallel()
	serverDone := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.CloseNow()
		messages := make(chan Message, 1)
		messages <- Message{Cursor: 2, Type: "run.progressed", OccurredAt: time.Now().UTC()}
		close(messages)
		serverDone <- WriteLoop(r.Context(), conn, messages, []domain.Event{{
			Cursor: 1, ProjectID: "p", Type: "run.started", OccurredAt: time.Now().UTC(),
		}}, nil)
	}))
	defer server.Close()
	conn, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	for _, want := range []string{"run.started", "run.progressed"} {
		_, raw, err := conn.Read(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		var message Message
		if err := json.Unmarshal(raw, &message); err != nil || message.Type != want {
			t.Fatalf("message=%s decoded=%+v err=%v", raw, message, err)
		}
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}

func TestWriteLoopHonorsCancellationAndJSONHelper(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := mustJSON(Message{Type: "heartbeat"}); !json.Valid(got) {
		t.Fatalf("invalid JSON: %s", got)
	}
	if err := WriteLoop(ctx, nil, make(chan Message), nil, nil); err != context.Canceled {
		t.Fatalf("cancellation error=%v", err)
	}
}
