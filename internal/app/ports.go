package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/msitarzewski/agent-room/internal/domain"
)

var (
	ErrNotFound        = errors.New("resource not found")
	ErrVersionConflict = errors.New("resource version conflict")
	ErrIdempotency     = errors.New("idempotency key reused with different command")
	ErrDenied          = errors.New("operation denied")
	ErrUnsupported     = errors.New("operation is not supported by the live runtime")
	ErrBudgetExceeded  = errors.New("an enforced project budget is exhausted")
)

type ValidationError struct{ Message string }

func (e ValidationError) Error() string { return e.Message }

func Invalid(message string) error { return ValidationError{Message: message} }

type Page struct {
	Items      []json.RawMessage `json:"items"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

type EventPage struct {
	Items      []domain.Event `json:"items"`
	NextCursor int64          `json:"next_cursor,omitempty"`
}

// WorkspaceSnapshot is a complete project projection and its event watermark,
// read from the same database snapshot. The resource arrays are always present.
type WorkspaceSnapshot struct {
	ProjectID   string             `json:"project_id"`
	Cursor      int64              `json:"cursor"`
	GeneratedAt time.Time          `json:"generated_at"`
	Resources   WorkspaceResources `json:"resources"`
	Sources     []WorkspaceSource  `json:"sources"`
}

type WorkspaceResources struct {
	Tasks     []json.RawMessage `json:"tasks"`
	Agents    []json.RawMessage `json:"agents"`
	Runs      []json.RawMessage `json:"runs"`
	Attention []json.RawMessage `json:"attention"`
	Evidence  []json.RawMessage `json:"evidence"`
	Artifacts []json.RawMessage `json:"artifacts"`
	Claims    []json.RawMessage `json:"claims"`
	Approvals []json.RawMessage `json:"approvals"`
	Requests  []json.RawMessage `json:"requests"`
}

type WorkspaceSource struct {
	System                     string                     `json:"system"`
	LastSeen                   time.Time                  `json:"last_seen"`
	LastReceivedAt             time.Time                  `json:"last_received_at"`
	LastCollectedAt            *time.Time                 `json:"last_collected_at"`
	LastSuccessfulCollectionAt *time.Time                 `json:"last_successful_collection_at"`
	ScanStatus                 *string                    `json:"scan_status"`
	ProjectObservation         *domain.ProjectObservation `json:"project_observation,omitempty"`
}

type CreateCoordinationInput struct {
	ID               string                       `json:"id"`
	Kind             string                       `json:"kind"`
	Title            string                       `json:"title"`
	Body             string                       `json:"body"`
	Recipient        domain.CoordinationRecipient `json:"recipient"`
	TaskID           string                       `json:"task_id,omitempty"`
	EvidenceIDs      []string                     `json:"evidence_ids,omitempty"`
	EscalationReason string                       `json:"escalation_reason,omitempty"`
}

type CoordinationActionInput struct {
	Body        string   `json:"body,omitempty"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
	Note        string   `json:"note,omitempty"`
}

type CoordinationCounts struct {
	RunningWork           int `json:"running_work"`
	AgentPresence         int `json:"agent_presence"`
	UnfinishedTasks       int `json:"unfinished_tasks"`
	SourceUnfinishedTasks int `json:"source_unfinished_tasks"`
	BlockedTasks          int `json:"blocked_tasks"`
	CanonicalBlockedTasks int `json:"canonical_blocked_tasks"`
	SourceBlockedTasks    int `json:"source_blocked_tasks"`
	Requests              int `json:"requests"`
	Reviews               int `json:"reviews"`
	HumanEscalations      int `json:"human_escalations"`
	HumanApprovals        int `json:"human_approvals"`
	OpenAttention         int `json:"open_attention"`
}

type CoordinationItem struct {
	Kind            string                        `json:"kind"`
	ResourceType    domain.ResourceType           `json:"resource_type"`
	ID              string                        `json:"id"`
	Title           string                        `json:"title"`
	Detail          string                        `json:"detail,omitempty"`
	Status          string                        `json:"status"`
	UpdatedAt       time.Time                     `json:"updated_at"`
	OwnerID         string                        `json:"owner_id,omitempty"`
	OwnerName       string                        `json:"owner_name,omitempty"`
	RecipientName   string                        `json:"recipient_name,omitempty"`
	RequestKind     string                        `json:"request_kind,omitempty"`
	Recipient       *domain.CoordinationRecipient `json:"recipient,omitempty"`
	TaskID          string                        `json:"task_id,omitempty"`
	TargetType      string                        `json:"target_type,omitempty"`
	TargetID        string                        `json:"target_id,omitempty"`
	EvidenceIDs     []string                      `json:"evidence_ids,omitempty"`
	SourceSystem    string                        `json:"source_system,omitempty"`
	HumanEscalation bool                          `json:"human_escalation"`
}

type CoordinationSummary struct {
	Counts    CoordinationCounts `json:"counts"`
	Items     []CoordinationItem `json:"items"`
	Truncated bool               `json:"truncated"`
	Complete  bool               `json:"complete"`
}

type WorkspaceRepository interface {
	Workspace(context.Context, string) (WorkspaceSnapshot, error)
}

type Mutation struct {
	Resource json.RawMessage
	Event    domain.Event
	Audit    domain.AuditRecord
	Control  *ControlIntent
}

// ControlIntent is a durable request for a side effect against a managed
// runtime. Repositories persist it in the same transaction as the command,
// event, audit record, and approval consumption. A post-commit dispatcher is
// the only component allowed to execute it.
type ControlIntent struct {
	ID          string
	ProjectID   string
	RunID       string
	ActorID     string
	CommandID   string
	Action      string
	Message     string
	RequestedAt time.Time
}

type Decision struct {
	ID, ProjectID, ActorID, Action, ResourceType, ResourceID string
	IdempotencyKey, Outcome, ReasonClass, CorrelationID      string
	OccurredAt                                               time.Time
}

type CommandResult struct {
	Resource json.RawMessage `json:"resource"`
	Event    domain.Event    `json:"event"`
	Replayed bool            `json:"replayed"`
}

type Repository interface {
	Get(ctx context.Context, projectID string, kind domain.ResourceType, id string) (json.RawMessage, error)
	List(ctx context.Context, projectID string, kind domain.ResourceType, cursor string, limit int) (Page, error)
	Execute(ctx context.Context, cmd domain.Command, build func(current json.RawMessage) (Mutation, error)) (CommandResult, error)
	Ingest(ctx context.Context, events []domain.Event) error
	Events(ctx context.Context, projectID string, after int64, limit int) (EventPage, error)
	Overview(ctx context.Context, projectID string) (map[string]any, error)
	Health(ctx context.Context) error
	Close()
}

type Publisher interface {
	Publish(domain.Event)
}

type RunController interface {
	Supports(runID, action string) bool
	Execute(ctx context.Context, runID, action, message string) error
}

type Clock interface {
	Now() time.Time
}

type IDGenerator interface {
	New() string
}

type Actor struct {
	Human        bool
	ID           string
	Capabilities map[string]struct{}
}

func (a Actor) Can(capability string) bool {
	_, ok := a.Capabilities[capability]
	return ok
}
