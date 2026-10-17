package adapters

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/msitarzewski/agent-room/internal/domain"
)

func TestContextBuildsStableNormalizedEvents(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 7, 27, 12, 0, 0, 0, time.FixedZone("source", 3600))
	context := Context{
		ProjectID: "project", ActorID: "agent", SourceSystem: "source",
		NativeEventID: "native", SourceSequence: 7,
		Now: func() time.Time { return at }, NewID: func() string { return "event-id" },
	}
	event, err := context.Event("run.started", domain.ResourceRun, "run", map[string]string{"status": "running"}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if event.ID != "event-id" || event.OccurredAt.Location() != time.UTC ||
		event.SourceEventID != "native" || event.SourceSequence != 7 || !json.Valid(event.Payload) {
		t.Fatalf("event=%+v", event)
	}
	context.NativeEventID = ""
	event, err = context.Event("run.started", domain.ResourceRun, "run", "payload", at)
	if err != nil || event.SourceEventID == "" ||
		event.SourceEventID != StableSourceID("source", "run.started", "run", `"payload"`) {
		t.Fatalf("stable event=%+v err=%v", event, err)
	}
}

func TestContextRejectsMissingScopeAndEncodingFailures(t *testing.T) {
	t.Parallel()
	for _, context := range []Context{{ActorID: "agent"}, {ProjectID: "project"}} {
		if _, err := context.Event("run.started", domain.ResourceRun, "run", nil, time.Time{}); err == nil {
			t.Fatalf("missing adapter scope accepted: %+v", context)
		}
	}
	context := Context{ProjectID: "project", ActorID: "agent"}
	if _, err := context.Event("run.started", domain.ResourceRun, "run", func() {}, time.Time{}); err == nil {
		t.Fatal("unencodable payload accepted")
	}
	first := StableSourceID("a", "b")
	if first == "" || first != StableSourceID("a", "b") || first == StableSourceID("ab") {
		t.Fatal("stable source IDs are not deterministic and boundary-safe")
	}
	event, err := context.Event("run.started", domain.ResourceRun, "run", nil, time.Time{})
	if err != nil || event.ID == "" || event.OccurredAt.IsZero() {
		t.Fatalf("default event=%+v err=%v", event, err)
	}
}

func TestUsageMetadataAllowsOnlyValidatedCounters(t *testing.T) {
	counts := UsageCounts(json.RawMessage(`{"input_tokens":12,"output_tokens":-1,"cache_read_input_tokens":null,"cached_input_tokens":1.5,"total_tokens":1000000000001,"prompt":"fixture-secret","extra":{"token":"fixture-secret"}}`))
	if len(counts) != 1 || counts["input_tokens"] != 12 {
		t.Fatalf("usage=%+v", counts)
	}
	if MetadataToken("tool with private input", 64) != "" || MetadataToken("Read", 2) != "" || MetadataToken("Read", 64) != "Read" {
		t.Fatal("invalid metadata token validation")
	}
	if _, err := SourceSequence(-1, 1); err == nil {
		t.Fatal("negative base sequence accepted")
	}
	if _, err := SourceSequence(1, 0); err == nil {
		t.Fatal("nonpositive sequence offset accepted")
	}
}
