package codex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/msitarzewski/agent-room/internal/adapters"
	"github.com/msitarzewski/agent-room/internal/domain"
)

type record struct {
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	ThreadID  string          `json:"thread_id"`
	TurnID    string          `json:"turn_id"`
	Item      json.RawMessage `json:"item"`
	Error     json.RawMessage `json:"error"`
	Usage     json.RawMessage `json:"usage"`
}

type itemSummary struct {
	ID   string `json:"id,omitempty"`
	Type string `json:"type,omitempty"`
}

func DecodeJSONL(reader io.Reader, adapterContext adapters.Context) ([]domain.Event, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 4<<20)
	var events []domain.Event
	for line := 1; scanner.Scan(); line++ {
		var record record
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, fmt.Errorf("codex JSONL line %d: %w", line, err)
		}
		eventType, subjectType, subjectID := mapRecord(record, adapterContext)
		if eventType == "" {
			continue
		}
		var item itemSummary
		_ = json.Unmarshal(record.Item, &item)
		event, err := adapterContext.Event(eventType, subjectType, subjectID, map[string]any{
			"source": "codex_jsonl", "source_type": record.Type, "thread_id": record.ThreadID,
			"turn_id": record.TurnID, "item": item, "error": record.Error, "usage": record.Usage,
		}, record.Timestamp)
		if err != nil {
			return nil, err
		}
		event.SourceSystem = "codex"
		event.SourceSequence = int64(line)
		event.SourceEventID = fmt.Sprintf("%s:%s:%s:%d", record.ThreadID, record.TurnID, item.ID, line)
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read codex JSONL: %w", err)
	}
	return events, nil
}

func mapRecord(record record, adapterContext adapters.Context) (string, domain.ResourceType, string) {
	switch record.Type {
	case "thread.started":
		return "session.started", domain.ResourceSession, first(record.ThreadID, adapterContext.SessionID)
	case "turn.started":
		return "run.started", domain.ResourceRun, first(record.TurnID, adapterContext.RunID)
	case "item.started":
		return "run.progressed", domain.ResourceRun, first(record.TurnID, adapterContext.RunID)
	case "item.completed":
		return "evidence.observed", domain.ResourceRun, first(record.TurnID, adapterContext.RunID)
	case "turn.completed":
		return "run.completed", domain.ResourceRun, first(record.TurnID, adapterContext.RunID)
	case "turn.failed", "error":
		return "run.failed", domain.ResourceRun, first(record.TurnID, adapterContext.RunID)
	default:
		return "", "", ""
	}
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "unknown"
}
