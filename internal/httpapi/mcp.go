package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/msitarzewski/agent-room/internal/app"
	"github.com/msitarzewski/agent-room/internal/domain"
)

type mcpListInput struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty" jsonschema:"maximum number of items to return, from 1 to 200"`
}

type mcpListOutput struct {
	Items      []any  `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

type mcpChatInput struct {
	Body           string `json:"body" jsonschema:"non-empty chat message body"`
	SessionID      string `json:"session_id,omitempty"`
	RunID          string `json:"run_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key" jsonschema:"non-empty idempotency key of at most 200 characters"`
}

type mcpMutationOutput struct {
	Resource any   `json:"resource"`
	Cursor   int64 `json:"cursor"`
}

type mcpTaskTransitionInput struct {
	TaskID          string `json:"task_id" jsonschema:"non-empty task identifier"`
	Status          string `json:"status" jsonschema:"target task status: inbox, ready, working, review, completed, archived, blocked, cancelled, reopened, or failed"`
	Reason          string `json:"reason,omitempty"`
	ExpectedVersion int64  `json:"expected_version" jsonschema:"positive expected resource version"`
	IdempotencyKey  string `json:"idempotency_key" jsonschema:"non-empty idempotency key of at most 200 characters"`
}

type mcpAttentionInput struct {
	Kind           string `json:"kind" jsonschema:"non-empty attention kind"`
	Severity       string `json:"severity" jsonschema:"attention severity: normal, high, or critical"`
	Title          string `json:"title" jsonschema:"non-empty attention title"`
	Detail         string `json:"detail,omitempty"`
	ResourceType   string `json:"resource_type,omitempty"`
	ResourceID     string `json:"resource_id,omitempty"`
	IdempotencyKey string `json:"idempotency_key" jsonschema:"non-empty idempotency key of at most 200 characters"`
}

type mcpEvidenceInput struct {
	TaskID         string `json:"task_id,omitempty"`
	RunID          string `json:"run_id,omitempty"`
	Kind           string `json:"kind" jsonschema:"non-empty evidence kind"`
	Summary        string `json:"summary" jsonschema:"non-empty evidence summary"`
	SourceSystem   string `json:"source_system" jsonschema:"non-empty source system"`
	ExternalID     string `json:"external_id,omitempty"`
	ExternalURL    string `json:"external_url,omitempty"`
	Digest         string `json:"digest,omitempty"`
	IdempotencyKey string `json:"idempotency_key" jsonschema:"non-empty idempotency key of at most 200 characters"`
}

type mcpClaimInput struct {
	TaskID          string `json:"task_id" jsonschema:"non-empty task identifier"`
	ExpectedVersion int64  `json:"expected_version" jsonschema:"positive expected resource version"`
	IdempotencyKey  string `json:"idempotency_key" jsonschema:"non-empty idempotency key of at most 200 characters"`
}

type mcpReviewInput struct {
	TaskID          string `json:"task_id" jsonschema:"non-empty task identifier"`
	Reason          string `json:"reason,omitempty"`
	ExpectedVersion int64  `json:"expected_version" jsonschema:"positive expected resource version"`
	IdempotencyKey  string `json:"idempotency_key" jsonschema:"non-empty idempotency key of at most 200 characters"`
}

func (s *Server) mcpHandler() http.Handler {
	return mcp.NewStreamableHTTPHandler(func(request *http.Request) *mcp.Server {
		projectID := request.URL.Query().Get("project_id")
		actor := currentActor(request)
		if projectID == "" || actor.ID == "" {
			return nil
		}
		server := mcp.NewServer(&mcp.Implementation{Name: "agent-room", Title: "Agent Room", Version: "1.0.0"}, nil)
		mcp.AddTool(server, &mcp.Tool{Name: "list_tasks", Description: "List normalized tasks in the authorized Agent Room project."},
			func(ctx context.Context, _ *mcp.CallToolRequest, input mcpListInput) (*mcp.CallToolResult, mcpListOutput, error) {
				if !actor.Can("task:read") && !actor.Can("resource:read") {
					return nil, mcpListOutput{}, app.ErrDenied
				}
				page, err := s.service.List(ctx, actor, projectID, domain.ResourceTask, input.Cursor, input.Limit)
				output, err := mcpListPage(page, err)
				return nil, output, err
			})
		mcp.AddTool(server, &mcp.Tool{Name: "list_attention", Description: "List actionable attention items in the authorized project."},
			func(ctx context.Context, _ *mcp.CallToolRequest, input mcpListInput) (*mcp.CallToolResult, mcpListOutput, error) {
				if !actor.Can("attention:read") && !actor.Can("resource:read") {
					return nil, mcpListOutput{}, app.ErrDenied
				}
				page, err := s.service.List(ctx, actor, projectID, domain.ResourceAttention, input.Cursor, input.Limit)
				output, err := mcpListPage(page, err)
				return nil, output, err
			})
		for _, definition := range []struct {
			name, description, capability string
			kind                          domain.ResourceType
		}{
			{"list_runs", "List worker runs in the authorized project.", "run:read", domain.ResourceRun},
			{"list_situations", "List correlated situations in the authorized project.", "situation:read", domain.ResourceSituation},
			{"list_evidence", "List completion evidence in the authorized project.", "evidence:read", domain.ResourceEvidence},
			{"list_artifacts", "List immutable artifact metadata in the authorized project.", "artifact:read", domain.ResourceArtifact},
			{"list_approvals", "List approval requests in the authorized project.", "approval:read", domain.ResourceApproval},
		} {
			definition := definition
			mcp.AddTool(server, &mcp.Tool{Name: definition.name, Description: definition.description},
				func(ctx context.Context, _ *mcp.CallToolRequest, input mcpListInput) (*mcp.CallToolResult, mcpListOutput, error) {
					if !actor.Can(definition.capability) && !actor.Can("resource:read") {
						return nil, mcpListOutput{}, app.ErrDenied
					}
					page, err := s.service.List(ctx, actor, projectID, definition.kind, input.Cursor, input.Limit)
					output, err := mcpListPage(page, err)
					return nil, output, err
				})
		}
		mcp.AddTool(server, &mcp.Tool{Name: "send_chat_message", Description: "Send a coordination message; chat is not an implicit command bus."},
			func(ctx context.Context, _ *mcp.CallToolRequest, input mcpChatInput) (*mcp.CallToolResult, mcpMutationOutput, error) {
				message := domain.ChatMessage{Base: domain.Base{ID: s.service.NewID()}, SessionID: input.SessionID, RunID: input.RunID, AuthorID: actor.ID, Role: "agent", Body: input.Body}
				result, err := s.service.Create(ctx, actor, domain.ResourceChatMessage, projectID, input.IdempotencyKey, message, "mcp")
				output, err := mcpMutationResult(result, err)
				return nil, output, err
			})
		mcp.AddTool(server, &mcp.Tool{Name: "transition_task", Description: "Request an allowed, version-checked task transition."},
			func(ctx context.Context, _ *mcp.CallToolRequest, input mcpTaskTransitionInput) (*mcp.CallToolResult, mcpMutationOutput, error) {
				result, err := s.service.TransitionTask(ctx, actor, projectID, input.TaskID, input.Status, input.Reason, input.IdempotencyKey, input.ExpectedVersion, "mcp")
				output, err := mcpMutationResult(result, err)
				return nil, output, err
			})
		mcp.AddTool(server, &mcp.Tool{Name: "request_attention", Description: "Create a structured attention request for human review."},
			func(ctx context.Context, _ *mcp.CallToolRequest, input mcpAttentionInput) (*mcp.CallToolResult, mcpMutationOutput, error) {
				if !actor.Can("attention:write") && !actor.Can("resource:write") {
					return nil, mcpMutationOutput{}, app.ErrDenied
				}
				item := domain.Attention{
					Base: domain.Base{ID: s.service.NewID()}, Kind: input.Kind, Severity: input.Severity,
					Title: input.Title, Detail: input.Detail, ResourceType: input.ResourceType,
					ResourceID: input.ResourceID, Status: "open",
				}
				result, err := s.service.Create(ctx, actor, domain.ResourceAttention, projectID, input.IdempotencyKey, item, "mcp")
				output, err := mcpMutationResult(result, err)
				return nil, output, err
			})
		mcp.AddTool(server, &mcp.Tool{Name: "post_evidence", Description: "Attach provenance-rich evidence to a task or run."},
			func(ctx context.Context, _ *mcp.CallToolRequest, input mcpEvidenceInput) (*mcp.CallToolResult, mcpMutationOutput, error) {
				if !actor.Can("evidence:write") && !actor.Can("resource:write") {
					return nil, mcpMutationOutput{}, app.ErrDenied
				}
				evidence := domain.Evidence{
					Base: domain.Base{ID: s.service.NewID()}, TaskID: input.TaskID, RunID: input.RunID,
					Kind: input.Kind, Summary: input.Summary, Digest: input.Digest,
					Source: domain.Source{System: input.SourceSystem, ExternalID: input.ExternalID, ExternalURL: input.ExternalURL},
				}
				result, err := s.service.Create(ctx, actor, domain.ResourceEvidence, projectID, input.IdempotencyKey, evidence, "mcp")
				output, err := mcpMutationResult(result, err)
				return nil, output, err
			})
		mcp.AddTool(server, &mcp.Tool{Name: "claim_task", Description: "Claim ownership of a task for the authenticated agent identity."},
			func(ctx context.Context, _ *mcp.CallToolRequest, input mcpClaimInput) (*mcp.CallToolResult, mcpMutationOutput, error) {
				if !actor.Can("task:claim") {
					return nil, mcpMutationOutput{}, app.ErrDenied
				}
				result, err := s.service.ClaimTask(ctx, actor, projectID, input.TaskID, input.IdempotencyKey, input.ExpectedVersion, "mcp")
				output, err := mcpMutationResult(result, err)
				return nil, output, err
			})
		mcp.AddTool(server, &mcp.Tool{Name: "request_review", Description: "Move owned work to review through a version-checked task transition."},
			func(ctx context.Context, _ *mcp.CallToolRequest, input mcpReviewInput) (*mcp.CallToolResult, mcpMutationOutput, error) {
				if !actor.Can("task:review") {
					return nil, mcpMutationOutput{}, app.ErrDenied
				}
				result, err := s.service.RequestTaskReview(ctx, actor, projectID, input.TaskID, input.Reason, input.IdempotencyKey, input.ExpectedVersion, "mcp")
				output, err := mcpMutationResult(result, err)
				return nil, output, err
			})
		return server
	}, &mcp.StreamableHTTPOptions{Stateless: true, SessionTimeout: 30 * time.Minute})
}

func mcpListPage(page app.Page, listErr error) (mcpListOutput, error) {
	if listErr != nil {
		return mcpListOutput{}, listErr
	}
	items := make([]any, 0, len(page.Items))
	for _, raw := range page.Items {
		var item any
		if err := json.Unmarshal(raw, &item); err != nil {
			return mcpListOutput{}, err
		}
		items = append(items, item)
	}
	return mcpListOutput{Items: items, NextCursor: page.NextCursor}, nil
}

func mcpMutationResult(result app.CommandResult, commandErr error) (mcpMutationOutput, error) {
	if commandErr != nil {
		return mcpMutationOutput{}, commandErr
	}
	var resource any
	if err := json.Unmarshal(result.Resource, &resource); err != nil {
		return mcpMutationOutput{}, err
	}
	return mcpMutationOutput{Resource: resource, Cursor: result.Event.Cursor}, nil
}
