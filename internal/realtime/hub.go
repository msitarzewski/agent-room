package realtime

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/msitarzewski/agent-room/internal/domain"
)

type Message struct {
	Cursor     int64           `json:"cursor"`
	Type       string          `json:"type"`
	OccurredAt time.Time       `json:"occurred_at"`
	Data       json.RawMessage `json:"data,omitempty"`
}

type subscriber struct {
	projectID string
	ch        chan Message
}

type Hub struct {
	mu          sync.RWMutex
	subscribers map[*subscriber]struct{}
}

func NewHub() *Hub {
	return &Hub{subscribers: make(map[*subscriber]struct{})}
}

func (h *Hub) Publish(event domain.Event) {
	raw, _ := json.Marshal(event)
	message := Message{Cursor: event.Cursor, Type: event.Type, OccurredAt: event.OccurredAt, Data: raw}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for subscriber := range h.subscribers {
		if subscriber.projectID != event.ProjectID {
			continue
		}
		select {
		case subscriber.ch <- message:
		default:
			select {
			case <-subscriber.ch:
			default:
			}
			resync, _ := json.Marshal(map[string]any{"after": event.Cursor})
			select {
			case subscriber.ch <- Message{Cursor: event.Cursor, Type: "resync_required", OccurredAt: time.Now().UTC(), Data: resync}:
			default:
			}
		}
	}
}

func (h *Hub) Subscribe(projectID string) (<-chan Message, func()) {
	sub := &subscriber{projectID: projectID, ch: make(chan Message, 256)}
	h.mu.Lock()
	h.subscribers[sub] = struct{}{}
	h.mu.Unlock()
	cancel := func() {
		h.mu.Lock()
		if _, ok := h.subscribers[sub]; ok {
			delete(h.subscribers, sub)
			close(sub.ch)
		}
		h.mu.Unlock()
	}
	return sub.ch, cancel
}

func WriteLoop(ctx context.Context, conn *websocket.Conn, messages <-chan Message, initial []domain.Event, validate func(context.Context) error) error {
	for _, event := range initial {
		raw, _ := json.Marshal(event)
		if err := write(ctx, conn, Message{Cursor: event.Cursor, Type: event.Type, OccurredAt: event.OccurredAt, Data: raw}); err != nil {
			return err
		}
	}
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case message, ok := <-messages:
			if !ok {
				return nil
			}
			if err := write(ctx, conn, message); err != nil {
				return err
			}
		case now := <-heartbeat.C:
			if validate != nil {
				validateCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				err := validate(validateCtx)
				cancel()
				if err != nil {
					return err
				}
			}
			if err := write(ctx, conn, Message{Type: "heartbeat", OccurredAt: now.UTC()}); err != nil {
				return err
			}
		}
	}
}

func write(ctx context.Context, conn *websocket.Conn, value Message) error {
	writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return conn.Write(writeCtx, websocket.MessageText, mustJSON(value))
}

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}
