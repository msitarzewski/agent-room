package hermes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/msitarzewski/agent-room/internal/adapters"
	"github.com/msitarzewski/agent-room/internal/domain"
)

const CompatibleRelease = "v2026.7.20"

type HookPayload struct {
	EventName    string          `json:"event_name"`
	SessionID    string          `json:"session_id"`
	TurnID       string          `json:"turn_id"`
	APIRequestID string          `json:"api_request_id"`
	TaskID       string          `json:"task_id"`
	APICallCount int             `json:"api_call_count"`
	ToolName     string          `json:"tool_name"`
	Status       string          `json:"status"`
	Timestamp    time.Time       `json:"timestamp"`
	Data         json.RawMessage `json:"data"`
}

func DecodeHook(payload HookPayload, adapterContext adapters.Context) (domain.Event, error) {
	if adapterContext.NativeEventID == "" || adapterContext.SourceSequence <= 0 || adapters.MetadataToken(payload.SessionID, 512) == "" {
		return domain.Event{}, errors.New("hermes hook requires a native invocation ID, durable sequence, and session ID")
	}
	eventType := map[string]string{
		"session_start": "session.started", "session_end": "session.completed",
		"pre_tool_call": "tool.started", "post_tool_call": "tool.completed",
		"pre_llm_call": "turn.started", "post_llm_call": "turn.completed",
		"subagent_start": "run.started", "subagent_end": "run.completed",
		"approval_requested": "approval.requested", "approval_resolved": "approval.resolved",
	}[payload.EventName]
	if eventType == "" {
		eventType = "hermes.hook.observed"
	}
	subjectType, subjectID := domain.ResourceSession, payload.SessionID
	if strings.HasPrefix(eventType, "run.") || strings.HasPrefix(eventType, "turn.") || strings.HasPrefix(eventType, "tool.") {
		subjectType, subjectID = domain.ResourceRun, payload.TurnID
		if subjectID == "" {
			subjectID = adapterContext.RunID
		}
		if payload.Status == "failed" {
			eventType = "run.failed"
		}
	}
	if adapters.MetadataToken(subjectID, 512) == "" {
		return domain.Event{}, errors.New("invalid native hermes subject identifier")
	}
	event, err := adapterContext.Event(eventType, subjectType, subjectID, map[string]any{
		"source": "hermes_hook", "compatible_release": CompatibleRelease, "event_name": adapters.MetadataToken(payload.EventName, 64),
		"session_id": adapters.MetadataToken(payload.SessionID, 512), "turn_id": adapters.MetadataToken(payload.TurnID, 512), "api_request_id": adapters.MetadataToken(payload.APIRequestID, 512),
		"task_id": adapters.MetadataToken(payload.TaskID, 512), "agent_id": adapterContext.AgentID, "tool_name": adapters.MetadataToken(payload.ToolName, 128),
		"failed": payload.Status == "failed",
	}, payload.Timestamp)
	event.SourceSystem = "hermes"
	event.SourceEventID = adapters.StableSourceID(adapterContext.ProjectID, payload.SessionID, adapterContext.NativeEventID)
	return event, err
}

type Client struct {
	baseURL *url.URL
	token   string
	http    *http.Client
}

func NewClient(baseURL, token string) (*Client, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("hermes base URL must be absolute and credential-free")
	}
	loopback := parsed.Hostname() == "localhost"
	if ip := net.ParseIP(parsed.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopback) {
		return nil, errors.New("hermes transport requires HTTPS or loopback HTTP")
	}
	if token == "" || len(token) > 16384 || strings.ContainsFunc(token, func(value rune) bool { return value <= 32 || value >= 127 }) {
		return nil, errors.New("hermes bearer token is required")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Client{baseURL: parsed, token: token, http: &http.Client{Timeout: 20 * time.Second, Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (c *Client) Sessions(ctx context.Context) (json.RawMessage, error) {
	return c.get(ctx, "/api/sessions")
}

func (c *Client) Session(ctx context.Context, id string) (json.RawMessage, error) {
	return c.get(ctx, "/api/sessions/"+url.PathEscape(id))
}

func (c *Client) Chat(ctx context.Context, sessionID, message string) (json.RawMessage, error) {
	body, _ := json.Marshal(map[string]string{"input": message})
	return c.do(ctx, http.MethodPost, "/api/sessions/"+url.PathEscape(sessionID)+"/chat", bytes.NewReader(body))
}

func (c *Client) get(ctx context.Context, path string) (json.RawMessage, error) {
	return c.do(ctx, http.MethodGet, path, nil)
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader) (json.RawMessage, error) {
	endpoint := c.baseURL.ResolveReference(&url.URL{Path: strings.TrimSuffix(c.baseURL.Path, "/") + path})
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return nil, errors.New("hermes API request cannot be constructed")
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, errors.New("hermes API transport failed")
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, errors.New("hermes API response cannot be read")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("hermes API HTTP %d", response.StatusCode)
	}
	if !json.Valid(content) {
		return nil, errors.New("hermes API returned invalid JSON")
	}
	return content, nil
}
