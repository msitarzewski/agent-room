package codex

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/msitarzewski/agent-room/internal/adapters"
)

func TestCodex0145Fixture(t *testing.T) {
	fixture, err := os.Open("testdata/codex-0.145.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	next := 0
	events, err := DecodeJSONL(fixture, adapters.Context{ProjectID: "p", ActorID: "codex", RunID: "run", SessionID: "session", Now: func() time.Time { return time.Unix(1, 0) }, NewID: func() string { next++; return string(rune('a' + next)) }})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatalf("events=%d", len(events))
	}
	if events[0].Type != "session.started" || events[3].Type != "run.completed" {
		t.Fatalf("types=%s,%s", events[0].Type, events[3].Type)
	}
	var payload map[string]any
	if err := json.Unmarshal(events[2].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(events[2].Payload), "sanitized response text") {
		t.Fatal("agent message text must not be copied into coordination telemetry")
	}
}

func TestCodexDropsPrivateMessagesErrorsAndUnknownUsage(t *testing.T) {
	input := `{"type":"thread.started","thread_id":"native-thread"}
{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"fixture-secret-prompt","command":"fixture-secret-command"},"usage":{"input_tokens":9,"private":"fixture-secret-usage"},"error":{"message":"fixture-secret-error","credential":"fixture-secret-token"}}
`
	context := adapters.Context{ProjectID: "p", ActorID: "actor", AgentID: "actor", RunID: "actual-run", SourceSequence: 100}
	events, err := DecodeJSONL(strings.NewReader(input), context)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if strings.Contains(string(event.Payload), "fixture-secret") {
			t.Fatal("private native content reached telemetry")
		}
	}
	if events[1].SourceSequence != 102 || !strings.Contains(string(events[1].Payload), `"input_tokens":9`) || !strings.Contains(string(events[1].Payload), `"thread_id":"native-thread"`) {
		t.Fatalf("normalized metadata=%+v", events[1])
	}
	context.RunID = "different-actual-run"
	next, err := DecodeJSONL(strings.NewReader(input), context)
	if err != nil || next[1].SourceEventID == events[1].SourceEventID {
		t.Fatal("different runs collided on native line number")
	}
	if _, err := DecodeJSONL(strings.NewReader(`{"type":"turn.started"}`), adapters.Context{ProjectID: "p", ActorID: "actor"}); err == nil {
		t.Fatal("missing native run ID became fabricated identity")
	}
}
