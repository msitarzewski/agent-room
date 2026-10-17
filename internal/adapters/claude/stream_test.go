package claude

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/msitarzewski/agent-room/internal/adapters"
	"github.com/msitarzewski/agent-room/internal/domain"
)

func TestClaude21220Fixture(t *testing.T) {
	fixture, err := os.Open("testdata/claude-2.1.220.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	events, err := DecodeStreamJSON(fixture, adapters.Context{ProjectID: "p", ActorID: "claude", RunID: "run"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatalf("events=%d", len(events))
	}
	if events[0].Type != "session.started" || events[1].Type != "budget.rate_limit_observed" || events[1].SubjectType != domain.ResourceBudget || events[3].Type != "run.completed" {
		t.Fatalf("events=%+v", events)
	}
	for _, event := range events {
		if strings.Contains(string(event.Payload), "sanitized") {
			t.Fatal("prompt/result-like content must not be copied into telemetry")
		}
	}
}

func TestClaudeMetadataNeverIncludesToolBodiesOrTranscriptPaths(t *testing.T) {
	context := adapters.Context{ProjectID: "p", ActorID: "actor", AgentID: "actor", RunID: "actual-run", NativeEventID: "invocation-1", SourceSequence: 200}
	payload := HookPayload{HookEventName: "PostToolUse", SessionID: "native-session", ToolName: "Read", CWD: "fixture-secret-cwd", TranscriptPath: "fixture-secret-transcript", ToolInput: json.RawMessage(`{"token":"fixture-secret-input"}`), ToolResponse: json.RawMessage(`{"body":"fixture-secret-response"}`)}
	event, err := DecodeHook(payload, context)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(event.Payload), "fixture-secret") || strings.Contains(string(event.Payload), "tool_input") || strings.Contains(string(event.Payload), "transcript_path") {
		t.Fatal("private hook content reached telemetry")
	}
	context.NativeEventID = "invocation-2"
	context.SourceSequence++
	next, err := DecodeHook(payload, context)
	if err != nil || next.SourceEventID == event.SourceEventID || next.SourceSequence <= event.SourceSequence {
		t.Fatal("repeated tool invocation identity collided")
	}
	context.NativeEventID = ""
	if _, err := DecodeHook(payload, context); err == nil {
		t.Fatal("unidentified hook invocation accepted")
	}
	input := `{"type":"result","subtype":"success","session_id":"native-session","result":"fixture-secret-result","message":{"prompt":"fixture-secret-message"},"usage":{"input_tokens":3,"token":"fixture-secret-usage"},"rate_limit_info":{"status":"allowed","private":"fixture-secret-rate"}}`
	events, err := DecodeStreamJSON(strings.NewReader(input), context)
	if err != nil || len(events) != 1 {
		t.Fatalf("events=%d err=%v", len(events), err)
	}
	if strings.Contains(string(events[0].Payload), "fixture-secret") || events[0].SubjectID != "actual-run" {
		t.Fatal("private stream metadata or incorrect run identity")
	}
}

func TestClaudeInvalidOrMissingMetricsRemainUnknown(t *testing.T) {
	input := `{"type":"result","subtype":"success","session_id":"native-session","total_cost_usd":-1,"duration_ms":-20,"usage":{"input_tokens":null},"rate_limit_info":{"status":"allowed","resets_at":-1}}`
	events, err := DecodeStreamJSON(strings.NewReader(input), adapters.Context{ProjectID: "p", ActorID: "actor", RunID: "actual-run"})
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(events[0].Payload, &metadata); err != nil {
		t.Fatal(err)
	}
	if _, ok := metadata["total_cost_usd"]; ok {
		t.Fatal("invalid native cost became a fabricated zero")
	}
	if _, ok := metadata["duration_ms"]; ok {
		t.Fatal("invalid native duration became a fabricated zero")
	}
	if strings.Contains(string(metadata["usage"]), "input_tokens") || strings.Contains(string(metadata["rate_limit_info"]), "resets_at") {
		t.Fatal("unknown native counters became fabricated measurements")
	}
}

func TestClaudeResultIsError(t *testing.T) {
	input := `{"type":"result","subtype":"success","is_error":true,"session_id":"s"}` + "\n"
	events, err := DecodeStreamJSON(strings.NewReader(input), adapters.Context{ProjectID: "p", ActorID: "claude", RunID: "run"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "run.failed" {
		t.Fatalf("events=%+v", events)
	}
}
