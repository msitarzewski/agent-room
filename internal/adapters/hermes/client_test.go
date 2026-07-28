package hermes

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
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
