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

type mcpCoordinationCreateInput struct {
	app.CreateCoordinationInput
	IdempotencyKey string `json:"idempotency_key"`
}

type mcpCoordinationActionInput struct {
	RequestID       string `json:"request_id"`
	Action          string `json:"action" jsonschema:"respond, resolve or cancel"`
	ExpectedVersion int64  `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
	app.CoordinationActionInput
}

type mcpEmptyInput struct{}

type mcpBriefInput struct {
	AfterCursor int64 `json:"after_cursor,omitempty" jsonschema:"optional non-negative event cursor to read changes after"`
}

type mcpTimelineInput struct {
	Cursor       int64  `json:"cursor,omitempty" jsonschema:"optional cursor boundary for paging older events"`
	Limit        int    `json:"limit,omitempty" jsonschema:"maximum number of events to return, from 1 to 200"`
	EventType    string `json:"event_type,omitempty" jsonschema:"optional exact event type filter"`
	Filter       string `json:"filter,omitempty" jsonschema:"optional text search filter"`
	IncludeTypes bool   `json:"include_types,omitempty" jsonschema:"include the list of visible event types in the project"`
}

type mcpTimelineOutput struct {
	Items      []domain.Event `json:"items"`
	NextCursor int64          `json:"next_cursor,omitempty"`
	EventTypes []string       `json:"event_types,omitempty"`
}

type mcpMapOutput struct {
	Data map[string]any `json:"data"`
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
			{"list_agents", "List agents and their presence in the authorized project.", "agent:read", domain.ResourceAgent},
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
		mcp.AddTool(server, &mcp.Tool{Name: "list_coordination_requests", Description: "Read explicit requests owned by or addressed to this participant; full project readers see all."},
			func(ctx context.Context, _ *mcp.CallToolRequest, input mcpListInput) (*mcp.CallToolResult, mcpListOutput, error) {
				page, err := s.service.List(ctx, actor, projectID, domain.ResourceCoordinationRequest, input.Cursor, input.Limit)
				output, err := mcpListPage(page, err)
				return nil, output, err
			})
		mcp.AddTool(server, &mcp.Tool{Name: "create_coordination_request", Description: "Ask a named participant for help, context, handoff or review. Does not execute work or transfer task ownership."},
			func(ctx context.Context, _ *mcp.CallToolRequest, input mcpCoordinationCreateInput) (*mcp.CallToolResult, mcpMutationOutput, error) {
				result, err := s.service.CreateCoordination(ctx, actor, projectID, input.IdempotencyKey, input.CreateCoordinationInput, "mcp")
				output, err := mcpMutationResult(result, err)
				return nil, output, err
			})
		mcp.AddTool(server, &mcp.Tool{Name: "act_coordination_request", Description: "Named recipient responds; requester resolves or cancels. Requires current version and idempotency key; human recipients require human participation."},
			func(ctx context.Context, _ *mcp.CallToolRequest, input mcpCoordinationActionInput) (*mcp.CallToolResult, mcpMutationOutput, error) {
				result, err := s.service.ActCoordination(ctx, actor, projectID, input.RequestID, input.Action, input.IdempotencyKey, input.ExpectedVersion, input.CoordinationActionInput, "mcp")
				output, err := mcpMutationResult(result, err)
				return nil, output, err
			})
		mcp.AddTool(server, &mcp.Tool{Name: "get_project_overview", Description: "Get resource counts, coordination summary, source freshness, and event cursor for the authorized project."},
			func(ctx context.Context, _ *mcp.CallToolRequest, _ mcpEmptyInput) (*mcp.CallToolResult, mcpMapOutput, error) {
				data, err := s.service.Overview(ctx, actor, projectID)
				if err != nil {
					return nil, mcpMapOutput{}, err
				}
				return nil, mcpMapOutput{Data: data}, nil
			})
		mcp.AddTool(server, &mcp.Tool{Name: "get_return_brief", Description: "Get the durable coordination return brief, event counts, and open attention items for the authorized project."},
			func(ctx context.Context, _ *mcp.CallToolRequest, input mcpBriefInput) (*mcp.CallToolResult, mcpMapOutput, error) {
				if input.AfterCursor < 0 {
					return nil, mcpMapOutput{}, app.Invalid("after_cursor must be non-negative")
				}
				data, err := s.service.Brief(ctx, actor, projectID, input.AfterCursor)
				if err != nil {
					return nil, mcpMapOutput{}, err
				}
				return nil, mcpMapOutput{Data: data}, nil
			})
		mcp.AddTool(server, &mcp.Tool{Name: "list_timeline", Description: "List project events newest-first with optional event-type and text filters."},
			func(ctx context.Context, _ *mcp.CallToolRequest, input mcpTimelineInput) (*mcp.CallToolResult, mcpTimelineOutput, error) {
				if input.Cursor < 0 {
					return nil, mcpTimelineOutput{}, app.Invalid("cursor must be non-negative")
				}
				limit := input.Limit
				if limit <= 0 {
					limit = 50
				}
				if limit > 200 {
					limit = 200
				}
				page, err := s.service.ListEvents(ctx, actor, projectID, app.EventQuery{
					Cursor:       input.Cursor,
					Limit:        limit,
					EventType:    input.EventType,
					Filter:       input.Filter,
					Descending:   true,
					IncludeTypes: input.IncludeTypes,
				})
				if err != nil {
					return nil, mcpTimelineOutput{}, err
				}
				return nil, mcpTimelineOutput{Items: page.Items, NextCursor: page.NextCursor, EventTypes: page.EventTypes}, nil
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
