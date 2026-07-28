package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ResourceType string

const (
	ResourceAgent          ResourceType = "agent"
	ResourceRun            ResourceType = "run"
	ResourceSession        ResourceType = "session"
	ResourceTask           ResourceType = "task"
	ResourceAttention      ResourceType = "attention"
	ResourceEvidence       ResourceType = "evidence"
	ResourceArtifact       ResourceType = "artifact"
	ResourceApproval       ResourceType = "approval"
	ResourceIntervention   ResourceType = "intervention"
	ResourceChatMessage    ResourceType = "chat_message"
	ResourceBudget         ResourceType = "budget"
	ResourceClaim          ResourceType = "claim"
	ResourceAudit          ResourceType = "audit"
	ResourceOrganization   ResourceType = "organization"
	ResourceHuman          ResourceType = "human"
	ResourceProject        ResourceType = "project"
	ResourceHost           ResourceType = "host"
	ResourceAgentInstance  ResourceType = "agent_instance"
	ResourceTaskTransition ResourceType = "task_transition"
	ResourceSituation      ResourceType = "situation"
	ResourcePolicy         ResourceType = "policy"
	ResourceDeployment     ResourceType = "deployment"
)

type Base struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (b Base) Validate() error {
	if strings.TrimSpace(b.ID) == "" {
		return errors.New("id is required")
	}
	if strings.TrimSpace(b.ProjectID) == "" {
		return errors.New("project_id is required")
	}
	if b.Version < 1 {
		return errors.New("version must be positive")
	}
	return nil
}

type Agent struct {
	Base
	DisplayName  string   `json:"display_name"`
	Kind         string   `json:"kind"`
	Status       string   `json:"status"`
	Capabilities []string `json:"capabilities"`
	Source       Source   `json:"source"`
}

type Organization struct {
	Base
	Name   string `json:"name"`
	Status string `json:"status"`
}

type Human struct {
	Base
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Status      string `json:"status"`
}

type Project struct {
	Base
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	Status         string `json:"status"`
}

type Host struct {
	Base
	Hostname string            `json:"hostname"`
	OS       string            `json:"os"`
	Arch     string            `json:"arch"`
	Status   string            `json:"status"`
	Labels   map[string]string `json:"labels,omitempty"`
}

type AgentInstance struct {
	Base
	AgentID   string     `json:"agent_id"`
	HostID    string     `json:"host_id"`
	Runtime   string     `json:"runtime"`
	ProcessID int        `json:"process_id,omitempty"`
	Status    string     `json:"status"`
	StartedAt time.Time  `json:"started_at"`
	StoppedAt *time.Time `json:"stopped_at,omitempty"`
}

type Run struct {
	Base
	AgentID       string   `json:"agent_id"`
	SessionID     string   `json:"session_id,omitempty"`
	TaskID        string   `json:"task_id,omitempty"`
	Status        string   `json:"status"`
	Summary       string   `json:"summary,omitempty"`
	ControlStatus string   `json:"control_status,omitempty"`
	ControlAction string   `json:"control_action,omitempty"`
	ControlError  string   `json:"control_error,omitempty"`
	Capabilities  []string `json:"capabilities"`
	Source        Source   `json:"source"`
}

type Session struct {
	Base
	AgentID          string     `json:"agent_id"`
	Status           string     `json:"status"`
	ExternalRef      string     `json:"external_ref,omitempty"`
	ConversationRef  string     `json:"conversation_ref,omitempty"`
	Source           Source     `json:"source"`
	LastReconciledAt *time.Time `json:"last_reconciled_at,omitempty"`
}

type Task struct {
	Base
	Title         string   `json:"title"`
	Description   string   `json:"description,omitempty"`
	OwnerID       string   `json:"owner_id,omitempty"`
	Status        string   `json:"status"`
	Priority      string   `json:"priority"`
	Dependencies  []string `json:"dependencies"`
	BudgetID      string   `json:"budget_id,omitempty"`
	ReviewState   string   `json:"review_state,omitempty"`
	ApprovalState string   `json:"approval_state,omitempty"`
	BlockedReason string   `json:"blocked_reason,omitempty"`
	Source        Source   `json:"source"`
}

type TaskTransition struct {
	Base
	TaskID    string `json:"task_id"`
	From      string `json:"from"`
	To        string `json:"to"`
	ActorID   string `json:"actor_id"`
	Reason    string `json:"reason,omitempty"`
	CommandID string `json:"command_id"`
}

func (t TaskTransition) Validate() error {
	if err := t.Base.Validate(); err != nil {
		return err
	}
	if t.TaskID == "" || t.ActorID == "" || t.CommandID == "" {
		return errors.New("task transition requires task, actor, and command")
	}
	return ValidateTaskTransition(t.From, t.To)
}

type Attention struct {
	Base
	Kind           string     `json:"kind"`
	Severity       string     `json:"severity"`
	Title          string     `json:"title"`
	Detail         string     `json:"detail,omitempty"`
	ResourceType   string     `json:"resource_type,omitempty"`
	ResourceID     string     `json:"resource_id,omitempty"`
	Status         string     `json:"status"`
	OwnerID        string     `json:"owner_id,omitempty"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
}

type Situation struct {
	Base
	Kind          string     `json:"kind"`
	Fingerprint   string     `json:"fingerprint"`
	Status        string     `json:"status"`
	Severity      string     `json:"severity"`
	Summary       string     `json:"summary"`
	Detail        string     `json:"detail,omitempty"`
	MaterialHash  string     `json:"material_hash"`
	Occurrences   int64      `json:"occurrences"`
	FirstObserved time.Time  `json:"first_observed_at"`
	LastObserved  time.Time  `json:"last_observed_at"`
	ResolvedAt    *time.Time `json:"resolved_at,omitempty"`
}

type Policy struct {
	Base
	Name     string          `json:"name"`
	Kind     string          `json:"kind"`
	Status   string          `json:"status"`
	Priority int             `json:"priority"`
	Rule     json.RawMessage `json:"rule"`
}

type Deployment struct {
	Base
	Environment string     `json:"environment"`
	HostID      string     `json:"host_id"`
	Release     string     `json:"release"`
	Commit      string     `json:"commit"`
	Status      string     `json:"status"`
	RollbackOf  string     `json:"rollback_of,omitempty"`
	DeployedAt  *time.Time `json:"deployed_at,omitempty"`
}

type Evidence struct {
	Base
	ScopeType string          `json:"scope_type"`
	TaskID    string          `json:"task_id,omitempty"`
	RunID     string          `json:"run_id,omitempty"`
	Kind      string          `json:"kind"`
	Summary   string          `json:"summary"`
	Source    Source          `json:"source"`
	Digest    string          `json:"digest,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
}

type Artifact struct {
	Base
	ScopeType string `json:"scope_type"`
	TaskID    string `json:"task_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	URI       string `json:"uri"`
	Digest    string `json:"digest,omitempty"`
	Source    Source `json:"source"`
}

type Approval struct {
	Base
	Kind                  string          `json:"kind"`
	Status                string          `json:"status"`
	ResourceType          string          `json:"resource_type"`
	ResourceID            string          `json:"resource_id"`
	RequestedBy           string          `json:"requested_by"`
	DecidedBy             string          `json:"decided_by,omitempty"`
	Reason                string          `json:"reason"`
	DecisionNote          string          `json:"decision_note,omitempty"`
	DecisionAt            *time.Time      `json:"decision_at,omitempty"`
	CommandDigest         string          `json:"command_digest"`
	CommandVersion        int             `json:"command_version"`
	ExpectedTargetVersion int64           `json:"expected_target_version"`
	ExpiresAt             time.Time       `json:"expires_at"`
	ConsumedAt            *time.Time      `json:"consumed_at,omitempty"`
	ConsumedBy            string          `json:"consumed_by,omitempty"`
	Context               json.RawMessage `json:"context,omitempty"`
}

type Intervention struct {
	Base
	RunID      string     `json:"run_id"`
	Action     string     `json:"action"`
	Status     string     `json:"status"`
	Message    string     `json:"message,omitempty"`
	ActorID    string     `json:"actor_id"`
	ApprovalID string     `json:"approval_id,omitempty"`
	ExecutedAt *time.Time `json:"executed_at,omitempty"`
}

type ChatMessage struct {
	Base
	SessionID string          `json:"session_id,omitempty"`
	RunID     string          `json:"run_id,omitempty"`
	AuthorID  string          `json:"author_id"`
	Role      string          `json:"role"`
	Body      string          `json:"body"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

type Budget struct {
	Base
	Name            string `json:"name"`
	ScopeType       string `json:"scope_type"`
	ScopeID         string `json:"scope_id"`
	EnforcementMode string `json:"enforcement_mode"`
	Status          string `json:"status"`
	TokenLimit      int64  `json:"token_limit,omitempty"`
	TokenUsed       int64  `json:"token_used,omitempty"`
	CostLimitCents  int64  `json:"cost_limit_cents,omitempty"`
	CostUsedCents   int64  `json:"cost_used_cents,omitempty"`
	TimeLimitSec    int64  `json:"time_limit_seconds,omitempty"`
	TimeUsedSec     int64  `json:"time_used_seconds,omitempty"`
	ConcurrentLimit int64  `json:"concurrent_limit,omitempty"`
	ConcurrentUsed  int64  `json:"concurrent_used,omitempty"`
}

type Claim struct {
	Base
	TaskID     string `json:"task_id"`
	AgentID    string `json:"agent_id"`
	Status     string `json:"status"`
	ReviewNote string `json:"review_note,omitempty"`
	ReviewerID string `json:"reviewer_id,omitempty"`
}

type AuditRecord struct {
	Base
	ActorID      string          `json:"actor_id"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	CommandID    string          `json:"command_id,omitempty"`
	Outcome      string          `json:"outcome"`
	RemoteAddr   string          `json:"remote_addr,omitempty"`
	Details      json.RawMessage `json:"details,omitempty"`
}

type Source struct {
	System      string `json:"system"`
	ExternalID  string `json:"external_id,omitempty"`
	ExternalURL string `json:"external_url,omitempty"`
}

type Event struct {
	ID             string          `json:"id"`
	Cursor         int64           `json:"cursor"`
	ProjectID      string          `json:"project_id"`
	Type           string          `json:"type"`
	SubjectType    ResourceType    `json:"subject_type"`
	SubjectID      string          `json:"subject_id"`
	ActorID        string          `json:"actor_id"`
	CommandID      string          `json:"command_id,omitempty"`
	CorrelationID  string          `json:"correlation_id,omitempty"`
	CausationID    string          `json:"causation_id,omitempty"`
	OccurredAt     time.Time       `json:"occurred_at"`
	SchemaVersion  int             `json:"schema_version"`
	SourceSystem   string          `json:"source_system,omitempty"`
	SourceEventID  string          `json:"source_event_id,omitempty"`
	SourceSequence int64           `json:"source_sequence,omitempty"`
	Payload        json.RawMessage `json:"payload"`
}

func NewBase(id, projectID string, at time.Time) Base {
	return Base{ID: id, ProjectID: projectID, Version: 1, CreatedAt: at.UTC(), UpdatedAt: at.UTC()}
}

func DecodeResource(kind ResourceType, raw json.RawMessage) (any, error) {
	var target any
	switch kind {
	case ResourceAgent:
		target = &Agent{}
	case ResourceRun:
		target = &Run{}
	case ResourceSession:
		target = &Session{}
	case ResourceTask:
		target = &Task{}
	case ResourceAttention:
		target = &Attention{}
	case ResourceEvidence:
		target = &Evidence{}
	case ResourceArtifact:
		target = &Artifact{}
	case ResourceApproval:
		target = &Approval{}
	case ResourceIntervention:
		target = &Intervention{}
	case ResourceChatMessage:
		target = &ChatMessage{}
	case ResourceBudget:
		target = &Budget{}
	case ResourceClaim:
		target = &Claim{}
	case ResourceAudit:
		target = &AuditRecord{}
	case ResourceOrganization:
		target = &Organization{}
	case ResourceHuman:
		target = &Human{}
	case ResourceProject:
		target = &Project{}
	case ResourceHost:
		target = &Host{}
	case ResourceAgentInstance:
		target = &AgentInstance{}
	case ResourceTaskTransition:
		target = &TaskTransition{}
	case ResourceSituation:
		target = &Situation{}
	case ResourcePolicy:
		target = &Policy{}
	case ResourceDeployment:
		target = &Deployment{}
	default:
		return nil, fmt.Errorf("unsupported resource type %q", kind)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return nil, fmt.Errorf("decode %s: %w", kind, err)
	}
	return target, nil
}

func ReconcileSituation(current *Situation, observed Situation, now time.Time) (Situation, bool, error) {
	if observed.Fingerprint == "" || observed.Kind == "" || observed.MaterialHash == "" {
		return Situation{}, false, errors.New("situation kind, fingerprint, and material_hash are required")
	}
	now = now.UTC()
	if current == nil {
		observed.Status = "open"
		observed.Occurrences = 1
		observed.FirstObserved, observed.LastObserved = now, now
		observed.ResolvedAt = nil
		return observed, true, nil
	}
	next := *current
	next.Occurrences++
	next.LastObserved = now
	materialChange := observed.MaterialHash != current.MaterialHash
	if materialChange {
		next.Kind, next.Severity, next.Summary, next.Detail = observed.Kind, observed.Severity, observed.Summary, observed.Detail
		next.MaterialHash = observed.MaterialHash
		next.Status, next.ResolvedAt = "open", nil
	}
	next.Version++
	next.UpdatedAt = now
	return next, materialChange, nil
}

func ResolveSituation(current Situation, now time.Time) (Situation, error) {
	if current.Status != "open" {
		return Situation{}, fmt.Errorf("cannot resolve situation in %q state", current.Status)
	}
	now = now.UTC()
	current.Status, current.ResolvedAt = "resolved", &now
	current.Version++
	current.UpdatedAt = now
	return current, nil
}
