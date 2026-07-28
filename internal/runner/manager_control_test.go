//go:build unix

package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManagedProcessControlsAndConfinement(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "work")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	manager, err := New(root, map[string]Runtime{"test": {Executable: "/bin/sh", BaseArgs: []string{"-c"}}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(Spec{RunID: "run-1", Runtime: "test", Workspace: "work", Input: "sleep 30", TimeLimit: time.Minute}); err != nil {
		t.Fatal(err)
	}
	if !manager.Supports("run-1", "pause") || manager.Supports("run-1", "message") {
		t.Fatal("live capabilities do not match transport")
	}
	if err := manager.Execute(context.Background(), "run-1", "pause", ""); err != nil {
		t.Fatal(err)
	}
	if err := manager.Execute(context.Background(), "run-1", "resume", ""); err != nil {
		t.Fatal(err)
	}
	if err := manager.Execute(context.Background(), "run-1", "cancel", ""); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(Spec{RunID: "escape", Runtime: "test", Workspace: "../", Input: "true", TimeLimit: time.Second}); err == nil {
		t.Fatal("workspace escape accepted")
	}
}

func TestManagerRejectsInvalidConfigurationAndRunSpecs(t *testing.T) {
	if _, err := New("/definitely/missing", nil, 1); err == nil {
		t.Fatal("missing workspace root accepted")
	}
	root := t.TempDir()
	if _, err := New(root, nil, 0); err == nil {
		t.Fatal("nonpositive concurrency accepted")
	}
	if _, err := New(root, map[string]Runtime{"bad": {Executable: "relative"}}, 1); err == nil {
		t.Fatal("relative runtime executable accepted")
	}
	workspace := filepath.Join(root, "work")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	manager, err := New(root, map[string]Runtime{"test": {Executable: "/bin/sh", BaseArgs: []string{"-c"}}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	for name, spec := range map[string]Spec{
		"unknown runtime":    {RunID: "one", Runtime: "missing", Workspace: "work", Input: "true", TimeLimit: time.Second},
		"absolute workspace": {RunID: "one", Runtime: "test", Workspace: workspace, Input: "true", TimeLimit: time.Second},
		"missing run id":     {Runtime: "test", Workspace: "work", Input: "true", TimeLimit: time.Second},
		"missing deadline":   {RunID: "one", Runtime: "test", Workspace: "work", Input: "true"},
	} {
		if err := manager.Start(spec); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if err := manager.Execute(context.Background(), "missing", "cancel", ""); err != nil {
		t.Fatalf("idempotent cancel of absent process failed: %v", err)
	}
	if err := manager.Execute(context.Background(), "missing", "pause", ""); err == nil {
		t.Fatal("pause of absent process succeeded")
	}
}

func TestManagerEnforcesConcurrencyAndDuplicateRunIDs(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "work"), 0o700); err != nil {
		t.Fatal(err)
	}
	manager, err := New(root, map[string]Runtime{"test": {Executable: "/bin/sh", BaseArgs: []string{"-c"}}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	spec := Spec{RunID: "active", Runtime: "test", Workspace: "work", Input: "sleep 30", TimeLimit: time.Minute}
	if err := manager.Start(spec); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(spec); err == nil {
		t.Fatal("duplicate run ID accepted")
	}
	if err := manager.Start(Spec{RunID: "second", Runtime: "test", Workspace: "work", Input: "true", TimeLimit: time.Second}); err == nil {
		t.Fatal("run beyond concurrency limit accepted")
	}
	if err := manager.Execute(context.Background(), "active", "redirect", "elsewhere"); err == nil {
		t.Fatal("unsupported live transport action succeeded")
	}
	if err := manager.Execute(context.Background(), "active", "cancel", ""); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for manager.Supports("active", "cancel") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if manager.Supports("active", "cancel") {
		t.Fatal("cancelled process remained active")
	}
	if err := manager.Start(Spec{RunID: "after", Runtime: "test", Workspace: "work", Input: "true", TimeLimit: time.Second}); err != nil {
		t.Fatalf("concurrency slot was not released: %v", err)
	}
}

func TestManagerStartFailureTimeoutAndSymlinkConfinement(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "work"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "escape-link")); err != nil {
		t.Fatal(err)
	}
	manager, err := New(root, map[string]Runtime{
		"missing": {Executable: "/definitely/missing"},
		"shell":   {Executable: "/bin/sh", BaseArgs: []string{"-c"}},
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(Spec{RunID: "bad-exec", Runtime: "missing", Workspace: "work", TimeLimit: time.Second}); err == nil {
		t.Fatal("missing executable start succeeded")
	}
	if manager.Supports("bad-exec", "cancel") {
		t.Fatal("failed start retained process registration")
	}
	if err := manager.Start(Spec{RunID: "link", Runtime: "shell", Workspace: "escape-link", Input: "true", TimeLimit: time.Second}); err == nil {
		t.Fatal("workspace symlink escape accepted")
	}
	if err := manager.Start(Spec{RunID: "timeout", Runtime: "shell", Workspace: "work", Input: "sleep 30", TimeLimit: 20 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for manager.Supports("timeout", "cancel") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if manager.Supports("timeout", "cancel") {
		t.Fatal("deadline did not terminate managed process")
	}
}
