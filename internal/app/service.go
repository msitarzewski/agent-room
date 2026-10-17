package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/msitarzewski/agent-room/internal/domain"
)

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

type randomIDs struct{}

func (randomIDs) New() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("secure random source unavailable: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	dst := make([]byte, 36)
	hex.Encode(dst[0:8], b[0:4])
	dst[8] = '-'
	hex.Encode(dst[9:13], b[4:6])
	dst[13] = '-'
	hex.Encode(dst[14:18], b[6:8])
	dst[18] = '-'
	hex.Encode(dst[19:23], b[8:10])
	dst[23] = '-'
	hex.Encode(dst[24:36], b[10:16])
	return string(dst)
}

type Service struct {
	repo       Repository
	publisher  Publisher
	clock      Clock
	ids        IDGenerator
	controller RunController
}

func NewService(repo Repository, publisher Publisher) *Service {
	if publisher == nil {
		publisher = discardPublisher{}
	}
	return &Service{repo: repo, publisher: publisher, clock: systemClock{}, ids: randomIDs{}}
}

func NewServiceWithController(repo Repository, publisher Publisher, controller RunController) *Service {
	service := NewService(repo, publisher)
	service.controller = controller
	return service
}

func NewServiceWithDependencies(repo Repository, publisher Publisher, clock Clock, ids IDGenerator) *Service {
	if publisher == nil {
		publisher = discardPublisher{}
	}
	return &Service{repo: repo, publisher: publisher, clock: clock, ids: ids}
}

type discardPublisher struct{}

func (discardPublisher) Publish(domain.Event) {}

func (s *Service) Get(ctx context.Context, actor Actor, projectID string, kind domain.ResourceType, id string) (json.RawMessage, error) {
	if !canRead(actor, kind) {
		return nil, ErrDenied
	}
	raw, err := s.repo.Get(ctx, projectID, kind, id)
	if err == nil && kind == domain.ResourceCoordinationRequest && !CoordinationVisible(actor, raw) {
		return nil, ErrNotFound
	}
	if err == nil && kind == domain.ResourceAudit {
		return redactCoordinationAudit(raw)
	}
	return raw, err
}

// Existing audit rows remain immutable. Request content is available only through
// its recipient-authorized resource, never through the general audit surface.
func redactCoordinationAudit(raw json.RawMessage) (json.RawMessage, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, err
	}
	var kind string
	if value, ok := document["resource_type"]; ok {
		if err := json.Unmarshal(value, &kind); err != nil {
			return nil, err
		}
	}
	if kind != string(domain.ResourceCoordinationRequest) {
		return raw, nil
	}
	delete(document, "details")
	return json.Marshal(document)
}

func (s *Service) NewID() string { return s.ids.New() }

func (s *Service) Health(ctx context.Context) error { return s.repo.Health(ctx) }

func (s *Service) ComponentHealth(ctx context.Context) (map[string]any, error) {
	type provider interface {
		ComponentHealth(context.Context) (map[string]any, error)
	}
	if health, ok := s.repo.(provider); ok {
		return health.ComponentHealth(ctx)
	}
	if err := s.repo.Health(ctx); err != nil {
		return nil, err
	}
	return map[string]any{"database": map[string]string{"status": "ok"}}, nil
}

func (s *Service) List(ctx context.Context, actor Actor, projectID string, kind domain.ResourceType, cursor string, limit int) (Page, error) {
	if !canRead(actor, kind) {
		return Page{}, ErrDenied
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	var page Page
	var err error
	if kind == domain.ResourceCoordinationRequest && !actor.Can("resource:read") {
		provider, ok := s.repo.(interface {
			ListCoordination(context.Context, string, Actor, string, int) (Page, error)
		})
		if !ok {
			return Page{}, ErrUnsupported
		}
		page, err = provider.ListCoordination(ctx, projectID, actor, cursor, limit)
	} else {
		page, err = s.repo.List(ctx, projectID, kind, cursor, limit)
	}
	if err == nil && kind == domain.ResourceAudit {
		for i, raw := range page.Items {
			safe, redactErr := redactCoordinationAudit(raw)
			if redactErr != nil {
				return Page{}, redactErr
			}
			page.Items[i] = safe
		}
	}
	if page.Items == nil {
		page.Items = make([]json.RawMessage, 0)
	}
	return page, err
}

func (s *Service) Events(ctx context.Context, actor Actor, projectID string, after int64, limit int) (EventPage, error) {
	if !actor.Can("event:read") && !actor.Can("resource:read") {
		return EventPage{}, ErrDenied
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	page, err := s.repo.Events(ctx, projectID, after, limit)
	if page.Items != nil {
		visible := make([]domain.Event, 0, len(page.Items))
		for _, event := range page.Items {
			if CoordinationEventVisible(actor, event) {
				visible = append(visible, event)
			}
		}
		page.Items = visible
	}
	if page.Items == nil {
		page.Items = make([]domain.Event, 0)
	}
	return page, err
}

// Workspace fails closed when the actor cannot read any of its resource kinds.
// A partial graph could imply that hidden dependencies or approvals do not exist.
func (s *Service) Workspace(ctx context.Context, actor Actor, projectID string) (WorkspaceSnapshot, error) {
	if strings.TrimSpace(projectID) == "" {
		return WorkspaceSnapshot{}, Invalid("project_id is required")
	}
	for _, kind := range []domain.ResourceType{
		domain.ResourceTask, domain.ResourceAgent, domain.ResourceRun, domain.ResourceAttention,
		domain.ResourceEvidence, domain.ResourceArtifact, domain.ResourceClaim, domain.ResourceApproval, domain.ResourceCoordinationRequest,
	} {
		if !canRead(actor, kind) {
			return WorkspaceSnapshot{}, ErrDenied
		}
	}
	provider, ok := s.repo.(WorkspaceRepository)
	if !ok {
		return WorkspaceSnapshot{}, ErrUnsupported
	}
	snapshot, err := provider.Workspace(ctx, projectID)
	if err == nil && !actor.Can("resource:read") {
		visible := make([]json.RawMessage, 0)
		for _, raw := range snapshot.Resources.Requests {
			if CoordinationVisible(actor, raw) {
				visible = append(visible, raw)
			}
		}
		snapshot.Resources.Requests = visible
	}
	return snapshot, err
}

func (s *Service) Overview(ctx context.Context, actor Actor, projectID string) (map[string]any, error) {
	if !actor.Can("overview:read") && !actor.Can("resource:read") {
		return nil, ErrDenied
	}
	if provider, ok := s.repo.(interface {
		OverviewForActor(context.Context, string, Actor) (map[string]any, error)
	}); ok {
		return provider.OverviewForActor(ctx, projectID, actor)
	}
	return s.repo.Overview(ctx, projectID)
}

func (s *Service) Brief(ctx context.Context, actor Actor, projectID string, after int64) (map[string]any, error) {
	if !actor.Can("overview:read") && !actor.Can("resource:read") {
		return nil, ErrDenied
	}
	reviewedCursor := int64(0)
	if provider, ok := s.repo.(interface {
		BriefCursor(context.Context, string, string) (int64, error)
	}); ok {
		var err error
		reviewedCursor, err = provider.BriefCursor(ctx, projectID, actor.ID)
		if err != nil {
			return nil, err
		}
	}
	if after < 0 {
		after = reviewedCursor
	}
	events, err := s.Events(ctx, actor, projectID, after, 1000)
	if err != nil {
		return nil, err
	}
	attention, err := s.List(ctx, actor, projectID, domain.ResourceAttention, "", 100)
	if err != nil {
		return nil, err
	}
	approvals, err := s.List(ctx, actor, projectID, domain.ResourceApproval, "", 100)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	through := after
	for _, event := range events.Items {
		counts[event.Type]++
		if event.Cursor > through {
			through = event.Cursor
		}
	}
	openAttention := filterDocuments(attention.Items, func(value map[string]any) bool {
		status, _ := value["status"].(string)
		return status == "open"
	})
	pendingApprovals := filterDocuments(approvals.Items, func(value map[string]any) bool {
		status, _ := value["status"].(string)
		return status == "pending"
	})
	recommendations := make([]string, 0, 3)
	if len(pendingApprovals) > 0 {
		recommendations = append(recommendations, "Review pending approvals")
	}
	if len(openAttention) > 0 {
		recommendations = append(recommendations, "Triage open attention items")
	}
	if len(events.Items) == 1000 {
		recommendations = append(recommendations, "Continue from the returned cursor; more than 1,000 events changed")
	}
	return map[string]any{
		"project_id": projectID, "reviewed_cursor": reviewedCursor, "from_cursor": after, "through_cursor": through,
		"event_counts": counts, "events": events.Items, "open_attention": openAttention,
		"pending_approvals": pendingApprovals, "recommended_actions": recommendations,
		"generated_at": s.clock.Now(),
	}, nil
}

func (s *Service) AcknowledgeBrief(ctx context.Context, actor Actor, projectID, idem string, expected, through int64) (bool, error) {
	if !actor.Can("overview:read") && !actor.Can("resource:read") {
		return false, ErrDenied
	}
	if expected < 0 || through < 0 {
		return false, Invalid("brief cursors cannot be negative")
	}
	provider, ok := s.repo.(interface {
		AcknowledgeBrief(context.Context, string, string, string, string, int64, int64, time.Time) (bool, error)
	})
	if !ok {
		return false, ErrUnsupported
	}
	return provider.AcknowledgeBrief(ctx, projectID, actor.ID, s.ids.New(), idem, expected, through, s.clock.Now())
}

func filterDocuments(documents []json.RawMessage, keep func(map[string]any) bool) []json.RawMessage {
	filtered := make([]json.RawMessage, 0)
	for _, document := range documents {
		var value map[string]any
		if json.Unmarshal(document, &value) == nil && keep(value) {
			filtered = append(filtered, document)
		}
	}
	return filtered
}

func (s *Service) Create(ctx context.Context, actor Actor, kind domain.ResourceType, projectID, idempotencyKey string, value any, remoteAddr string) (CommandResult, error) {
	if kind == domain.ResourceCoordinationRequest {
		return CommandResult{}, Invalid("coordination requests require typed create/respond/resolve/cancel commands")
	}
	if !actor.Can("resource:write") && !actor.Can(string(kind)+":write") &&
		!(kind == domain.ResourceApproval && actor.Can("approval:request")) {
		return CommandResult{}, s.recordRejection(ctx, actor, projectID, "create."+string(kind), string(kind), "", idempotencyKey, ErrDenied)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return CommandResult{}, fmt.Errorf("encode resource: %w", err)
	}
	var base struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &base); err != nil {
		return CommandResult{}, err
	}
	raw, err = s.validateResourceLinks(ctx, projectID, kind, raw)
	if err != nil {
		return CommandResult{}, err
	}
	if base.ID == "" {
		return CommandResult{}, Invalid("resource id is required")
	}
	cmd := domain.Command{
		ID: s.ids.New(), ProjectID: projectID, Type: "create." + string(kind),
		SubjectType: kind, SubjectID: base.ID, ActorID: actor.ID,
		IdempotencyKey: idempotencyKey, RequestedAt: s.clock.Now(), RemoteAddr: remoteAddr,
		Payload: raw,
	}
	return s.execute(ctx, cmd, func(current json.RawMessage) (json.RawMessage, string, error) {
		if len(current) != 0 {
			return nil, "", ErrVersionConflict
		}
		return normalizeBase(raw, base.ID, projectID, 1, s.clock.Now()), string(kind) + ".created", nil
	})
}

func (s *Service) validateResourceLinks(ctx context.Context, projectID string, kind domain.ResourceType, raw json.RawMessage) (json.RawMessage, error) {
	if kind != domain.ResourceArtifact && kind != domain.ResourceEvidence {
		return raw, nil
	}
	var links struct {
		TaskID string `json:"task_id"`
		RunID  string `json:"run_id"`
	}
	if err := json.Unmarshal(raw, &links); err != nil {
		return nil, err
	}
	// Resources have no delete command in the founding release, so same-project
	// existence checks cannot race with target deletion.
	if links.TaskID != "" {
		if _, err := s.repo.Get(ctx, projectID, domain.ResourceTask, links.TaskID); err != nil {
			return nil, Invalid("task_id must reference an existing task in the same project")
		}
	}
	if links.RunID != "" {
		if _, err := s.repo.Get(ctx, projectID, domain.ResourceRun, links.RunID); err != nil {
			return nil, Invalid("run_id must reference an existing run in the same project")
		}
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, err
	}
	scope := "project"
	if links.TaskID != "" {
		scope = "task"
	} else if links.RunID != "" {
		scope = "run"
	}
	document["scope_type"] = scope
	return json.Marshal(document)
}

func (s *Service) TransitionTask(ctx context.Context, actor Actor, projectID, taskID, status, reason, idem string, expected int64, remoteAddr string) (CommandResult, error) {
	if !actor.Can("task:transition") {
		return CommandResult{}, s.recordRejection(ctx, actor, projectID, "transition.task", string(domain.ResourceTask), taskID, idem, ErrDenied)
	}
	payload, _ := json.Marshal(map[string]string{"status": status, "reason": reason})
	cmd := domain.Command{
		ID: s.ids.New(), ProjectID: projectID, Type: "transition.task", SubjectType: domain.ResourceTask,
		SubjectID: taskID, ActorID: actor.ID, ExpectedVersion: expected, IdempotencyKey: idem,
		RequestedAt: s.clock.Now(), RemoteAddr: remoteAddr, Payload: payload,
	}
	return s.execute(ctx, cmd, func(current json.RawMessage) (json.RawMessage, string, error) {
		var task domain.Task
		if len(current) == 0 {
			return nil, "", ErrNotFound
		}
		if err := json.Unmarshal(current, &task); err != nil {
			return nil, "", err
		}
		if err := domain.ValidateTaskTransition(task.Status, status); err != nil {
			return nil, "", Invalid(err.Error())
		}
		task.Status = status
		task.BlockedReason = ""
		if status == "blocked" {
			if strings.TrimSpace(reason) == "" {
				return nil, "", Invalid("blocked task requires a reason")
			}
			task.BlockedReason = reason
		}
		task.Version++
		task.UpdatedAt = s.clock.Now()
		next, _ := json.Marshal(task)
		return next, "task." + status, nil
	})
}

func (s *Service) ClaimTask(ctx context.Context, actor Actor, projectID, taskID, idem string, expected int64, remoteAddr string) (CommandResult, error) {
	if !actor.Can("task:claim") {
		return CommandResult{}, s.recordRejection(ctx, actor, projectID, "claim.task", string(domain.ResourceTask), taskID, idem, ErrDenied)
	}
	payload, _ := json.Marshal(map[string]string{"owner_id": actor.ID})
	cmd := domain.Command{
		ID: s.ids.New(), ProjectID: projectID, Type: "claim.task", SubjectType: domain.ResourceTask,
		SubjectID: taskID, ActorID: actor.ID, ExpectedVersion: expected, IdempotencyKey: idem,
		RequestedAt: s.clock.Now(), RemoteAddr: remoteAddr, Payload: payload,
	}
	return s.execute(ctx, cmd, func(current json.RawMessage) (json.RawMessage, string, error) {
		var task domain.Task
		if len(current) == 0 {
			return nil, "", ErrNotFound
		}
		if err := json.Unmarshal(current, &task); err != nil {
			return nil, "", err
		}
		if task.OwnerID != "" && task.OwnerID != actor.ID {
			return nil, "", ErrVersionConflict
		}
		task.OwnerID = actor.ID
		task.Version++
		task.UpdatedAt = s.clock.Now()
		next, _ := json.Marshal(task)
		return next, "task.claimed", nil
	})
}

func (s *Service) RequestTaskReview(ctx context.Context, actor Actor, projectID, taskID, reason, idem string, expected int64, remoteAddr string) (CommandResult, error) {
	if !actor.Can("task:review") {
		return CommandResult{}, s.recordRejection(ctx, actor, projectID, "request_review.task", string(domain.ResourceTask), taskID, idem, ErrDenied)
	}
	payload, _ := json.Marshal(map[string]string{"reason": reason})
	cmd := domain.Command{
		ID: s.ids.New(), ProjectID: projectID, Type: "request_review.task", SubjectType: domain.ResourceTask,
		SubjectID: taskID, ActorID: actor.ID, ExpectedVersion: expected, IdempotencyKey: idem,
		RequestedAt: s.clock.Now(), RemoteAddr: remoteAddr, Payload: payload,
	}
	return s.execute(ctx, cmd, func(current json.RawMessage) (json.RawMessage, string, error) {
		var task domain.Task
		if len(current) == 0 {
			return nil, "", ErrNotFound
		}
		if err := json.Unmarshal(current, &task); err != nil {
			return nil, "", err
		}
		if task.OwnerID != actor.ID && !actor.Can("resource:write") {
			return nil, "", ErrDenied
		}
		if err := domain.ValidateTaskTransition(task.Status, "review"); err != nil {
			return nil, "", Invalid(err.Error())
		}
		task.Status = "review"
		task.ReviewState = "requested"
		task.Version++
		task.UpdatedAt = s.clock.Now()
		next, _ := json.Marshal(task)
		return next, "task.review_requested", nil
	})
}

func (s *Service) validateRuntimeObservation(ctx context.Context, actor Actor, event *domain.Event, batch []domain.Event) error {
	if event.SourceSequence <= 0 {
		return Invalid("runtime observations require a positive source_sequence")
	}
	var agentID, nativeID, status string
	var observedAt time.Time
	decoder := json.NewDecoder(bytes.NewReader(event.Payload))
	decoder.DisallowUnknownFields()
	switch event.Type {
	case "agent.observed":
		var input domain.AgentObservation
		if event.SubjectType != domain.ResourceAgent || decoder.Decode(&input) != nil {
			return Invalid("invalid typed agent observation")
		}
		agentID, nativeID, status, observedAt = input.AgentID, input.InstanceID, input.Status, input.ObservedAt
		if strings.TrimSpace(input.DisplayName) == "" || len(input.DisplayName) > 256 || (status != "active" && status != "idle" && status != "blocked" && status != "disconnected") {
			return Invalid("invalid agent display_name or status")
		}
		event.SubjectID = actor.ID
	case "session.observed":
		var input domain.SessionObservation
		if event.SubjectType != domain.ResourceSession || decoder.Decode(&input) != nil {
			return Invalid("invalid typed session observation")
		}
		agentID, nativeID, status, observedAt = input.AgentID, input.SessionID, input.Status, input.ObservedAt
		if status != "active" && status != "idle" && status != "closed" {
			return Invalid("invalid session status")
		}
		event.SubjectID = domain.ObservedRuntimeID(event.ProjectID, event.SourceSystem, actor.ID, "session", nativeID)
	case "run.observed":
		var input domain.RunObservation
		if event.SubjectType != domain.ResourceRun || decoder.Decode(&input) != nil {
			return Invalid("invalid typed run observation")
		}
		agentID, nativeID, status, observedAt = input.AgentID, input.RunID, input.Status, input.ObservedAt
		if status != "running" && status != "idle" && status != "blocked" && status != "completed" && status != "failed" && status != "cancelled" {
			return Invalid("invalid run status")
		}
		if len(input.Summary) > 4096 || len(input.SessionID) > 512 || len(input.TaskID) > 512 {
			return Invalid("run references and summary must be bounded")
		}
		if input.TaskID != "" {
			if _, err := s.repo.Get(ctx, event.ProjectID, domain.ResourceTask, input.TaskID); err != nil {
				return Invalid("run task must be in the same project")
			}
		}
		if input.SessionID != "" {
			id := domain.ObservedRuntimeID(event.ProjectID, event.SourceSystem, actor.ID, "session", input.SessionID)
			found := false
			if raw, err := s.repo.Get(ctx, event.ProjectID, domain.ResourceSession, id); err == nil {
				var session domain.Session
				if json.Unmarshal(raw, &session) == nil && session.AgentID == actor.ID {
					found = true
				}
			}
			for _, prior := range batch {
				if prior.Type == "session.observed" && prior.SubjectID == id && prior.ProjectID == event.ProjectID && prior.SourceSystem == event.SourceSystem {
					found = true
				}
			}
			if !found {
				return Invalid("run session must be this actor's same-project session")
			}
		}
		event.SubjectID = domain.ObservedRuntimeID(event.ProjectID, event.SourceSystem, actor.ID, "run", nativeID)
	}
	if agentID != actor.ID {
		return ErrDenied
	}
	if event.Type != "agent.observed" {
		found := false
		if raw, err := s.repo.Get(ctx, event.ProjectID, domain.ResourceAgent, actor.ID); err == nil {
			var agent domain.Agent
			if json.Unmarshal(raw, &agent) == nil && agent.Source.System == event.SourceSystem {
				found = true
			}
		}
		for _, prior := range batch {
			if prior.Type == "agent.observed" && prior.ProjectID == event.ProjectID && prior.SubjectID == actor.ID && prior.SourceSystem == event.SourceSystem {
				found = true
			}
		}
		if !found {
			return Invalid("runtime session/run requires this actor's same-project registered agent")
		}
	}
	if strings.TrimSpace(nativeID) == "" || len(nativeID) > 512 || observedAt.IsZero() {
		return Invalid("runtime native identity and observed_at are required")
	}
	if !event.OccurredAt.IsZero() && !event.OccurredAt.Equal(observedAt) {
		return Invalid("occurred_at must equal observed_at")
	}
	event.OccurredAt = observedAt
	return nil
}

func actorType(actor Actor) string {
	if actor.Human {
		return "human"
	}
	return "agent"
}

// CoordinationVisible applies recipient isolation to snapshots, lists and event payloads.
func CoordinationVisible(actor Actor, raw json.RawMessage) bool {
	if actor.Can("resource:read") {
		return true
	}
	if !actor.Can("coordination:read") {
		return false
	}
	var request domain.CoordinationRequest
	if json.Unmarshal(raw, &request) != nil {
		return false
	}
	return (request.OwnerID == actor.ID && request.OwnerType == actorType(actor)) || (request.Recipient.ID == actor.ID && request.Recipient.Type == actorType(actor))
}

func CoordinationEventVisible(actor Actor, event domain.Event) bool {
	if event.SubjectType != domain.ResourceCoordinationRequest {
		return true
	}
	var payload struct {
		Resource json.RawMessage `json:"resource"`
	}
	if json.Unmarshal(event.Payload, &payload) != nil {
		return false
	}
	return CoordinationVisible(actor, payload.Resource)
}

func (s *Service) coordinationLinks(ctx context.Context, projectID, taskID string, evidenceIDs []string) error {
	if len(evidenceIDs) > 100 {
		return Invalid("at most 100 evidence references are allowed")
	}
	if taskID != "" {
		if _, err := s.repo.Get(ctx, projectID, domain.ResourceTask, taskID); err != nil {
			return Invalid("task_id must reference the same project")
		}
	}
	seen := map[string]bool{}
	for _, id := range evidenceIDs {
		if id == "" || len(id) > 512 || seen[id] {
			return Invalid("evidence_ids must be distinct bounded identifiers")
		}
		seen[id] = true
		if _, err := s.repo.Get(ctx, projectID, domain.ResourceEvidence, id); err != nil {
			return Invalid("evidence_ids must reference the same project")
		}
	}
	return nil
}

func (s *Service) CreateCoordination(ctx context.Context, actor Actor, projectID, idem string, input CreateCoordinationInput, remoteAddr string) (CommandResult, error) {
	if !actor.Can("coordination:create") && !actor.Can("resource:write") {
		return CommandResult{}, s.recordRejection(ctx, actor, projectID, "create.coordination_request", "coordination_request", input.ID, idem, ErrDenied)
	}
	if strings.TrimSpace(input.ID) == "" || len(input.ID) > 512 || strings.TrimSpace(input.Title) == "" || len(input.Title) > 4096 || strings.TrimSpace(input.Body) == "" || len(input.Body) > 32768 || len(input.EscalationReason) > 4096 {
		return CommandResult{}, Invalid("request requires bounded id, title and body")
	}
	if input.Kind != "help" && input.Kind != "context" && input.Kind != "handoff" && input.Kind != "review" {
		return CommandResult{}, Invalid("request kind must be help, context, handoff or review")
	}
	if (input.Recipient.Type != "agent" && input.Recipient.Type != "human") || input.Recipient.ID == "" || len(input.Recipient.ID) > 512 {
		return CommandResult{}, Invalid("named agent or human recipient is required")
	}
	if input.Recipient.Type == "human" && strings.TrimSpace(input.EscalationReason) == "" {
		return CommandResult{}, Invalid("human recipient requires an explicit escalation_reason")
	}
	if input.EscalationReason != "" && input.Recipient.Type != "human" {
		return CommandResult{}, Invalid("human escalation requires a human recipient")
	}
	if input.Recipient.ID == actor.ID && input.Recipient.Type == actorType(actor) {
		return CommandResult{}, Invalid("request recipient must be another participant")
	}
	if err := s.coordinationLinks(ctx, projectID, input.TaskID, input.EvidenceIDs); err != nil {
		return CommandResult{}, err
	}
	provider, ok := s.repo.(interface {
		CoordinationRecipientExists(context.Context, string, domain.CoordinationRecipient) (bool, error)
	})
	if !ok {
		return CommandResult{}, ErrUnsupported
	}
	exists, err := provider.CoordinationRecipientExists(ctx, projectID, input.Recipient)
	if err != nil {
		return CommandResult{}, err
	}
	if !exists {
		return CommandResult{}, Invalid("recipient must be registered in this project")
	}
	request := domain.CoordinationRequest{Base: domain.Base{ID: input.ID}, Kind: input.Kind, Title: input.Title, Body: input.Body, OwnerID: actor.ID, OwnerType: actorType(actor), Recipient: input.Recipient, TaskID: input.TaskID, EvidenceIDs: input.EvidenceIDs, Status: "open", EscalationReason: input.EscalationReason}
	if request.EvidenceIDs == nil {
		request.EvidenceIDs = []string{}
	}
	raw, _ := json.Marshal(request)
	cmd := domain.Command{ID: s.ids.New(), ProjectID: projectID, Type: "create.coordination_request", SubjectType: domain.ResourceCoordinationRequest, SubjectID: input.ID, ActorID: actor.ID, IdempotencyKey: idem, RequestedAt: s.clock.Now(), RemoteAddr: remoteAddr, Payload: raw}
	return s.execute(ctx, cmd, func(current json.RawMessage) (json.RawMessage, string, error) {
		if len(current) != 0 {
			return nil, "", ErrVersionConflict
		}
		return normalizeBase(raw, input.ID, projectID, 1, s.clock.Now()), "coordination_request.created", nil
	})
}

func (s *Service) ActCoordination(ctx context.Context, actor Actor, projectID, id, action, idem string, expected int64, input CoordinationActionInput, remoteAddr string) (CommandResult, error) {
	if action != "respond" && action != "resolve" && action != "cancel" {
		return CommandResult{}, Invalid("unsupported coordination action")
	}
	if !actor.Can("coordination:"+action) && !actor.Can("resource:write") {
		return CommandResult{}, s.recordRejection(ctx, actor, projectID, action+".coordination_request", "coordination_request", id, idem, ErrDenied)
	}
	if expected < 1 {
		return CommandResult{}, Invalid("positive expected version is required")
	}
	if len(input.Body) > 32768 || len(input.Note) > 4096 || (action == "respond" && (strings.TrimSpace(input.Body) == "" || input.Note != "")) || (action != "respond" && (input.Body != "" || len(input.EvidenceIDs) != 0)) {
		return CommandResult{}, Invalid("invalid coordination action payload")
	}
	if err := s.coordinationLinks(ctx, projectID, "", input.EvidenceIDs); err != nil {
		return CommandResult{}, err
	}
	// Actor identity is part of the new command fingerprint, including retries.
	payload, _ := json.Marshal(struct {
		ActorID   string                  `json:"actor_id"`
		ActorType string                  `json:"actor_type"`
		Input     CoordinationActionInput `json:"input"`
	}{actor.ID, actorType(actor), input})
	cmd := domain.Command{ID: s.ids.New(), ProjectID: projectID, Type: action + ".coordination_request", SubjectType: domain.ResourceCoordinationRequest, SubjectID: id, ActorID: actor.ID, ExpectedVersion: expected, IdempotencyKey: idem, RequestedAt: s.clock.Now(), RemoteAddr: remoteAddr, Payload: payload}
	return s.execute(ctx, cmd, func(current json.RawMessage) (json.RawMessage, string, error) {
		if len(current) == 0 {
			return nil, "", ErrNotFound
		}
		var request domain.CoordinationRequest
		if err := json.Unmarshal(current, &request); err != nil {
			return nil, "", err
		}
		operator := actor.Can("resource:write")
		if action == "respond" {
			if request.Recipient.Type == "human" && !actor.Human {
				return nil, "", ErrDenied
			}
			if !operator && (request.Recipient.ID != actor.ID || request.Recipient.Type != actorType(actor)) {
				return nil, "", ErrDenied
			}
		} else if !operator && (request.OwnerID != actor.ID || request.OwnerType != actorType(actor)) {
			return nil, "", ErrDenied
		}
		if request.Status != "open" && request.Status != "responded" {
			return nil, "", Invalid("request is already closed")
		}
		if action == "respond" && request.Status != "open" {
			return nil, "", Invalid("request already has a response")
		}
		now := s.clock.Now()
		if action == "respond" {
			evidence := input.EvidenceIDs
			if evidence == nil {
				evidence = []string{}
			}
			request.Status = "responded"
			request.Response = &domain.CoordinationResponse{ActorID: actor.ID, Body: input.Body, EvidenceIDs: evidence, RespondedAt: now}
		} else {
			if action == "resolve" {
				request.Status = "resolved"
			} else {
				request.Status = "cancelled"
			}
			request.Resolution = &domain.CoordinationResolution{ActorID: actor.ID, Note: input.Note, ClosedAt: now}
		}
		request.Version++
		request.UpdatedAt = now
		raw, _ := json.Marshal(request)
		return raw, "coordination_request." + request.Status, nil
	})
}

func (s *Service) SetAttentionStatus(ctx context.Context, actor Actor, projectID, id, status, idem string, expected int64, remoteAddr string) (CommandResult, error) {
	if !actor.Can("attention:manage") {
		return CommandResult{}, ErrDenied
	}
	if status != "acknowledged" && status != "resolved" {
		return CommandResult{}, Invalid(fmt.Sprintf("unsupported attention status %q", status))
	}
	payload, _ := json.Marshal(map[string]string{"status": status})
	cmd := domain.Command{
		ID: s.ids.New(), ProjectID: projectID, Type: status + ".attention", SubjectType: domain.ResourceAttention,
		SubjectID: id, ActorID: actor.ID, ExpectedVersion: expected, IdempotencyKey: idem,
		RequestedAt: s.clock.Now(), RemoteAddr: remoteAddr, Payload: payload,
	}
	return s.execute(ctx, cmd, func(current json.RawMessage) (json.RawMessage, string, error) {
		var item domain.Attention
		if len(current) == 0 {
			return nil, "", ErrNotFound
		}
		if err := json.Unmarshal(current, &item); err != nil {
			return nil, "", err
		}
		now := s.clock.Now()
		item.Status, item.Version, item.UpdatedAt = status, item.Version+1, now
		if status == "acknowledged" {
			item.AcknowledgedAt = &now
		} else {
			item.ResolvedAt = &now
		}
		next, _ := json.Marshal(item)
		return next, "attention." + status, nil
	})
}

func (s *Service) DecideApproval(ctx context.Context, actor Actor, projectID, id, decision, note, idem string, expected int64, remoteAddr string) (CommandResult, error) {
	if !actor.Can("approval:decide") {
		return CommandResult{}, ErrDenied
	}
	if decision != "approved" && decision != "rejected" {
		return CommandResult{}, Invalid("decision must be approved or rejected")
	}
	payload, _ := json.Marshal(map[string]string{"decision": decision, "note": note})
	cmd := domain.Command{ID: s.ids.New(), ProjectID: projectID, Type: "decide.approval", SubjectType: domain.ResourceApproval, SubjectID: id, ActorID: actor.ID, ExpectedVersion: expected, IdempotencyKey: idem, RequestedAt: s.clock.Now(), RemoteAddr: remoteAddr, Payload: payload}
	return s.execute(ctx, cmd, func(current json.RawMessage) (json.RawMessage, string, error) {
		var item domain.Approval
		if len(current) == 0 {
			return nil, "", ErrNotFound
		}
		if err := json.Unmarshal(current, &item); err != nil {
			return nil, "", err
		}
		if item.Status != "pending" {
			return nil, "", Invalid(fmt.Sprintf("approval is already %s", item.Status))
		}
		now := s.clock.Now()
		if item.CommandVersion != 1 || !strings.HasPrefix(item.CommandDigest, "sha256:") || item.ExpiresAt.IsZero() || !now.Before(item.ExpiresAt) {
			return nil, "", Invalid("approval command digest, version, or expiry is invalid")
		}
		item.Status, item.DecidedBy, item.DecisionNote, item.DecisionAt = decision, actor.ID, note, &now
		item.Version, item.UpdatedAt = item.Version+1, now
		next, _ := json.Marshal(item)
		return next, "approval." + decision, nil
	})
}

func (s *Service) RequestRunApproval(ctx context.Context, actor Actor, projectID, runID, action, message, idem, remoteAddr string, expectedTargetVersion int64, lifetime time.Duration) (CommandResult, error) {
	if !actor.Can("approval:request") {
		return CommandResult{}, s.recordRejection(ctx, actor, projectID, "request.approval", string(domain.ResourceRun), runID, idem, ErrDenied)
	}
	if !domain.RequiresApproval(action) {
		return CommandResult{}, Invalid("approval requests are only valid for destructive or irreversible actions")
	}
	if lifetime <= 0 || lifetime > 24*time.Hour {
		return CommandResult{}, Invalid("approval expiry must be positive and at most 24 hours")
	}
	if expectedTargetVersion < 1 {
		return CommandResult{}, Invalid("expected_target_version must be a positive integer")
	}
	current, err := s.repo.Get(ctx, projectID, domain.ResourceRun, runID)
	if err != nil {
		return CommandResult{}, err
	}
	var target domain.Run
	if err := json.Unmarshal(current, &target); err != nil {
		return CommandResult{}, err
	}
	if target.Version != expectedTargetVersion {
		return CommandResult{}, ErrVersionConflict
	}
	now := s.clock.Now()
	approval := domain.Approval{
		Base: domain.Base{ID: s.ids.New()}, Kind: "run_action", Status: "pending",
		ResourceType: string(domain.ResourceRun), ResourceID: runID, RequestedBy: actor.ID,
		Reason: action + " run", CommandDigest: RunActionDigest(projectID, runID, actor.ID, action, message, expectedTargetVersion),
		CommandVersion: 1, ExpectedTargetVersion: expectedTargetVersion, ExpiresAt: now.Add(lifetime),
	}
	approval.Context, _ = json.Marshal(map[string]any{"action": action, "message": message, "expected_target_version": expectedTargetVersion})
	return s.Create(ctx, actor, domain.ResourceApproval, projectID, idem, approval, remoteAddr)
}

func (s *Service) RunAction(ctx context.Context, actor Actor, projectID, id, action, message, approvalID, idem string, expected int64, remoteAddr string) (CommandResult, error) {
	if !actor.Can("run:" + action) {
		return CommandResult{}, s.recordRejection(ctx, actor, projectID, action+".run", string(domain.ResourceRun), id, idem, ErrDenied)
	}
	allowed := map[string]struct{}{"pause": {}, "resume": {}, "cancel": {}, "message": {}, "redirect": {}}
	_, ok := allowed[action]
	if !ok {
		return CommandResult{}, Invalid(fmt.Sprintf("unsupported run action %q", action))
	}
	if s.controller == nil || !s.controller.Supports(id, action) {
		return CommandResult{}, ErrUnsupported
	}
	approvalDigest := ""
	if domain.RequiresApproval(action) {
		if approvalID == "" {
			return CommandResult{}, Invalid("destructive action requires approval_id")
		}
		approvalDigest = RunActionDigest(projectID, id, actor.ID, action, message, expected)
	}
	payload, _ := json.Marshal(map[string]string{"action": action, "message": message, "approval_id": approvalID})
	cmd := domain.Command{ID: s.ids.New(), ProjectID: projectID, Type: action + ".run", SubjectType: domain.ResourceRun, SubjectID: id, ActorID: actor.ID, ExpectedVersion: expected, IdempotencyKey: idem, RequestedAt: s.clock.Now(), RemoteAddr: remoteAddr, Payload: payload, ApprovalID: approvalID, ApprovalDigest: approvalDigest}
	control := &ControlIntent{
		ID: cmd.ID, ProjectID: projectID, RunID: id, ActorID: actor.ID,
		CommandID: cmd.ID, Action: action, Message: message, RequestedAt: cmd.RequestedAt,
	}
	return s.executeWithControl(ctx, cmd, control, func(current json.RawMessage) (json.RawMessage, string, error) {
		var run domain.Run
		if len(current) == 0 {
			return nil, "", ErrNotFound
		}
		if err := json.Unmarshal(current, &run); err != nil {
			return nil, "", err
		}
		run.ControlStatus = "pending"
		run.ControlAction = action
		run.ControlError = ""
		run.Summary = message
		run.Version++
		run.UpdatedAt = s.clock.Now()
		next, _ := json.Marshal(run)
		return next, "run.control_requested", nil
	})
}

func RunActionDigest(projectID, runID, actorID, action, message string, expectedTargetVersion int64) string {
	canonical, _ := json.Marshal(struct {
		Version               int    `json:"version"`
		ProjectID             string `json:"project_id"`
		RunID                 string `json:"run_id"`
		ActorID               string `json:"actor_id"`
		Action                string `json:"action"`
		Message               string `json:"message"`
		ExpectedTargetVersion int64  `json:"expected_target_version"`
	}{
		Version: 1, ProjectID: projectID, RunID: runID, ActorID: actorID,
		Action: action, Message: message, ExpectedTargetVersion: expectedTargetVersion,
	})
	digest := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func (s *Service) ReviewClaim(ctx context.Context, actor Actor, projectID, id, decision, note, idem string, expected int64, remoteAddr string) (CommandResult, error) {
	if !actor.Can("claim:review") {
		return CommandResult{}, ErrDenied
	}
	if decision != "accepted" && decision != "rejected" {
		return CommandResult{}, Invalid("claim decision must be accepted or rejected")
	}
	payload, _ := json.Marshal(map[string]string{"decision": decision, "note": note})
	cmd := domain.Command{ID: s.ids.New(), ProjectID: projectID, Type: "review.claim", SubjectType: domain.ResourceClaim, SubjectID: id, ActorID: actor.ID, ExpectedVersion: expected, IdempotencyKey: idem, RequestedAt: s.clock.Now(), RemoteAddr: remoteAddr, Payload: payload}
	return s.execute(ctx, cmd, func(current json.RawMessage) (json.RawMessage, string, error) {
		var claim domain.Claim
		if len(current) == 0 {
			return nil, "", ErrNotFound
		}
		if err := json.Unmarshal(current, &claim); err != nil {
			return nil, "", err
		}
		claim.Status, claim.ReviewNote, claim.ReviewerID = decision, note, actor.ID
		claim.Version++
		claim.UpdatedAt = s.clock.Now()
		next, _ := json.Marshal(claim)
		return next, "claim." + decision, nil
	})
}

func (s *Service) Ingest(ctx context.Context, actor Actor, events []domain.Event) error {
	if !actor.Can("event:ingest") {
		return ErrDenied
	}
	for i := range events {
		event := &events[i]
		if !actor.Can("source:"+event.SourceSystem) && !actor.Can("source:admin") {
			return ErrDenied
		}
		if !validIngestName(event.SourceSystem, 64) || !validIngestName(event.Type, 128) {
			return Invalid("source_system and event type must use bounded canonical names")
		}
		if event.SubjectType == domain.ResourceCoordinationRequest {
			return Invalid("source ingestion cannot create or change coordination requests")
		}
		if event.SubjectID == "" || !knownResourceType(event.SubjectType) {
			return Invalid("ingested event requires a known subject_type and nonempty subject_id")
		}
		if event.SchemaVersion != 0 && event.SchemaVersion != 1 {
			return Invalid("unsupported event schema_version")
		}
		if !json.Valid(event.Payload) || len(event.Payload) > 256<<10 {
			return Invalid("event payload must be valid JSON and at most 256 KiB")
		}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(event.Payload, &payload); err != nil || payload == nil {
			return Invalid("event payload must be a JSON object")
		}
		if _, ok := payload["resource"]; ok {
			return Invalid("ingestion cannot replace control-plane resources")
		}
		if event.SourceEventID == "" || len(event.SourceEventID) > 512 || event.SourceSequence < 0 {
			return Invalid("ingestion requires a bounded source_event_id and nonnegative source_sequence")
		}
		if event.SourceSystem == "codex" || event.SourceSystem == "claude" || event.SourceSystem == "hermes" {
			typed := event.Type == "agent.observed" || event.Type == "session.observed" || event.Type == "run.observed"
			// Native lifecycle codecs are compatibility foundations, not authority
			// for narrow reporters to mutate arbitrary coordination resources.
			if !typed && !actor.Can("source:admin") {
				return ErrDenied
			}
			if typed {
				if err := s.validateRuntimeObservation(ctx, actor, event, events[:i]); err != nil {
					return err
				}
			}
		}
		if domain.IsTaskObservationSource(event.SourceSystem) {
			if event.SourceSequence <= 0 {
				return Invalid("source task/project observations require a positive source_sequence")
			}
			switch {
			case event.Type == "task.observed" && event.SubjectType == domain.ResourceTask:
				var observation domain.TaskObservation
				decoder := json.NewDecoder(bytes.NewReader(event.Payload))
				decoder.DisallowUnknownFields()
				if err := decoder.Decode(&observation); err != nil {
					return Invalid("invalid task observation payload")
				}
				if strings.TrimSpace(observation.ExternalID) == "" || len(observation.ExternalID) > 1024 ||
					strings.TrimSpace(observation.Title) == "" || len(observation.Title) > 4096 ||
					strings.TrimSpace(observation.Status) == "" || len(observation.Status) > 128 || len(observation.Dependencies) > 1000 {
					return Invalid("task observation requires bounded external_id, title, status, and dependencies")
				}
				if observation.ExternalURL != "" {
					parsed, err := url.Parse(observation.ExternalURL)
					if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
						return Invalid("task observation external_url must be an HTTP or HTTPS URL without credentials")
					}
				}
				for _, dependency := range observation.Dependencies {
					if strings.TrimSpace(dependency) == "" || len(dependency) > 1024 || dependency == observation.ExternalID {
						return Invalid("task observation contains an invalid dependency")
					}
				}
				event.SubjectID = domain.ObservedTaskIDForSource(event.ProjectID, event.SourceSystem, observation.ExternalID)
			case event.Type == "project.observed" && event.SubjectType == domain.ResourceProject:
				var observation domain.ProjectObservation
				decoder := json.NewDecoder(bytes.NewReader(event.Payload))
				decoder.DisallowUnknownFields()
				if err := decoder.Decode(&observation); err != nil {
					return Invalid("invalid project observation payload")
				}
				if strings.TrimSpace(observation.ExternalID) == "" || len(observation.ExternalID) > 1024 ||
					strings.TrimSpace(observation.Name) == "" || len(observation.Name) > 4096 ||
					strings.TrimSpace(observation.Path) == "" || len(observation.Path) > 4096 ||
					strings.TrimSpace(observation.Status) == "" || len(observation.Status) > 128 ||
					len(observation.Reason) > 4096 || len(observation.ScanError) > 4096 || observation.TaskCount < 0 || observation.CollectedAt.IsZero() {
					return Invalid("project observation requires bounded identity, metadata, collection time, and nonnegative task_count")
				}
				if event.SourceSystem == "github" {
					parsed, err := url.Parse(observation.Path)
					if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
						return Invalid("GitHub project path must be an HTTP or HTTPS repository URL without credentials")
					}
				}
				if raw, ok := payload["task_count"]; !ok || string(raw) == "null" {
					return Invalid("project task_count is required")
				}
				if observation.ScanStatus != "complete" && observation.ScanStatus != "partial" && observation.ScanStatus != "unreadable" {
					return Invalid("project scan_status must be complete, partial, or unreadable")
				}
				if !event.OccurredAt.IsZero() && !event.OccurredAt.Equal(observation.CollectedAt) {
					return Invalid("project occurred_at must equal collected_at")
				}
				event.OccurredAt = observation.CollectedAt
				event.SubjectID = domain.ObservedProjectIDForSource(event.ProjectID, event.SourceSystem)
			default:
				return Invalid("source ingestion requires typed task.observed or project.observed")
			}
		}
		// External event identifiers and attribution are untrusted. The original
		// native identifier belongs in source metadata; the coordination event
		// always receives a server-generated ID and authenticated actor.
		event.ID = s.ids.New()
		now := s.clock.Now()
		if event.OccurredAt.IsZero() {
			event.OccurredAt = now
		} else if event.OccurredAt.After(now.Add(5*time.Minute)) || event.OccurredAt.Before(now.Add(-30*24*time.Hour)) {
			return Invalid("event occurred_at is outside the accepted clock-skew window")
		}
		if event.SchemaVersion == 0 {
			event.SchemaVersion = 1
		}
		event.ActorID = actor.ID
		event.Cursor = 0
		event.CommandID = ""
	}
	if err := s.repo.Ingest(ctx, events); err != nil {
		return err
	}
	return nil
}

var ingestNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]*$`)

func validIngestName(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && ingestNamePattern.MatchString(value)
}

func knownResourceType(kind domain.ResourceType) bool {
	switch kind {
	case domain.ResourceAgent, domain.ResourceRun, domain.ResourceSession, domain.ResourceTask,
		domain.ResourceAttention, domain.ResourceEvidence, domain.ResourceArtifact, domain.ResourceApproval,
		domain.ResourceIntervention, domain.ResourceChatMessage, domain.ResourceBudget, domain.ResourceClaim,
		domain.ResourceAudit, domain.ResourceOrganization, domain.ResourceHuman, domain.ResourceProject,
		domain.ResourceHost, domain.ResourceAgentInstance, domain.ResourceTaskTransition,
		domain.ResourceSituation, domain.ResourcePolicy, domain.ResourceDeployment, domain.ResourceCoordinationRequest:
		return true
	default:
		return false
	}
}

func (s *Service) execute(ctx context.Context, cmd domain.Command, mutate func(json.RawMessage) (json.RawMessage, string, error)) (CommandResult, error) {
	return s.executeWithControl(ctx, cmd, nil, mutate)
}

func (s *Service) executeWithControl(ctx context.Context, cmd domain.Command, control *ControlIntent, mutate func(json.RawMessage) (json.RawMessage, string, error)) (CommandResult, error) {
	if err := cmd.Validate(); err != nil {
		return CommandResult{}, Invalid(err.Error())
	}
	result, err := s.repo.Execute(ctx, cmd, func(current json.RawMessage) (Mutation, error) {
		resource, eventType, err := mutate(current)
		if err != nil {
			return Mutation{}, err
		}
		eventPayload, _ := json.Marshal(map[string]any{"resource": json.RawMessage(resource), "command_type": cmd.Type})
		event := domain.Event{ID: s.ids.New(), ProjectID: cmd.ProjectID, Type: eventType, SubjectType: cmd.SubjectType, SubjectID: cmd.SubjectID, ActorID: cmd.ActorID, CommandID: cmd.ID, CorrelationID: cmd.CorrelationID, OccurredAt: s.clock.Now(), SchemaVersion: 1, Payload: eventPayload}
		audit := domain.AuditRecord{Base: domain.NewBase(s.ids.New(), cmd.ProjectID, s.clock.Now()), ActorID: cmd.ActorID, Action: cmd.Type, ResourceType: string(cmd.SubjectType), ResourceID: cmd.SubjectID, CommandID: cmd.ID, Outcome: "accepted", RemoteAddr: cmd.RemoteAddr, Details: cmd.Payload}
		if cmd.SubjectType == domain.ResourceCoordinationRequest {
			audit.Details = nil
		}
		return Mutation{Resource: resource, Event: event, Audit: audit, Control: control}, nil
	})
	if err != nil {
		err = s.recordRejection(ctx, Actor{ID: cmd.ActorID}, cmd.ProjectID, cmd.Type, string(cmd.SubjectType), cmd.SubjectID, cmd.IdempotencyKey, err)
	}
	return result, err
}

func (s *Service) recordRejection(ctx context.Context, actor Actor, projectID, action, resourceType, resourceID, idem string, cause error) error {
	reason := rejectionClass(cause)
	if reason == "" || actor.ID == "" || projectID == "" {
		return cause
	}
	provider, ok := s.repo.(interface {
		RecordDecision(context.Context, Decision) error
	})
	if !ok {
		return cause
	}
	decision := Decision{
		ID: s.ids.New(), ProjectID: projectID, ActorID: actor.ID, Action: action,
		ResourceType: resourceType, ResourceID: resourceID, IdempotencyKey: idem,
		Outcome: "rejected", ReasonClass: reason, OccurredAt: s.clock.Now(),
	}
	if err := provider.RecordDecision(ctx, decision); err != nil {
		return err
	}
	return cause
}

func rejectionClass(err error) string {
	var validation ValidationError
	switch {
	case errors.Is(err, ErrDenied):
		return "authorization_or_approval"
	case errors.Is(err, ErrVersionConflict):
		return "version_conflict"
	case errors.Is(err, ErrBudgetExceeded):
		return "policy_budget"
	case errors.Is(err, ErrUnsupported):
		return "unsupported_runtime_action"
	case errors.Is(err, ErrIdempotency):
		return "idempotency_conflict"
	case errors.As(err, &validation):
		return "validation"
	default:
		return ""
	}
}

func ParseExpectedVersion(value string) (int64, error) {
	value = strings.Trim(strings.TrimSpace(value), `"`)
	if value == "" {
		return 0, errors.New("If-Match is required")
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 1 {
		return 0, errors.New("If-Match must be a positive integer version")
	}
	return n, nil
}

func normalizeBase(raw json.RawMessage, id, projectID string, version int64, now time.Time) json.RawMessage {
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return raw
	}
	object["id"] = id
	object["project_id"] = projectID
	object["version"] = version
	object["created_at"] = now.UTC()
	object["updated_at"] = now.UTC()
	normalized, err := json.Marshal(object)
	if err != nil {
		return raw
	}
	return normalized
}

func canRead(actor Actor, kind domain.ResourceType) bool {
	return actor.Can("resource:read") || (kind == domain.ResourceCoordinationRequest && actor.Can("coordination:read")) || actor.Can(string(kind)+":read")
}
