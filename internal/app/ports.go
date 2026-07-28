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
	ID           string
	Capabilities map[string]struct{}
}

func (a Actor) Can(capability string) bool {
	_, ok := a.Capabilities[capability]
	return ok
}
