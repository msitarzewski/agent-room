package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/msitarzewski/agent-room/internal/adapters"
	"github.com/msitarzewski/agent-room/internal/domain"
)

type streamRecord struct {
	Type              string          `json:"type"`
	Subtype           string          `json:"subtype"`
	SessionID         string          `json:"session_id"`
	Timestamp         time.Time       `json:"timestamp"`
	Message           json.RawMessage `json:"message"`
	Result            json.RawMessage `json:"result"`
	Error             string          `json:"error"`
	IsError           bool            `json:"is_error"`
	ClaudeCodeVersion string          `json:"claude_code_version"`
	Model             string          `json:"model"`
	Tools             []string        `json:"tools"`
	Usage             json.RawMessage `json:"usage"`
	RateLimitInfo     json.RawMessage `json:"rate_limit_info"`
	CostUSD           float64         `json:"total_cost_usd"`
	DurationMS        int64           `json:"duration_ms"`
}

type HookPayload struct {
	HookEventName  string          `json:"hook_event_name"`
	SessionID      string          `json:"session_id"`
	CWD            string          `json:"cwd"`
	TranscriptPath string          `json:"transcript_path"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input"`
	ToolResponse   json.RawMessage `json:"tool_response"`
}

func DecodeStreamJSON(reader io.Reader, adapterContext adapters.Context) ([]domain.Event, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 4<<20)
	var events []domain.Event
	for line := 1; scanner.Scan(); line++ {
		var record streamRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, fmt.Errorf("claude stream-json line %d: %w", line, err)
		}
		eventType, subjectType := mapStreamType(record)
		if eventType == "" {
			continue
		}
		subject := record.SessionID
		if subject == "" {
			subject = adapterContext.RunID
		}
		event, err := adapterContext.Event(eventType, subjectType, subject, map[string]any{
			"source": "claude_stream_json", "source_type": record.Type, "subtype": record.Subtype,
			"session_id": record.SessionID, "claude_code_version": record.ClaudeCodeVersion,
			"model": record.Model, "tools": record.Tools, "is_error": record.IsError,
			"usage": record.Usage, "rate_limit_info": record.RateLimitInfo,
			"total_cost_usd": record.CostUSD, "duration_ms": record.DurationMS,
		}, record.Timestamp)
		if err != nil {
			return nil, err
		}
		event.SourceSystem = "claude"
		event.SourceSequence = int64(line)
		event.SourceEventID = fmt.Sprintf("%s:%s:%s:%d", record.SessionID, record.Type, record.Subtype, line)
		events = append(events, event)
	}
	return events, scanner.Err()
}

func DecodeHook(payload HookPayload, adapterContext adapters.Context) (domain.Event, error) {
	eventType := map[string]string{
		"SessionStart": "session.started", "SessionEnd": "session.completed",
		"PreToolUse": "tool.started", "PostToolUse": "tool.completed",
		"PostToolUseFailure": "tool.failed", "Stop": "run.completed",
	}[payload.HookEventName]
	if eventType == "" {
		eventType = "claude.hook.observed"
	}
	event, err := adapterContext.Event(eventType, domain.ResourceSession, payload.SessionID, payload, time.Time{})
	event.SourceSystem = "claude"
	event.SourceEventID = payload.SessionID + ":" + payload.HookEventName + ":" + payload.ToolName
	return event, err
}

func mapStreamType(record streamRecord) (string, domain.ResourceType) {
	switch record.Type {
	case "system":
		return "session.started", domain.ResourceSession
	case "rate_limit_event":
		return "budget.rate_limit_observed", domain.ResourceBudget
	case "assistant":
		return "run.progressed", domain.ResourceRun
	case "result":
		if record.IsError || record.Error != "" || record.Subtype == "error" {
			return "run.failed", domain.ResourceRun
		}
		return "run.completed", domain.ResourceRun
	default:
		return "", ""
	}
}
