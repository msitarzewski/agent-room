package adapters

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func reportFixture(t *testing.T, endpoint string) ReportOptions {
	t.Helper()
	root := t.TempDir()
	token := filepath.Join(root, "token")
	if err := os.WriteFile(token, []byte("fixture-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	return ReportOptions{ProjectID: "project", Runtime: "codex", AgentID: "actual-agent", InstanceID: "workstation", DisplayName: "Actual coding agent", SessionID: "native-session", StateDir: filepath.Join(root, "state"), TokenFile: token, AdapterURL: endpoint,
		Now: func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) }}
}

func TestReporterPersistsImmutableRetryBeforeDelivery(t *testing.T) {
	var bodies []string
	var mu sync.Mutex
	uncertain := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(body))
		fail := uncertain
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer fixture-token" || r.URL.Path != "/api/v1/ingest" || r.URL.Query().Get("project_id") != "project" {
			t.Error("invalid private publication contract")
		}
		if fail {
			connection, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = connection.Close()
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	options := reportFixture(t, server.URL)
	result, err := Report(options)
	if err == nil || result.Pending != 1 {
		t.Fatalf("uncertain result=%+v err=%v", result, err)
	}
	if strings.Contains(err.Error(), "fixture-token") || strings.Contains(err.Error(), server.URL) {
		t.Fatal("transport error leaked private details")
	}
	data, err := os.ReadFile(filepath.Join(options.StateDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var state reportState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	firstBody := bodies[0]
	mu.Unlock()
	if len(state.Queue) != 1 || state.Queue[0].Body != firstBody {
		t.Fatal("HTTP body was not durably persisted before delivery")
	}
	var envelope struct {
		Events []struct {
			Type     string         `json:"type"`
			Sequence int64          `json:"source_sequence"`
			Payload  map[string]any `json:"payload"`
		} `json:"events"`
	}
	if err := json.Unmarshal([]byte(firstBody), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Events) != 2 || envelope.Events[0].Type != "agent.observed" || envelope.Events[1].Type != "session.observed" || envelope.Events[0].Payload["agent_id"] != options.AgentID || envelope.Events[1].Sequence <= envelope.Events[0].Sequence {
		t.Fatalf("metadata=%+v", envelope)
	}
	mu.Lock()
	uncertain = false
	mu.Unlock()
	options.RetryOnly = true
	options.SessionID = ""
	options.DisplayName = ""
	options.Now = func() time.Time { return state.Queue[0].ObservedAt.Add(time.Hour) }
	result, err = Report(options)
	mu.Lock()
	defer mu.Unlock()
	if err != nil || result.Delivered != 1 || result.Pending != 0 || len(bodies) != 2 || bodies[0] != bodies[1] {
		t.Fatalf("retry result=%+v err=%v", result, err)
	}
}

func TestReporterUsesOwnThreadEnvironmentAndNeverInventsRun(t *testing.T) {
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { body, _ = io.ReadAll(r.Body); w.WriteHeader(202) }))
	defer server.Close()
	t.Setenv("CODEX_THREAD_ID", "actual-child-thread")
	t.Setenv("CODEX_SESSION_ID", "different-parent-conversation")
	options := reportFixture(t, server.URL)
	options.SessionID = ""
	if _, err := Report(options); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "actual-child-thread") || strings.Contains(string(body), "different-parent-conversation") || strings.Contains(string(body), "run.observed") {
		t.Fatal("reporter inferred unrelated native identity")
	}
	options.RunStatus = "running"
	if _, err := Report(options); err == nil {
		t.Fatal("run status accepted without an explicit native run ID")
	}
}

func TestReporterRejectsScopeRemappingAndUnsafeTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(202) }))
	defer server.Close()
	options := reportFixture(t, server.URL)
	if _, err := Report(options); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*ReportOptions){func(o *ReportOptions) { o.ProjectID = "different" }, func(o *ReportOptions) { o.AgentID = "different" }, func(o *ReportOptions) { o.InstanceID = "different" }, func(o *ReportOptions) { o.Runtime = "claude" }, func(o *ReportOptions) { o.AdapterURL = server.URL + "/api/v1/ingest?token=secret" }, func(o *ReportOptions) { o.AdapterURL = "http://outside.invalid" }, func(o *ReportOptions) { o.AdapterURL = "https://user:password@outside.invalid" }} {
		changed := options
		mutate(&changed)
		if _, err := Report(changed); err == nil {
			t.Fatal("scope remapping or unsafe endpoint accepted")
		}
	}
	if err := os.Chmod(options.TokenFile, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Report(options); err == nil {
		t.Fatal("world-readable bearer token accepted")
	}
}

func TestReporterDoesNotForwardBearerAcrossRedirect(t *testing.T) {
	forwarded := false
	outside := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { forwarded = true; w.WriteHeader(202) }))
	defer outside.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, outside.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	result, err := Report(reportFixture(t, server.URL))
	if err == nil || result.Pending != 1 || forwarded {
		t.Fatalf("redirect result=%+v err=%v forwarded=%v", result, err, forwarded)
	}
}

func TestReporterQuarantinesOldOrRejectedBodiesWithoutRetimestamps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	defer server.Close()
	options := reportFixture(t, server.URL)
	if _, err := Report(options); err == nil {
		t.Fatal("unavailable source acknowledged")
	}
	before, _ := os.ReadFile(filepath.Join(options.StateDir, "report.json"))
	var original reportState
	_ = json.Unmarshal(before, &original)
	options.RetryOnly = true
	options.Now = func() time.Time { return original.Queue[0].ObservedAt.Add(31 * 24 * time.Hour) }
	result, err := Report(options)
	if err == nil || result.Pending != 0 || result.Quarantined != 1 {
		t.Fatalf("old result=%+v err=%v", result, err)
	}
	after, _ := os.ReadFile(filepath.Join(options.StateDir, "report.json"))
	var retained reportState
	_ = json.Unmarshal(after, &retained)
	if retained.Queue[0].Body != original.Queue[0].Body || !retained.Queue[0].ObservedAt.Equal(original.Queue[0].ObservedAt) {
		t.Fatal("quarantine changed the original observation")
	}
}

func TestReporterExplicitRunLinksAndPrivateSingleWriterState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"run_id":"actual-native-run"`) || !strings.Contains(string(body), `"task_id":"canonical-task"`) {
			t.Error("explicit run linkage missing")
		}
		w.WriteHeader(202)
	}))
	defer server.Close()
	options := reportFixture(t, server.URL)
	options.RunID = "actual-native-run"
	options.RunStatus = "running"
	options.TaskID = "canonical-task"
	options.Summary = "Bounded source task"
	if _, err := Report(options); err != nil {
		t.Fatal(err)
	}
	lock, err := lockReportDirectory(options.StateDir)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if _, err := Report(options); err == nil {
		t.Fatal("second writer acquired report state")
	}
	info, err := os.Stat(filepath.Join(options.StateDir, "report.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("report state is not private")
	}
}
