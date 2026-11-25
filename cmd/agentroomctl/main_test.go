package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/msitarzewski/agent-room/internal/auth"
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

func TestSummarizeServiceTokenExpiry(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	soon := now.Add(5 * 24 * time.Hour)
	later := now.Add(40 * 24 * time.Hour)
	revoked := now.Add(-24 * time.Hour)

	summary := auth.SummarizeServiceTokenExpiry([]auth.ServiceTokenMetadata{
		{ID: "expired", Name: "Expired", ActorID: "a1", ExpiresAt: &past},
		{ID: "expiring", Name: "Expiring", ActorID: "a2", ExpiresAt: &soon},
		{ID: "healthy", Name: "Healthy", ActorID: "a3", ExpiresAt: &later},
		{ID: "revoked", Name: "Revoked", ActorID: "a4", ExpiresAt: &past, RevokedAt: &revoked},
	}, now, 14*24*time.Hour)

	if summary.Active != 2 || summary.Revoked != 1 || len(summary.Expired) != 1 || len(summary.Expiring) != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if summary.Expired[0].ID != "expired" || summary.Expiring[0].ID != "expiring" {
		t.Fatalf("unexpected token classification: %+v", summary)
	}
}
