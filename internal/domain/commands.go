package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

type Command struct {
	ID              string          `json:"id"`
	ProjectID       string          `json:"project_id"`
	Type            string          `json:"type"`
	SubjectType     ResourceType    `json:"subject_type"`
	SubjectID       string          `json:"subject_id"`
	ActorID         string          `json:"actor_id"`
	ExpectedVersion int64           `json:"expected_version,omitempty"`
	IdempotencyKey  string          `json:"idempotency_key"`
	CorrelationID   string          `json:"correlation_id,omitempty"`
	Payload         json.RawMessage `json:"payload"`
	RequestedAt     time.Time       `json:"requested_at"`
	RemoteAddr      string          `json:"remote_addr,omitempty"`
	ApprovalID      string          `json:"approval_id,omitempty"`
	ApprovalDigest  string          `json:"approval_digest,omitempty"`
}

func (c Command) Validate() error {
	switch {
	case strings.TrimSpace(c.ID) == "":
		return errors.New("command id is required")
	case strings.TrimSpace(c.ProjectID) == "":
		return errors.New("project_id is required")
	case strings.TrimSpace(c.Type) == "":
		return errors.New("command type is required")
	case strings.TrimSpace(c.SubjectID) == "":
		return errors.New("subject_id is required")
	case strings.TrimSpace(c.ActorID) == "":
		return errors.New("actor_id is required")
	case strings.TrimSpace(c.IdempotencyKey) == "":
		return errors.New("idempotency_key is required")
	case c.ExpectedVersion < 0:
		return errors.New("expected_version cannot be negative")
	}
	return nil
}

var taskTransitions = map[string][]string{
	"inbox":     {"ready", "cancelled", "failed"},
	"ready":     {"working", "blocked", "cancelled", "failed"},
	"working":   {"review", "blocked", "cancelled", "failed"},
	"review":    {"working", "completed", "blocked", "cancelled", "failed"},
	"completed": {"archived", "reopened"},
	"reopened":  {"ready", "cancelled", "failed"},
	"blocked":   {"ready", "working", "cancelled", "failed"},
	"failed":    {"reopened", "cancelled"},
	"archived":  {},
	"cancelled": {},
}

func ValidateTaskTransition(from, to string) error {
	if _, known := taskTransitions[from]; !known {
		return fmt.Errorf("unknown current task status %q", from)
	}
	if !slices.Contains(taskTransitions[from], to) {
		return fmt.Errorf("task transition %q -> %q is not allowed", from, to)
	}
	return nil
}

func RequiresApproval(action string) bool {
	switch action {
	case "cancel", "delete", "destroy", "force_push", "publish", "deploy_production", "rotate_secret":
		return true
	default:
		return false
	}
}
