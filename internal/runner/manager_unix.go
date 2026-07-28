//go:build unix

package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Runtime struct {
	Executable string
	BaseArgs   []string
}

type Spec struct {
	RunID     string
	Runtime   string
	Workspace string
	Input     string
	TimeLimit time.Duration
	Stdout    io.Writer
	Stderr    io.Writer
}

type process struct {
	command *exec.Cmd
	cancel  context.CancelFunc
	done    chan struct{}
}

type Manager struct {
	mu            sync.Mutex
	workspaceRoot string
	runtimes      map[string]Runtime
	processes     map[string]*process
	maxConcurrent int
}

func New(workspaceRoot string, runtimes map[string]Runtime, maxConcurrent int) (*Manager, error) {
	root, err := filepath.EvalSymlinks(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}
	if maxConcurrent <= 0 {
		return nil, errors.New("maximum concurrency must be positive")
	}
	for name, runtime := range runtimes {
		if name == "" || runtime.Executable == "" || !filepath.IsAbs(runtime.Executable) {
			return nil, errors.New("runtime names and absolute executable paths are required")
		}
	}
	return &Manager{workspaceRoot: root, runtimes: runtimes, processes: make(map[string]*process), maxConcurrent: maxConcurrent}, nil
}

func (m *Manager) Start(spec Spec) error {
	runtime, ok := m.runtimes[spec.Runtime]
	if !ok {
		return errors.New("runtime is not allowlisted")
	}
	workspace, err := m.confinedWorkspace(spec.Workspace)
	if err != nil {
		return err
	}
	if spec.RunID == "" || spec.TimeLimit <= 0 {
		return errors.New("run ID and positive time limit are required")
	}
	m.mu.Lock()
	if len(m.processes) >= m.maxConcurrent {
		m.mu.Unlock()
		return errors.New("managed-run concurrency limit reached")
	}
	if _, exists := m.processes[spec.RunID]; exists {
		m.mu.Unlock()
		return errors.New("run is already managed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), spec.TimeLimit)
	args := append(append([]string(nil), runtime.BaseArgs...), spec.Input)
	command := exec.CommandContext(ctx, runtime.Executable, args...)
	command.Dir, command.Stdout, command.Stderr = workspace, spec.Stdout, spec.Stderr
	command.Env = safeEnvironment()
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	entry := &process{command: command, cancel: cancel, done: make(chan struct{})}
	m.processes[spec.RunID] = entry
	m.mu.Unlock()
	if err := command.Start(); err != nil {
		cancel()
		m.mu.Lock()
		delete(m.processes, spec.RunID)
		m.mu.Unlock()
		return err
	}
	go func() {
		_ = command.Wait()
		close(entry.done)
		if closer, ok := spec.Stdout.(io.Closer); ok {
			_ = closer.Close()
		}
		if closer, ok := spec.Stderr.(io.Closer); ok && spec.Stderr != spec.Stdout {
			_ = closer.Close()
		}
		cancel()
		m.mu.Lock()
		delete(m.processes, spec.RunID)
		m.mu.Unlock()
	}()
	return nil
}

func (m *Manager) Supports(runID, action string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, exists := m.processes[runID]
	if !exists {
		return false
	}
	return action == "pause" || action == "resume" || action == "cancel"
}

func (m *Manager) Execute(_ context.Context, runID, action, _ string) error {
	m.mu.Lock()
	entry := m.processes[runID]
	m.mu.Unlock()
	if entry == nil || entry.command.Process == nil {
		if action == "cancel" {
			return nil
		}
		return errors.New("managed run is not active")
	}
	pgid := -entry.command.Process.Pid
	switch action {
	case "pause":
		return syscall.Kill(pgid, syscall.SIGSTOP)
	case "resume":
		return syscall.Kill(pgid, syscall.SIGCONT)
	case "cancel":
		if err := syscall.Kill(pgid, syscall.SIGTERM); err != nil {
			if errors.Is(err, syscall.ESRCH) {
				return nil
			}
			return err
		}
		go func() {
			timer := time.NewTimer(5 * time.Second)
			defer timer.Stop()
			select {
			case <-entry.done:
				return
			case <-timer.C:
				_ = syscall.Kill(pgid, syscall.SIGKILL)
			}
		}()
		return nil
	default:
		return errors.New("runtime transport does not support this action")
	}
}

func safeEnvironment() []string {
	const safePath = "/usr/local/bin:/usr/bin:/bin"
	environment := []string{"PATH=" + safePath}
	for _, key := range []string{"HOME", "USER", "LOGNAME", "LANG", "LC_ALL", "TERM", "TMPDIR"} {
		if value, ok := os.LookupEnv(key); ok && !strings.ContainsRune(value, '\x00') {
			environment = append(environment, key+"="+value)
		}
	}
	return environment
}

func (m *Manager) confinedWorkspace(relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return "", errors.New("workspace must be a relative path")
	}
	candidate, err := filepath.EvalSymlinks(filepath.Join(m.workspaceRoot, filepath.Clean(relative)))
	if err != nil {
		return "", err
	}
	prefix := m.workspaceRoot + string(filepath.Separator)
	if candidate != m.workspaceRoot && !strings.HasPrefix(candidate, prefix) {
		return "", errors.New("workspace escapes configured root")
	}
	return candidate, nil
}
