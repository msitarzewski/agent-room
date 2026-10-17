package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportDispatchDoesNotLoadDaemonConfigOrDatabase(t *testing.T) {
	previous := os.Args
	defer func() { os.Args = previous }()
	t.Setenv("AGENTROOM_DATABASE_URL", "invalid-database-never-contacted")
	os.Args = []string{"agentroomctl", "report", "--unexpected", "private-value"}
	err := run()
	if err == nil || !strings.Contains(err.Error(), "invalid report arguments") || strings.Contains(err.Error(), "private-value") {
		t.Fatalf("dispatch error=%v", err)
	}
	os.Args = []string{"agentroomctl", "report", "unapproved-body"}
	if err := run(); err == nil || !strings.Contains(err.Error(), "named metadata flags only") {
		t.Fatalf("positional input error=%v", err)
	}
}

func TestReportRetryDoesNotRequireNativeSessionOrCreateObservations(t *testing.T) {
	root := t.TempDir()
	token := filepath.Join(root, "token")
	if err := os.WriteFile(token, []byte("fixture-only-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_THREAD_ID", "")
	if err := report([]string{"--retry", "--project-id", "project", "--runtime", "codex", "--agent-id", "actor", "--instance-id", "instance", "--state-dir", filepath.Join(root, "state"), "--token-file", token, "--adapter-url", "http://127.0.0.1:9091"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "state", "report.json")); !os.IsNotExist(err) {
		t.Fatal("retry-only fabricated an observation")
	}
}
