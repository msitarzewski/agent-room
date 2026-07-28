package hermes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	if payload.Status == "failed" {
		eventType = "run.failed"
	}
	subjectType, subjectID := domain.ResourceSession, payload.SessionID
	if payload.TaskID != "" {
		subjectType, subjectID = domain.ResourceTask, payload.TaskID
	} else if payload.TurnID != "" {
		subjectType, subjectID = domain.ResourceRun, payload.TurnID
	}
	event, err := adapterContext.Event(eventType, subjectType, subjectID, map[string]any{
		"source": "hermes_hook", "compatible_release": CompatibleRelease, "event_name": payload.EventName,
		"session_id": payload.SessionID, "turn_id": payload.TurnID, "api_request_id": payload.APIRequestID,
		"task_id": payload.TaskID, "api_call_count": payload.APICallCount, "tool_name": payload.ToolName,
		"status": payload.Status, "data": payload.Data,
	}, payload.Timestamp)
	event.SourceSystem = "hermes"
	event.SourceEventID = adapters.StableSourceID(payload.SessionID, payload.TurnID, payload.APIRequestID, payload.EventName, string(payload.Data))
	event.SourceSequence = int64(payload.APICallCount)
	return event, err
}

type Client struct {
	baseURL *url.URL
	token   string
	http    *http.Client
}

func NewClient(baseURL, token string) (*Client, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("hermes base URL must be absolute")
	}
	if token == "" {
		return nil, errors.New("hermes bearer token is required")
	}
	return &Client{baseURL: parsed, token: token, http: &http.Client{Timeout: 20 * time.Second}}, nil
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
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("hermes API %s: HTTP %d", path, response.StatusCode)
	}
	if !json.Valid(content) {
		return nil, errors.New("hermes API returned invalid JSON")
	}
	return content, nil
}
