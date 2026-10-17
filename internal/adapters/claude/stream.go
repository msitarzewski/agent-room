package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
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
	CostUSD           *float64        `json:"total_cost_usd"`
	DurationMS        *int64          `json:"duration_ms"`
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
	sessionID := adapterContext.SessionID
	for line := 1; scanner.Scan(); line++ {
		var record streamRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, fmt.Errorf("invalid claude stream-json record at line %d", line)
		}
		if record.SessionID != "" {
			sessionID = adapters.MetadataToken(record.SessionID, 512)
			if sessionID == "" {
				return nil, fmt.Errorf("invalid native claude session identifier")
			}
		}
		record.SessionID = sessionID
		eventType, subjectType := mapStreamType(record)
		if eventType == "" {
			continue
		}
		subject := sessionID
		if subjectType == domain.ResourceRun || subject == "" {
			subject = adapterContext.RunID
		}
		if adapters.MetadataToken(subject, 512) == "" {
			return nil, fmt.Errorf("claude record requires an explicit native session or run identity")
		}
		tools := []string{}
		for _, tool := range record.Tools {
			if token := adapters.MetadataToken(tool, 128); token != "" && len(tools) < 100 {
				tools = append(tools, token)
			}
		}
		var rateLimit struct {
			Status   string `json:"status"`
			ResetsAt *int64 `json:"resets_at,omitempty"`
		}
		_ = json.Unmarshal(record.RateLimitInfo, &rateLimit)
		if rateLimit.Status != "allowed" && rateLimit.Status != "allowed_warning" && rateLimit.Status != "rejected" {
			rateLimit.Status = "unknown"
		}
		if rateLimit.ResetsAt != nil && (*rateLimit.ResetsAt < 0 || *rateLimit.ResetsAt > 4102444800) {
			rateLimit.ResetsAt = nil
		}
		metadata := map[string]any{
			"source": "claude_stream_json", "source_type": record.Type, "subtype": adapters.MetadataToken(record.Subtype, 64),
			"session_id": sessionID, "agent_id": adapterContext.AgentID, "claude_code_version": adapters.MetadataToken(record.ClaudeCodeVersion, 64),
			"model": adapters.MetadataToken(record.Model, 128), "tools": tools, "is_error": record.IsError,
			"usage": adapters.UsageCounts(record.Usage), "rate_limit_info": rateLimit,
		}
		if record.CostUSD != nil && !math.IsNaN(*record.CostUSD) && !math.IsInf(*record.CostUSD, 0) && *record.CostUSD >= 0 && *record.CostUSD <= 1_000_000 {
			metadata["total_cost_usd"] = *record.CostUSD
		}
		if record.DurationMS != nil && *record.DurationMS >= 0 && *record.DurationMS <= 7*24*60*60*1000 {
			metadata["duration_ms"] = *record.DurationMS
		}
		event, err := adapterContext.Event(eventType, subjectType, subject, metadata, record.Timestamp)
		if err != nil {
			return nil, err
		}
		event.SourceSystem = "claude"
		event.SourceSequence, err = adapters.SourceSequence(adapterContext.SourceSequence, int64(line))
		if err != nil {
			return nil, err
		}
		event.SourceEventID = adapters.StableSourceID(adapterContext.NativeEventID, adapterContext.ProjectID, adapterContext.AgentID, sessionID, subject, record.Type, record.Subtype, fmt.Sprint(event.SourceSequence))
		events = append(events, event)
	}
	return events, scanner.Err()
}

func DecodeHook(payload HookPayload, adapterContext adapters.Context) (domain.Event, error) {
	if adapterContext.NativeEventID == "" || adapterContext.SourceSequence <= 0 || adapters.MetadataToken(payload.SessionID, 512) == "" {
		return domain.Event{}, fmt.Errorf("claude hook requires a native invocation ID, durable sequence, and session ID")
	}
	eventType := map[string]string{
		"SessionStart": "session.started", "SessionEnd": "session.completed",
		"PreToolUse": "tool.started", "PostToolUse": "tool.completed",
		"PostToolUseFailure": "tool.failed", "Stop": "run.completed",
	}[payload.HookEventName]
	if eventType == "" {
		eventType = "claude.hook.observed"
	}
	event, err := adapterContext.Event(eventType, domain.ResourceSession, payload.SessionID, map[string]any{
		"source": "claude_hook", "event_name": adapters.MetadataToken(payload.HookEventName, 64), "session_id": adapters.MetadataToken(payload.SessionID, 512),
		"agent_id": adapterContext.AgentID, "tool_name": adapters.MetadataToken(payload.ToolName, 128),
	}, time.Time{})
	event.SourceSystem = "claude"
	event.SourceEventID = adapters.StableSourceID(adapterContext.ProjectID, payload.SessionID, adapterContext.NativeEventID)
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
