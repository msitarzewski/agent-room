package hermes

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/msitarzewski/agent-room/internal/adapters"
	"github.com/msitarzewski/agent-room/internal/domain"
)

func TestClientExactSessionContracts(t *testing.T) {
	fixture, err := os.ReadFile("testdata/sessions-v2026.7.20.json")
	if err != nil {
		t.Fatal(err)
	}
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer private-token" {
			t.Errorf("authorization=%q", got)
		}
		calls = append(calls, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/sessions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(fixture)
		case r.Method == http.MethodGet && r.URL.Path == "/api/sessions/20260727_120000_a1b2c3d4":
			_, _ = w.Write([]byte(`{"object":"hermes.session","session":{"id":"20260727_120000_a1b2c3d4"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/sessions/20260727_120000_a1b2c3d4/chat":
			body, _ := io.ReadAll(r.Body)
			var value map[string]any
			if err := json.Unmarshal(body, &value); err != nil {
				t.Fatal(err)
			}
			if len(value) != 1 || value["input"] != "status please" {
				t.Errorf("chat body=%s", body)
			}
			_, _ = w.Write([]byte(`{"object":"hermes.session.chat.completion","session_id":"20260727_120000_a1b2c3d4","message":{"role":"assistant","content":"ready"}}`))
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "private-token")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Sessions(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Session(context.Background(), "20260727_120000_a1b2c3d4"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Chat(context.Background(), "20260727_120000_a1b2c3d4", "status please"); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(calls, "\n")
	want := strings.Join([]string{
		"GET /api/sessions",
		"GET /api/sessions/20260727_120000_a1b2c3d4",
		"POST /api/sessions/20260727_120000_a1b2c3d4/chat",
	}, "\n")
	if got != want {
		t.Fatalf("calls:\n%s\nwant:\n%s", got, want)
	}
}

func TestHermesHookUsesOnlyMetadataAndDurableInvocationIdentity(t *testing.T) {
	context := adapters.Context{ProjectID: "p", ActorID: "actor", AgentID: "actor", NativeEventID: "invocation-1", SourceSequence: 100}
	payload := HookPayload{EventName: "post_tool_call", SessionID: "native-session", TurnID: "actual-turn", ToolName: "Read", APICallCount: 1, Data: json.RawMessage(`{"prompt":"fixture-secret-prompt","tool_input":"fixture-secret-input","tool_response":"fixture-secret-response","token":"fixture-secret-token"}`)}
	event, err := DecodeHook(payload, context)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(event.Payload), "fixture-secret") || strings.Contains(string(event.Payload), `"data"`) || event.SourceSequence != 100 {
		t.Fatal("private data or API-call sequencing reached normalized event")
	}
	context.NativeEventID = "invocation-2"
	context.SourceSequence = 101
	next, err := DecodeHook(payload, context)
	if err != nil || next.SourceEventID == event.SourceEventID {
		t.Fatal("different invocation IDs collided")
	}
	context.SourceSequence = 0
	if _, err := DecodeHook(payload, context); err == nil {
		t.Fatal("unsequenced hook accepted")
	}
}

func TestHermesHookKeepsNativeLifecycleSubjectsSeparateFromTaskMetadata(t *testing.T) {
	context := adapters.Context{ProjectID: "p", ActorID: "actor", NativeEventID: "invocation-1", SourceSequence: 100}
	for _, test := range []struct {
		name, eventName, status, eventType, subjectID string
		subjectType                                   domain.ResourceType
	}{
		{"session start", "session_start", "", "session.started", "native-session", domain.ResourceSession},
		{"session end", "session_end", "failed", "session.completed", "native-session", domain.ResourceSession},
		{"tool", "post_tool_call", "", "tool.completed", "native-turn", domain.ResourceRun},
		{"turn", "pre_llm_call", "", "turn.started", "native-turn", domain.ResourceRun},
		{"subagent", "subagent_end", "", "run.completed", "native-turn", domain.ResourceRun},
		{"failed run", "subagent_end", "failed", "run.failed", "native-turn", domain.ResourceRun},
		{"approval context", "approval_requested", "", "approval.requested", "native-session", domain.ResourceSession},
	} {
		t.Run(test.name, func(t *testing.T) {
			event, err := DecodeHook(HookPayload{EventName: test.eventName, SessionID: "native-session", TurnID: "native-turn", TaskID: "associated-task", Status: test.status}, context)
			if err != nil || event.Type != test.eventType || event.SubjectType != test.subjectType || event.SubjectID != test.subjectID {
				t.Fatalf("incorrect native lifecycle subject: event=%+v err=%v", event, err)
			}
		})
	}
	for _, turnID := range []string{"", "invalid private body"} {
		if _, err := DecodeHook(HookPayload{EventName: "subagent_start", SessionID: "native-session", TurnID: turnID, TaskID: "associated-task"}, context); err == nil {
			t.Fatal("missing or invalid native run identity was replaced with task metadata")
		}
	}
	context.RunID = "explicit-native-run"
	event, err := DecodeHook(HookPayload{EventName: "subagent_start", SessionID: "native-session", TaskID: "associated-task"}, context)
	if err != nil || event.SubjectType != domain.ResourceRun || event.SubjectID != context.RunID {
		t.Fatal("explicit caller run identity was not preserved")
	}
}

func TestHermesRefusesCredentialsAndRedirectDowngrade(t *testing.T) {
	for _, target := range []string{"http://outside.invalid", "https://user:private@outside.invalid", "https://outside.invalid?token=private", "https://outside.invalid#private"} {
		if _, err := NewClient(target, "fixture-token"); err == nil {
			t.Fatal("unsafe Hermes transport accepted")
		}
	}
	forwarded := false
	outside := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { forwarded = true; w.WriteHeader(200) }))
	defer outside.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, outside.URL, http.StatusTemporaryRedirect)
	}))
	defer secure.Close()
	client, err := NewClient(secure.URL, "fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(secure.Certificate())
	client.http.Transport.(*http.Transport).TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	if _, err := client.Sessions(context.Background()); err == nil || strings.Contains(err.Error(), "fixture-token") || forwarded {
		t.Fatal("redirect forwarded private bearer or leaked transport details")
	}
}

func TestClientRejectsInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "private-token")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Sessions(context.Background()); err == nil {
		t.Fatal("expected invalid JSON rejection")
	}
}

func TestClientOperationalErrorsOmitSessionAndResponseDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "fixture-private-response", http.StatusForbidden)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "fixture-private-token")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Session(context.Background(), "fixture-private-session")
	if err == nil || err.Error() != "hermes API HTTP 403" {
		t.Fatalf("operational error contains unnecessary private context: %v", err)
	}
}
