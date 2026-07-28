package claude

import (
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
