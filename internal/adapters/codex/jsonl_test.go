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
