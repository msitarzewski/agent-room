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
	threadID := adapterContext.SessionID
	for line := 1; scanner.Scan(); line++ {
		var record record
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, fmt.Errorf("invalid codex JSONL record at line %d", line)
		}
		if record.ThreadID != "" {
			threadID = adapters.MetadataToken(record.ThreadID, 512)
			if threadID == "" {
				return nil, fmt.Errorf("invalid native codex thread identifier")
			}
		}
		record.ThreadID = threadID
		if record.TurnID != "" {
			record.TurnID = adapters.MetadataToken(record.TurnID, 512)
			if record.TurnID == "" {
				return nil, fmt.Errorf("invalid native codex turn identifier")
			}
		}
		eventType, subjectType, subjectID := mapRecord(record, adapterContext)
		if eventType == "" {
			continue
		}
		if adapters.MetadataToken(subjectID, 512) == "" {
			return nil, fmt.Errorf("codex record requires an explicit native session or run identity")
		}
		var item itemSummary
		_ = json.Unmarshal(record.Item, &item)
		item.ID = adapters.MetadataToken(item.ID, 512)
		item.Type = adapters.MetadataToken(item.Type, 64)
		event, err := adapterContext.Event(eventType, subjectType, subjectID, map[string]any{
			"source": "codex_jsonl", "source_type": record.Type, "thread_id": record.ThreadID,
			"turn_id": adapters.MetadataToken(record.TurnID, 512), "agent_id": adapterContext.AgentID,
			"item": item, "has_error": len(record.Error) > 0, "usage": adapters.UsageCounts(record.Usage),
		}, record.Timestamp)
		if err != nil {
			return nil, err
		}
		event.SourceSystem = "codex"
		event.SourceSequence, err = adapters.SourceSequence(adapterContext.SourceSequence, int64(line))
		if err != nil {
			return nil, err
		}
		event.SourceEventID = adapters.StableSourceID(adapterContext.NativeEventID, adapterContext.ProjectID, adapterContext.AgentID, record.ThreadID, subjectID, record.Type, item.ID, fmt.Sprint(event.SourceSequence))
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
	return ""
}
