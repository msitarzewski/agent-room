//go:build unix

package security_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/msitarzewski/agent-room/internal/runner"
)

func TestManagedRuntimeDoesNotInheritDaemonCredentialEnvironment(t *testing.T) {
	t.Setenv("AGENTROOM_DATABASE_URL_FILE", "/run/credentials/agentroom.service/database-url")
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(root, "child-environment")
	output, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := runner.New(root, map[string]runner.Runtime{
		"security-test": {Executable: "/bin/sh", BaseArgs: []string{"-c"}},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(runner.Spec{
		RunID: "environment-boundary", Runtime: "security-test", Workspace: "workspace",
		Input: "env", TimeLimit: 5 * time.Second, Stdout: output, Stderr: output,
	}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for manager.Supports("environment-boundary", "cancel") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if manager.Supports("environment-boundary", "cancel") {
		t.Fatal("managed runtime did not exit")
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "AGENTROOM_DATABASE_URL_FILE=") {
		t.Fatal("managed runtime inherited the daemon credential-file location")
	}
}
