package adapters

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/msitarzewski/agent-room/internal/domain"
)

// ReportOptions contains explicit metadata, never a transcript or tool body.
type ReportOptions struct {
	ProjectID, Runtime, AgentID, InstanceID, DisplayName string
	SessionID, AgentStatus, SessionStatus                string
	RunID, RunStatus, TaskID, Summary                    string
	StateDir, TokenFile, AdapterURL                      string
	RetryOnly                                            bool
	Timeout                                              time.Duration
	Now                                                  func() time.Time
}

type ReportResult struct {
	Delivered   int `json:"delivered"`
	Pending     int `json:"pending"`
	Quarantined int `json:"quarantined"`
}

type reportIdentity struct {
	ProjectID, Runtime, AgentID, InstanceID, Endpoint string
}

type queuedReport struct {
	Body        string    `json:"body"`
	ObservedAt  time.Time `json:"observed_at"`
	Quarantined string    `json:"quarantined,omitempty"`
}

type reportState struct {
	Schema   int            `json:"schema"`
	Identity reportIdentity `json:"identity"`
	Sequence int64          `json:"sequence"`
	Queue    []queuedReport `json:"queue"`
}

const maxReportStateBytes = 8 << 20

func validateReport(options *ReportOptions) (string, error) {
	if options.Runtime != "codex" && options.Runtime != "claude" && options.Runtime != "hermes" {
		return "", errors.New("report runtime must be codex, claude, or hermes")
	}
	for _, value := range []string{options.ProjectID, options.AgentID, options.InstanceID} {
		if value == "" || len(value) > 512 || strings.ContainsAny(value, "\r\n\x00") {
			return "", errors.New("report requires bounded project, actor, and instance identifiers")
		}
	}
	if !filepath.IsAbs(options.StateDir) || options.TokenFile == "" {
		return "", errors.New("report requires an absolute private state directory and token file")
	}
	endpoint, err := url.Parse(options.AdapterURL)
	if err != nil || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return "", errors.New("report adapter URL must be a credential-free private origin")
	}
	loopback := endpoint.Hostname() == "localhost"
	if ip := net.ParseIP(endpoint.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	if endpoint.Scheme != "https" && !(endpoint.Scheme == "http" && loopback) {
		return "", errors.New("report transport requires HTTPS or loopback HTTP")
	}
	if endpoint.Path == "" || endpoint.Path == "/" {
		endpoint.Path = "/api/v1/ingest"
	}
	if endpoint.Path != "/api/v1/ingest" || endpoint.RawPath != "" {
		return "", errors.New("report adapter URL must name the private ingestion endpoint")
	}
	if options.Timeout == 0 {
		options.Timeout = 5 * time.Second
	}
	if options.Timeout < time.Second || options.Timeout > 30*time.Second {
		return "", errors.New("report timeout must be between 1 and 30 seconds")
	}
	if options.RetryOnly {
		return endpoint.String(), nil
	}
	if options.SessionID == "" && options.Runtime == "codex" {
		options.SessionID = os.Getenv("CODEX_THREAD_ID")
	}
	if options.SessionID == "" || len(options.SessionID) > 512 || options.DisplayName == "" || len(options.DisplayName) > 256 {
		return "", errors.New("report requires a bounded native session ID and display name")
	}
	if options.AgentStatus == "" {
		options.AgentStatus = "active"
	}
	if !oneOf(options.AgentStatus, "active", "idle", "blocked", "disconnected") {
		return "", errors.New("invalid reported agent status")
	}
	if options.SessionStatus == "" {
		options.SessionStatus = "active"
		if options.AgentStatus == "idle" || options.AgentStatus == "disconnected" {
			options.SessionStatus = "idle"
		}
	}
	if !oneOf(options.SessionStatus, "active", "idle", "closed") {
		return "", errors.New("invalid reported session status")
	}
	if options.RunID == "" {
		if options.RunStatus != "" || options.TaskID != "" || options.Summary != "" {
			return "", errors.New("run metadata requires an explicit native run ID")
		}
	} else if len(options.RunID) > 512 || len(options.TaskID) > 512 || len(options.Summary) > 4096 || !oneOf(options.RunStatus, "running", "idle", "blocked", "completed", "failed", "cancelled") {
		return "", errors.New("invalid bounded native run metadata")
	}
	return endpoint.String(), nil
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if candidate == value {
			return true
		}
	}
	return false
}

func openPrivateReportFile(path string, create bool) (*os.File, error) {
	flags := os.O_RDONLY
	if create {
		flags = os.O_RDWR | os.O_CREATE
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, errors.New("private report directory is unavailable")
	}
	defer root.Close()
	file, err := root.OpenFile(filepath.Base(path), flags|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, errors.New("private report file is unavailable")
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, errors.New("private report file cannot be inspected")
	}
	if !sameReportOwner(info) || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		_ = file.Close()
		return nil, errors.New("report files require current-user ownership and mode 0600")
	}
	return file, nil
}

func sameReportOwner(info os.FileInfo) bool {
	owner, ok := info.Sys().(*syscall.Stat_t)
	uid := os.Getuid()
	if !ok || uid < 0 {
		return false
	}
	return uint64(owner.Uid) == uint64(uid)
}

func lockReportDirectory(path string) (*os.File, error) {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return nil, errors.New("private report state directory cannot be created")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, errors.New("private report state directory is unavailable")
	}
	if !sameReportOwner(info) || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("report state requires a current-user directory with mode 0700")
	}
	lock, err := openPrivateReportFile(filepath.Join(path, "report.lock"), true)
	if err != nil {
		return nil, err
	}
	descriptor := lock.Fd()
	if descriptor > math.MaxInt {
		_ = lock.Close()
		return nil, errors.New("report lock file descriptor is outside the supported range")
	}
	if err := syscall.Flock(int(descriptor), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lock.Close()
		return nil, errors.New("another reporter owns this state directory")
	}
	return lock, nil
}

func readReportState(directory string, identity reportIdentity) (reportState, error) {
	path := filepath.Join(directory, "report.json")
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return reportState{Schema: 1, Identity: identity}, nil
	}
	file, err := openPrivateReportFile(path, false)
	if err != nil {
		return reportState{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, maxReportStateBytes))
	decoder.DisallowUnknownFields()
	var state reportState
	if decoder.Decode(&state) != nil || state.Schema != 1 || state.Sequence < 0 || state.Identity != identity {
		return reportState{}, errors.New("report state is invalid or belongs to another actor, instance, project, runtime, or endpoint")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return reportState{}, errors.New("report state has trailing data")
	}
	return state, nil
}

func writeReportState(directory string, state reportState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return errors.New("report state cannot be encoded")
	}
	if len(data)+1 > maxReportStateBytes {
		return errors.New("report state size limit reached; retry delivery before adding observations")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return errors.New("report state directory is unavailable")
	}
	defer root.Close()
	temporary := ".report-" + randomID()
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return errors.New("report state cannot be persisted")
	}
	defer root.Remove(temporary)
	if _, err := file.Write(append(data, '\n')); err != nil {
		_ = file.Close()
		return errors.New("report state cannot be encoded")
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return errors.New("report state cannot be synchronized")
	}
	if err := file.Close(); err != nil {
		return errors.New("report state cannot be closed")
	}
	if err := root.Rename(temporary, "report.json"); err != nil {
		return errors.New("report state cannot be replaced")
	}
	dir, err := root.Open(".")
	if err != nil {
		return errors.New("report directory cannot be synchronized")
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return errors.New("report directory cannot be synchronized")
	}
	return nil
}

func buildReport(options ReportOptions, state *reportState, now time.Time) (queuedReport, error) {
	types := []struct {
		name string
		kind domain.ResourceType
		id   string
		data map[string]any
	}{
		{"agent.observed", domain.ResourceAgent, options.AgentID, map[string]any{"agent_id": options.AgentID, "instance_id": options.InstanceID, "display_name": options.DisplayName, "status": options.AgentStatus, "observed_at": now}},
		{"session.observed", domain.ResourceSession, options.SessionID, map[string]any{"agent_id": options.AgentID, "session_id": options.SessionID, "status": options.SessionStatus, "observed_at": now}},
	}
	if options.RunID != "" {
		run := map[string]any{"agent_id": options.AgentID, "run_id": options.RunID, "session_id": options.SessionID, "status": options.RunStatus, "observed_at": now}
		if options.TaskID != "" {
			run["task_id"] = options.TaskID
		}
		if options.Summary != "" {
			run["summary"] = options.Summary
		}
		types = append(types, struct {
			name string
			kind domain.ResourceType
			id   string
			data map[string]any
		}{"run.observed", domain.ResourceRun, options.RunID, run})
	}
	var events []domain.Event
	for _, item := range types {
		if state.Sequence == math.MaxInt64 {
			return queuedReport{}, errors.New("report source sequence is exhausted")
		}
		state.Sequence++
		if now.UnixNano() > state.Sequence {
			state.Sequence = now.UnixNano()
		}
		adapterContext := Context{ProjectID: options.ProjectID, ActorID: options.AgentID, AgentID: options.AgentID, SessionID: options.SessionID, RunID: options.RunID, SourceSystem: options.Runtime, SourceSequence: state.Sequence,
			NativeEventID: StableSourceID(options.ProjectID, options.Runtime, options.AgentID, options.InstanceID, item.name, strconv.FormatInt(state.Sequence, 10))}
		event, err := adapterContext.Event(item.name, item.kind, item.id, item.data, now)
		if err != nil {
			return queuedReport{}, errors.New("runtime metadata cannot be normalized")
		}
		events = append(events, event)
	}
	body, err := json.Marshal(struct {
		Events []domain.Event `json:"events"`
	}{events})
	if err != nil {
		return queuedReport{}, errors.New("runtime report cannot be encoded")
	}
	return queuedReport{Body: string(body), ObservedAt: now}, nil
}

// Report explicitly observes a real caller; retry-only never creates observations.
func Report(options ReportOptions) (ReportResult, error) {
	endpoint, err := validateReport(&options)
	if err != nil {
		return ReportResult{}, err
	}
	lock, err := lockReportDirectory(options.StateDir)
	if err != nil {
		return ReportResult{}, err
	}
	defer lock.Close()
	identity := reportIdentity{options.ProjectID, options.Runtime, options.AgentID, options.InstanceID, endpoint}
	state, err := readReportState(options.StateDir, identity)
	if err != nil {
		return ReportResult{}, err
	}
	now := time.Now().UTC()
	if options.Now != nil {
		now = options.Now().UTC()
	}
	if !options.RetryOnly {
		if len(state.Queue) >= 1000 {
			return ReportResult{}, errors.New("report queue is full; retry delivery before adding observations")
		}
		item, err := buildReport(options, &state, now)
		if err != nil {
			return ReportResult{}, err
		}
		state.Queue = append(state.Queue, item)
		if err := writeReportState(options.StateDir, state); err != nil {
			return ReportResult{}, err
		}
	}
	result, err := deliverReports(options, endpoint, &state, now)
	return result, err
}

func deliverReports(options ReportOptions, endpoint string, state *reportState, now time.Time) (ReportResult, error) {
	result := ReportResult{}
	var deliveryError error
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	client := &http.Client{Transport: transport, Timeout: options.Timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	defer transport.CloseIdleConnections()
	for index := 0; index < len(state.Queue); {
		item := &state.Queue[index]
		if item.Quarantined != "" {
			index++
			continue
		}
		if now.Sub(item.ObservedAt) > 30*24*time.Hour {
			item.Quarantined = "observation_older_than_30_days_requires_review"
			if err := writeReportState(options.StateDir, *state); err != nil {
				return result, err
			}
			continue
		}
		if result.Delivered >= 10 {
			break
		}
		status, err := publishReport(client, options.TokenFile, endpoint, options.ProjectID, []byte(item.Body))
		if err != nil || status != http.StatusAccepted {
			if oneOf(strconv.Itoa(status), "400", "409", "413", "422") {
				item.Quarantined = fmt.Sprintf("http_%d_requires_review", status)
				if err := writeReportState(options.StateDir, *state); err != nil {
					return result, err
				}
				continue
			}
			deliveryError = err
			if deliveryError == nil {
				deliveryError = fmt.Errorf("runtime report HTTP %d; queued observations retained", status)
			}
			break
		}
		state.Queue = append(state.Queue[:index], state.Queue[index+1:]...)
		if err := writeReportState(options.StateDir, *state); err != nil {
			return result, err
		}
		result.Delivered++
	}
	for _, item := range state.Queue {
		if item.Quarantined != "" {
			result.Quarantined++
		} else {
			result.Pending++
		}
	}
	if deliveryError == nil && (result.Pending > 0 || result.Quarantined > 0) {
		deliveryError = errors.New("runtime observations remain pending or quarantined; inspect the private state and retry explicitly")
	}
	return result, deliveryError
}

func publishReport(client *http.Client, tokenFile, endpoint, projectID string, body []byte) (int, error) {
	file, err := openPrivateReportFile(tokenFile, false)
	if err != nil {
		return 0, err
	}
	data, err := io.ReadAll(io.LimitReader(file, 16385))
	_ = file.Close()
	if err != nil {
		return 0, errors.New("report bearer file cannot be read")
	}
	token := strings.TrimSpace(string(data))
	if len(token) == 0 || len(token) > 16384 || strings.ContainsFunc(token, func(value rune) bool { return value <= 32 || value >= 127 }) {
		return 0, errors.New("report bearer file is invalid")
	}
	request, err := http.NewRequest(http.MethodPost, endpoint+"?"+url.Values{"project_id": {projectID}}.Encode(), bytes.NewReader(body))
	if err != nil {
		return 0, errors.New("runtime report request cannot be constructed")
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return 0, errors.New("runtime report transport failed; queued observations retained")
	}
	defer response.Body.Close()
	return response.StatusCode, nil
}
