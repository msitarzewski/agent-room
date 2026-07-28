package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	return s.repo.Get(ctx, projectID, kind, id)
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
	page, err := s.repo.List(ctx, projectID, kind, cursor, limit)
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
	if page.Items == nil {
		page.Items = make([]domain.Event, 0)
	}
	return page, err
}

func (s *Service) Overview(ctx context.Context, actor Actor, projectID string) (map[string]any, error) {
	if !actor.Can("overview:read") && !actor.Can("resource:read") {
		return nil, ErrDenied
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
		if event.SubjectID == "" || !knownResourceType(event.SubjectType) {
			return Invalid("ingested event requires a known subject_type and nonempty subject_id")
		}
		if event.SchemaVersion != 0 && event.SchemaVersion != 1 {
			return Invalid("unsupported event schema_version")
		}
		if !json.Valid(event.Payload) || len(event.Payload) > 256<<10 {
			return Invalid("event payload must be valid JSON and at most 256 KiB")
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
		domain.ResourceSituation, domain.ResourcePolicy, domain.ResourceDeployment:
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
	return actor.Can("resource:read") || actor.Can(string(kind)+":read")
}
