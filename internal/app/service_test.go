package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/msitarzewski/agent-room/internal/domain"
)

type testClock struct{ now time.Time }

func (c testClock) Now() time.Time { return c.now }

type testIDs struct{ next int }

func (g *testIDs) New() string {
	g.next++
	return fmt.Sprintf("generated-%d", g.next)
}

type memoryRepository struct {
	resources    map[string]json.RawMessage
	results      map[string]CommandResult
	fingerprints map[string]string
	events       []domain.Event
	audits       []domain.AuditRecord
	decisions    []Decision
	briefCursor  int64
	ackExpected  int64
	ackThrough   int64
	ackReplayed  bool
	healthErr    error
	overview     map[string]any
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		resources:    make(map[string]json.RawMessage),
		results:      make(map[string]CommandResult),
		fingerprints: make(map[string]string),
		overview:     map[string]any{"status": "ok"},
	}
}

func resourceKey(projectID string, kind domain.ResourceType, id string) string {
	return projectID + "/" + string(kind) + "/" + id
}

func (r *memoryRepository) put(projectID string, kind domain.ResourceType, id string, value any) {
	raw, _ := json.Marshal(value)
	r.resources[resourceKey(projectID, kind, id)] = raw
}

func (r *memoryRepository) Get(_ context.Context, projectID string, kind domain.ResourceType, id string) (json.RawMessage, error) {
	value, ok := r.resources[resourceKey(projectID, kind, id)]
	if !ok {
		return nil, ErrNotFound
	}
	return append(json.RawMessage(nil), value...), nil
}

func (r *memoryRepository) List(_ context.Context, projectID string, kind domain.ResourceType, _ string, limit int) (Page, error) {
	prefix := projectID + "/" + string(kind) + "/"
	items := make([]json.RawMessage, 0)
	for key, value := range r.resources {
		if strings.HasPrefix(key, prefix) && len(items) < limit {
			items = append(items, append(json.RawMessage(nil), value...))
		}
	}
	return Page{Items: items}, nil
}

func (r *memoryRepository) Execute(_ context.Context, cmd domain.Command, build func(json.RawMessage) (Mutation, error)) (CommandResult, error) {
	fingerprint := cmd.Type + "|" + string(cmd.Payload) + "|" + strconv64(cmd.ExpectedVersion)
	idempotencyScope := cmd.ProjectID + "|" + cmd.ActorID + "|" + cmd.IdempotencyKey
	if previous, ok := r.results[idempotencyScope]; ok {
		if r.fingerprints[idempotencyScope] != fingerprint {
			return CommandResult{}, ErrIdempotency
		}
		previous.Replayed = true
		return previous, nil
	}
	key := resourceKey(cmd.ProjectID, cmd.SubjectType, cmd.SubjectID)
	current := r.resources[key]
	if cmd.ExpectedVersion > 0 && len(current) > 0 {
		var base domain.Base
		_ = json.Unmarshal(current, &base)
		if base.Version != cmd.ExpectedVersion {
			return CommandResult{}, ErrVersionConflict
		}
	}
	mutation, err := build(current)
	if err != nil {
		return CommandResult{}, err
	}
	r.resources[key] = mutation.Resource
	r.events = append(r.events, mutation.Event)
	r.audits = append(r.audits, mutation.Audit)
	result := CommandResult{Resource: mutation.Resource, Event: mutation.Event}
	r.results[idempotencyScope], r.fingerprints[idempotencyScope] = result, fingerprint
	return result, nil
}

func strconv64(value int64) string { return fmt.Sprintf("%d", value) }
func (r *memoryRepository) CoordinationRecipientExists(_ context.Context, projectID string, recipient domain.CoordinationRecipient) (bool, error) {
	kind := domain.ResourceAgent
	if recipient.Type == "human" {
		kind = domain.ResourceHuman
	}
	_, ok := r.resources[resourceKey(projectID, kind, recipient.ID)]
	return ok, nil
}

func (r *memoryRepository) ListCoordination(ctx context.Context, projectID string, actor Actor, cursor string, limit int) (Page, error) {
	page, err := r.List(ctx, projectID, domain.ResourceCoordinationRequest, cursor, limit)
	visible := []json.RawMessage{}
	for _, raw := range page.Items {
		if CoordinationVisible(actor, raw) {
			visible = append(visible, raw)
		}
	}
	page.Items = visible
	return page, err
}

func (r *memoryRepository) Ingest(_ context.Context, events []domain.Event) error {
	r.events = append(r.events, events...)
	return nil
}
func (r *memoryRepository) Events(_ context.Context, projectID string, after int64, limit int) (EventPage, error) {
	items := make([]domain.Event, 0)
	for _, event := range r.events {
		if event.ProjectID == projectID && event.Cursor > after && len(items) < limit {
			items = append(items, event)
		}
	}
	return EventPage{Items: items}, nil
}
func (r *memoryRepository) Overview(context.Context, string) (map[string]any, error) {
	return r.overview, nil
}
func (r *memoryRepository) Health(context.Context) error { return r.healthErr }
func (r *memoryRepository) Close()                       {}
func (r *memoryRepository) BriefCursor(context.Context, string, string) (int64, error) {
	return r.briefCursor, nil
}
func (r *memoryRepository) AcknowledgeBrief(_ context.Context, _, _, _, _ string, expected, through int64, _ time.Time) (bool, error) {
	r.ackExpected, r.ackThrough = expected, through
	return r.ackReplayed, nil
}
func (r *memoryRepository) RecordDecision(_ context.Context, decision Decision) error {
	r.decisions = append(r.decisions, decision)
	return nil
}

type testController struct {
	supported bool
}

func (c testController) Supports(string, string) bool                          { return c.supported }
func (c testController) Execute(context.Context, string, string, string) error { return nil }

func actor(id string, capabilities ...string) Actor {
	value := Actor{ID: id, Capabilities: make(map[string]struct{}, len(capabilities))}
	for _, capability := range capabilities {
		value.Capabilities[capability] = struct{}{}
	}
	return value
}

func base(id string, version int64, now time.Time) domain.Base {
	return domain.Base{ID: id, ProjectID: "project", Version: version, CreatedAt: now, UpdatedAt: now}
}

func decode[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

type componentRepository struct {
	*memoryRepository
	components map[string]any
	err        error
}

func (r componentRepository) ComponentHealth(context.Context) (map[string]any, error) {
	return r.components, r.err
}

func TestServiceReadSurfacesAndBrief(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepository()
	repo.briefCursor = 10
	repo.put("project", domain.ResourceTask, "task", domain.Task{Base: base("task", 1, now), Status: "ready"})
	repo.put("project", domain.ResourceAttention, "open", domain.Attention{Base: base("open", 1, now), Status: "open"})
	repo.put("project", domain.ResourceAttention, "closed", domain.Attention{Base: base("closed", 1, now), Status: "resolved"})
	repo.put("project", domain.ResourceApproval, "pending", domain.Approval{Base: base("pending", 1, now), Status: "pending"})
	repo.events = []domain.Event{{Cursor: 11, ProjectID: "project", Type: "task.started"}, {Cursor: 12, ProjectID: "project", Type: "task.started"}}
	service := NewServiceWithDependencies(repo, nil, testClock{now}, &testIDs{})

	if _, err := service.Get(ctx, actor("reader"), "project", domain.ResourceTask, "task"); !errors.Is(err, ErrDenied) {
		t.Fatalf("Get denied error = %v", err)
	}
	raw, err := service.Get(ctx, actor("reader", "task:read"), "project", domain.ResourceTask, "task")
	if err != nil || decode[domain.Task](t, raw).ID != "task" {
		t.Fatalf("Get = %s, %v", raw, err)
	}
	page, err := service.List(ctx, actor("reader", "resource:read"), "project", domain.ResourceTask, "", 0)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("List = %#v, %v", page, err)
	}
	eventPage, err := service.Events(ctx, actor("reader", "event:read"), "project", 10, 0)
	if err != nil || len(eventPage.Items) != 2 {
		t.Fatalf("Events = %#v, %v", eventPage, err)
	}
	if _, err := service.Events(ctx, actor("reader"), "project", 0, 1); !errors.Is(err, ErrDenied) {
		t.Fatalf("Events denied error = %v", err)
	}
	if _, err := service.Overview(ctx, actor("reader", "overview:read"), "project"); err != nil {
		t.Fatal(err)
	}
	brief, err := service.Brief(ctx, actor("reader", "resource:read"), "project", -1)
	if err != nil {
		t.Fatal(err)
	}
	if brief["reviewed_cursor"] != int64(10) || brief["through_cursor"] != int64(12) {
		t.Fatalf("brief cursors = %#v", brief)
	}
	if got := brief["recommended_actions"].([]string); !reflect.DeepEqual(got, []string{"Review pending approvals", "Triage open attention items"}) {
		t.Fatalf("recommended actions = %#v", got)
	}
	if _, err := service.Brief(ctx, actor("reader"), "project", 0); !errors.Is(err, ErrDenied) {
		t.Fatalf("Brief denied error = %v", err)
	}
	if _, err := service.AcknowledgeBrief(ctx, actor("reader"), "project", "idem", 0, 1); !errors.Is(err, ErrDenied) {
		t.Fatalf("AcknowledgeBrief denied error = %v", err)
	}
	if _, err := service.AcknowledgeBrief(ctx, actor("reader", "overview:read"), "project", "idem", -1, 1); err == nil {
		t.Fatal("negative brief cursor accepted")
	}
	repo.ackReplayed = true
	replayed, err := service.AcknowledgeBrief(ctx, actor("reader", "overview:read"), "project", "idem", 10, 12)
	if err != nil || !replayed || repo.ackExpected != 10 || repo.ackThrough != 12 {
		t.Fatalf("AcknowledgeBrief = %v, %v", replayed, err)
	}
	if err := service.Health(ctx); err != nil {
		t.Fatal(err)
	}
	health, err := service.ComponentHealth(ctx)
	if err != nil || health["database"] == nil {
		t.Fatalf("ComponentHealth = %#v, %v", health, err)
	}
}

func TestServiceCreateValidationAndIdempotency(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepository()
	service := NewServiceWithDependencies(repo, nil, testClock{now}, &testIDs{})
	writer := actor("writer", "resource:write")

	if _, err := service.Create(ctx, actor("denied"), domain.ResourceTask, "project", "denied", map[string]any{"id": "task"}, "127.0.0.1"); !errors.Is(err, ErrDenied) {
		t.Fatalf("Create denied error = %v", err)
	}
	if len(repo.decisions) != 1 || repo.decisions[0].ReasonClass != "authorization_or_approval" {
		t.Fatalf("denied decisions = %#v", repo.decisions)
	}
	if _, err := service.Create(ctx, writer, domain.ResourceTask, "project", "missing-id", map[string]any{"title": "missing"}, ""); err == nil {
		t.Fatal("Create accepted missing id")
	}
	result, err := service.Create(ctx, writer, domain.ResourceTask, "project", "create-task", map[string]any{"id": "task", "status": "ready"}, "")
	if err != nil {
		t.Fatal(err)
	}
	task := decode[domain.Task](t, result.Resource)
	if task.ProjectID != "project" || task.Version != 1 || task.CreatedAt != now {
		t.Fatalf("normalized task = %#v", task)
	}
	replay, err := service.Create(ctx, writer, domain.ResourceTask, "project", "create-task", map[string]any{"id": "task", "status": "ready"}, "")
	if err != nil || !replay.Replayed {
		t.Fatalf("idempotent replay = %#v, %v", replay, err)
	}
	if _, err := service.Create(ctx, writer, domain.ResourceTask, "project", "create-task", map[string]any{"id": "other"}, ""); !errors.Is(err, ErrIdempotency) {
		t.Fatalf("idempotency conflict error = %v", err)
	}
	repo.put("project", domain.ResourceRun, "run", domain.Run{Base: base("run", 1, now)})
	artifact, err := service.Create(ctx, writer, domain.ResourceArtifact, "project", "artifact", map[string]any{"id": "artifact", "run_id": "run"}, "")
	if err != nil || decode[domain.Artifact](t, artifact.Resource).ScopeType != "run" {
		t.Fatalf("artifact = %#v, %v", artifact, err)
	}
	if _, err := service.Create(ctx, writer, domain.ResourceEvidence, "project", "bad-link", map[string]any{"id": "evidence", "task_id": "missing"}, ""); err == nil {
		t.Fatal("cross-resource missing link accepted")
	}
}

func TestServiceTaskAttentionAndClaimCommands(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepository()
	ids := &testIDs{}
	service := NewServiceWithDependencies(repo, nil, testClock{now}, ids)
	repo.put("project", domain.ResourceTask, "task", domain.Task{Base: base("task", 1, now), Status: "ready", OwnerID: "agent"})
	taskActor := actor("agent", "task:transition", "task:claim", "task:review")

	if _, err := service.TransitionTask(ctx, taskActor, "project", "task", "blocked", "", "block-no-reason", 1, ""); err == nil {
		t.Fatal("blocked task accepted without reason")
	}
	result, err := service.TransitionTask(ctx, taskActor, "project", "task", "working", "", "work", 1, "")
	if err != nil || decode[domain.Task](t, result.Resource).Status != "working" {
		t.Fatalf("transition = %#v, %v", result, err)
	}
	if _, err := service.TransitionTask(ctx, taskActor, "project", "task", "archived", "", "invalid-transition", 2, ""); err == nil {
		t.Fatal("invalid transition accepted")
	}
	result, err = service.RequestTaskReview(ctx, taskActor, "project", "task", "review", "review", 2, "")
	if err != nil || decode[domain.Task](t, result.Resource).ReviewState != "requested" {
		t.Fatalf("review request = %#v, %v", result, err)
	}
	repo.put("project", domain.ResourceTask, "claimable", domain.Task{Base: base("claimable", 1, now), Status: "ready"})
	result, err = service.ClaimTask(ctx, taskActor, "project", "claimable", "claim", 1, "")
	if err != nil || decode[domain.Task](t, result.Resource).OwnerID != "agent" {
		t.Fatalf("claim = %#v, %v", result, err)
	}
	repo.put("project", domain.ResourceTask, "owned", domain.Task{Base: base("owned", 1, now), Status: "ready", OwnerID: "other"})
	if _, err := service.ClaimTask(ctx, taskActor, "project", "owned", "claim-owned", 1, ""); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("owned task claim error = %v", err)
	}

	repo.put("project", domain.ResourceAttention, "attention", domain.Attention{Base: base("attention", 1, now), Status: "open"})
	manager := actor("manager", "attention:manage")
	if _, err := service.SetAttentionStatus(ctx, manager, "project", "attention", "ignored", "invalid-attention", 1, ""); err == nil {
		t.Fatal("invalid attention status accepted")
	}
	result, err = service.SetAttentionStatus(ctx, manager, "project", "attention", "acknowledged", "ack", 1, "")
	item := decode[domain.Attention](t, result.Resource)
	if err != nil || item.AcknowledgedAt == nil {
		t.Fatalf("acknowledge = %#v, %v", item, err)
	}
	result, err = service.SetAttentionStatus(ctx, manager, "project", "attention", "resolved", "resolve", 2, "")
	if err != nil || decode[domain.Attention](t, result.Resource).ResolvedAt == nil {
		t.Fatalf("resolve = %#v, %v", result, err)
	}

	repo.put("project", domain.ResourceClaim, "claim", domain.Claim{Base: base("claim", 1, now), Status: "pending"})
	reviewer := actor("reviewer", "claim:review")
	if _, err := service.ReviewClaim(ctx, reviewer, "project", "claim", "unknown", "", "bad-review", 1, ""); err == nil {
		t.Fatal("invalid claim decision accepted")
	}
	result, err = service.ReviewClaim(ctx, reviewer, "project", "claim", "accepted", "verified", "review", 1, "")
	claim := decode[domain.Claim](t, result.Resource)
	if err != nil || claim.Status != "accepted" || claim.ReviewerID != "reviewer" {
		t.Fatalf("claim review = %#v, %v", claim, err)
	}
}

func TestServiceApprovalsAndRunControl(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepository()
	ids := &testIDs{}
	service := NewServiceWithDependencies(repo, nil, testClock{now}, ids)
	service.controller = testController{supported: true}
	repo.put("project", domain.ResourceRun, "run", domain.Run{Base: base("run", 3, now), Status: "working"})
	requester := actor("operator", "approval:request", "resource:write", "run:pause", "run:cancel")

	for _, test := range []struct {
		name     string
		action   string
		version  int64
		lifetime time.Duration
	}{
		{"non-destructive", "pause", 3, time.Hour},
		{"bad version", "cancel", 0, time.Hour},
		{"bad lifetime", "cancel", 3, 25 * time.Hour},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := service.RequestRunApproval(ctx, requester, "project", "run", test.action, "", "approval-"+test.name, "", test.version, test.lifetime); err == nil {
				t.Fatal("invalid approval request accepted")
			}
		})
	}
	approvalResult, err := service.RequestRunApproval(ctx, requester, "project", "run", "cancel", "unsafe", "approval", "", 3, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	approval := decode[domain.Approval](t, approvalResult.Resource)
	wantDigest := RunActionDigest("project", "run", "operator", "cancel", "unsafe", 3)
	if approval.CommandDigest != wantDigest || approval.ExpiresAt != now.Add(time.Hour) || approval.ExpectedTargetVersion != 3 {
		t.Fatalf("approval = %#v", approval)
	}

	if _, err := service.DecideApproval(ctx, actor("reviewer", "approval:decide"), "project", approval.ID, "maybe", "", "bad-decision", 1, ""); err == nil {
		t.Fatal("invalid approval decision accepted")
	}
	expired := approval
	expired.ID, expired.ExpiresAt = "expired", now
	repo.put("project", domain.ResourceApproval, "expired", expired)
	if _, err := service.DecideApproval(ctx, actor("reviewer", "approval:decide"), "project", "expired", "approved", "", "expired-decision", 1, ""); err == nil {
		t.Fatal("expired approval accepted")
	}
	decision, err := service.DecideApproval(ctx, actor("reviewer", "approval:decide"), "project", approval.ID, "approved", "verified", "decision", 1, "")
	approved := decode[domain.Approval](t, decision.Resource)
	if err != nil || approved.Status != "approved" || approved.DecidedBy != "reviewer" {
		t.Fatalf("approval decision = %#v, %v", approved, err)
	}

	if _, err := service.RunAction(ctx, requester, "project", "run", "unknown", "", "", "unknown", 3, ""); err == nil {
		t.Fatal("unsupported action accepted")
	}
	if _, err := service.RunAction(ctx, requester, "project", "run", "cancel", "unsafe", "", "cancel-no-approval", 3, ""); err == nil {
		t.Fatal("destructive action accepted without approval")
	}
	runResult, err := service.RunAction(ctx, requester, "project", "run", "pause", "investigate", "", "pause", 3, "")
	run := decode[domain.Run](t, runResult.Resource)
	if err != nil || run.ControlStatus != "pending" || run.ControlAction != "pause" || run.Version != 4 {
		t.Fatalf("run action = %#v, %v", run, err)
	}
	unsupported := NewServiceWithController(repo, nil, testController{supported: false})
	if _, err := unsupported.RunAction(ctx, requester, "project", "run", "pause", "", "", "unsupported", 4, ""); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported controller error = %v", err)
	}
}

func TestServiceIngestAndHelpers(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepository()
	service := NewServiceWithDependencies(repo, nil, testClock{now}, &testIDs{})
	ingester := actor("adapter", "event:ingest", "source:fixture")
	valid := domain.Event{ProjectID: "project", Type: "run.updated", SubjectType: domain.ResourceRun, SubjectID: "run", SourceSystem: "fixture", SourceEventID: "native", Payload: json.RawMessage(`{"ok":true}`)}

	if err := service.Ingest(ctx, actor("denied"), []domain.Event{valid}); !errors.Is(err, ErrDenied) {
		t.Fatalf("ingest denied error = %v", err)
	}
	cases := []domain.Event{
		func() domain.Event { value := valid; value.SourceSystem = "other"; return value }(),
		func() domain.Event { value := valid; value.Type = "Bad Type"; return value }(),
		func() domain.Event { value := valid; value.SubjectType = "unknown"; return value }(),
		func() domain.Event { value := valid; value.SchemaVersion = 2; return value }(),
		func() domain.Event { value := valid; value.Payload = json.RawMessage(`{`); return value }(),
		func() domain.Event { value := valid; value.OccurredAt = now.Add(6 * time.Minute); return value }(),
	}
	for index, event := range cases {
		if err := service.Ingest(ctx, ingester, []domain.Event{event}); err == nil {
			t.Fatalf("invalid ingest case %d accepted", index)
		}
	}
	events := []domain.Event{valid}
	if err := service.Ingest(ctx, ingester, events); err != nil {
		t.Fatal(err)
	}
	if len(repo.events) != 1 || repo.events[0].ActorID != "adapter" || repo.events[0].SchemaVersion != 1 || repo.events[0].OccurredAt != now {
		t.Fatalf("normalized event = %#v", repo.events)
	}
	if RunActionDigest("p", "r", "a", "cancel", "m", 1) == RunActionDigest("p", "r", "a", "cancel", "m", 2) {
		t.Fatal("digest does not bind target version")
	}
	for _, value := range []string{"1", `"2"`, " 3 "} {
		if _, err := ParseExpectedVersion(value); err != nil {
			t.Fatalf("ParseExpectedVersion(%q) = %v", value, err)
		}
	}
	for _, value := range []string{"", "0", "-1", "word"} {
		if _, err := ParseExpectedVersion(value); err == nil {
			t.Fatalf("ParseExpectedVersion(%q) accepted", value)
		}
	}
	for err, want := range map[error]string{
		ErrDenied: "authorization_or_approval", ErrVersionConflict: "version_conflict",
		ErrBudgetExceeded: "policy_budget", ErrUnsupported: "unsupported_runtime_action",
		ErrIdempotency: "idempotency_conflict", Invalid("bad"): "validation",
	} {
		if got := rejectionClass(err); got != want {
			t.Fatalf("rejectionClass(%v) = %q, want %q", err, got, want)
		}
	}
}

func TestServiceBoundaryAndDeniedBranches(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepository()
	ids := &testIDs{}
	service := NewServiceWithDependencies(repo, nil, testClock{now}, ids)

	if (ValidationError{Message: "invalid"}).Error() != "invalid" {
		t.Fatal("validation error did not preserve its message")
	}
	if got := (systemClock{}).Now(); got.Location() != time.UTC {
		t.Fatalf("system clock location = %v", got.Location())
	}
	randomID := (randomIDs{}).New()
	if len(randomID) != 36 || randomID[14] != '4' {
		t.Fatalf("random ID is not a UUIDv4: %q", randomID)
	}
	(discardPublisher{}).Publish(domain.Event{})
	if service.NewID() == "" {
		t.Fatal("NewID returned empty")
	}

	if _, err := service.List(ctx, actor("none"), "project", domain.ResourceTask, "", 1); !errors.Is(err, ErrDenied) {
		t.Fatalf("List denied error = %v", err)
	}
	page, err := service.List(ctx, actor("reader", "resource:read"), "project", domain.ResourceTask, "", 500)
	if err != nil || page.Items == nil {
		t.Fatalf("bounded empty List = %#v, %v", page, err)
	}
	events, err := service.Events(ctx, actor("reader", "resource:read"), "project", 0, 5000)
	if err != nil || events.Items == nil {
		t.Fatalf("bounded empty Events = %#v, %v", events, err)
	}
	if _, err := service.Overview(ctx, actor("none"), "project"); !errors.Is(err, ErrDenied) {
		t.Fatalf("Overview denied error = %v", err)
	}
	repo.healthErr = errors.New("database unavailable")
	if _, err := service.ComponentHealth(ctx); err == nil {
		t.Fatal("ComponentHealth ignored repository failure")
	}
	components := componentRepository{memoryRepository: repo, components: map[string]any{"schema": "ok"}}
	gotComponents, err := NewService(components, nil).ComponentHealth(ctx)
	if err != nil || gotComponents["schema"] != "ok" {
		t.Fatalf("component provider = %#v, %v", gotComponents, err)
	}
	repo.healthErr = nil

	deniedCalls := []struct {
		name string
		call func() error
	}{
		{"transition", func() error {
			_, err := service.TransitionTask(ctx, actor("none"), "project", "task", "ready", "", "d1", 1, "")
			return err
		}},
		{"claim task", func() error {
			_, err := service.ClaimTask(ctx, actor("none"), "project", "task", "d2", 1, "")
			return err
		}},
		{"task review", func() error {
			_, err := service.RequestTaskReview(ctx, actor("none"), "project", "task", "", "d3", 1, "")
			return err
		}},
		{"attention", func() error {
			_, err := service.SetAttentionStatus(ctx, actor("none"), "project", "attention", "resolved", "d4", 1, "")
			return err
		}},
		{"approval decision", func() error {
			_, err := service.DecideApproval(ctx, actor("none"), "project", "approval", "approved", "", "d5", 1, "")
			return err
		}},
		{"approval request", func() error {
			_, err := service.RequestRunApproval(ctx, actor("none"), "project", "run", "cancel", "", "d6", "", 1, time.Hour)
			return err
		}},
		{"run action", func() error {
			_, err := service.RunAction(ctx, actor("none"), "project", "run", "pause", "", "", "d7", 1, "")
			return err
		}},
		{"claim review", func() error {
			_, err := service.ReviewClaim(ctx, actor("none"), "project", "claim", "accepted", "", "d8", 1, "")
			return err
		}},
	}
	for _, test := range deniedCalls {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, ErrDenied) {
				t.Fatalf("denied error = %v", err)
			}
		})
	}

	transitioner := actor("worker", "task:transition", "task:claim", "task:review")
	if _, err := service.TransitionTask(ctx, transitioner, "project", "missing", "ready", "", "n1", 1, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing transition error = %v", err)
	}
	if _, err := service.ClaimTask(ctx, transitioner, "project", "missing", "n2", 1, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing claim error = %v", err)
	}
	if _, err := service.RequestTaskReview(ctx, transitioner, "project", "missing", "", "n3", 1, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing review error = %v", err)
	}
	if _, err := service.SetAttentionStatus(ctx, actor("manager", "attention:manage"), "project", "missing", "resolved", "n4", 1, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing attention error = %v", err)
	}
	if _, err := service.DecideApproval(ctx, actor("reviewer", "approval:decide"), "project", "missing", "approved", "", "n5", 1, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing approval error = %v", err)
	}
	if _, err := service.ReviewClaim(ctx, actor("reviewer", "claim:review"), "project", "missing", "accepted", "", "n6", 1, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing claim review error = %v", err)
	}
}

func TestWorkspaceReadCapabilitiesFailClosed(t *testing.T) {
	ctx := context.Background()
	repo := &workspaceTestRepository{memoryRepository: newMemoryRepository()}
	service := NewService(repo, nil)
	for _, value := range []Actor{actor("none"), actor("overview", "overview:read"), actor("tasks", "task:read")} {
		if _, err := service.Workspace(ctx, value, "project"); !errors.Is(err, ErrDenied) {
			t.Fatalf("actor %s workspace error=%v", value.ID, err)
		}
	}
	if repo.calls != 0 {
		t.Fatal("denied request reached repository")
	}
	value, err := service.Workspace(ctx, actor("reader", "resource:read"), "project")
	if err != nil || value.ProjectID != "project" || repo.calls != 1 {
		t.Fatalf("workspace=%+v calls=%d err=%v", value, repo.calls, err)
	}
	capabilities := []string{"task:read", "agent:read", "run:read", "attention:read", "evidence:read", "artifact:read", "claim:read", "approval:read", "coordination:read"}
	if _, err := service.Workspace(ctx, actor("scoped", capabilities...), "project"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Workspace(ctx, actor("reader", "resource:read"), ""); err == nil {
		t.Fatal("blank project accepted")
	}
}

type workspaceTestRepository struct {
	*memoryRepository
	calls int
}

func (r *workspaceTestRepository) Workspace(_ context.Context, projectID string) (WorkspaceSnapshot, error) {
	r.calls++
	return WorkspaceSnapshot{ProjectID: projectID}, nil
}

func TestSentryObservationIngestionRejectsControlState(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 13, 20, 0, 0, 0, time.UTC)
	repo := newMemoryRepository()
	service := NewServiceWithDependencies(repo, nil, testClock{now}, &testIDs{})
	ingester := actor("sentry-adapter", "event:ingest", "source:sentry")
	valid := domain.Event{ProjectID: "project", Type: "task.observed", SubjectType: domain.ResourceTask,
		SubjectID: "untrusted-subject", SourceSystem: "sentry", SourceEventID: "scan-1-task-1", SourceSequence: 1,
		Payload: json.RawMessage(`{"external_id":"Software/agent-rooms#release","title":"First spatial release","status":"completed","dependencies":["Software/agent-rooms#api"]}`)}
	for name, mutate := range map[string]func(*domain.Event){
		"privileged resource": func(event *domain.Event) { event.Payload = json.RawMessage(`{"resource":{"status":"completed"}}`) },
		"unexpected field": func(event *domain.Event) {
			event.Payload = json.RawMessage(`{"external_id":"task","title":"Task","status":"done","owner_id":"admin"}`)
		},
		"unsafe URL": func(event *domain.Event) {
			event.Payload = json.RawMessage(`{"external_id":"task","title":"Task","status":"done","external_url":"javascript:alert(1)"}`)
		},
		"missing external identity": func(event *domain.Event) { event.Payload = json.RawMessage(`{"title":"Task","status":"done"}`) },
		"unsequenced observation":   func(event *domain.Event) { event.SourceSequence = 0 },
		"supervised event type":     func(event *domain.Event) { event.Type = "task.completed" },
		"other resource":            func(event *domain.Event) { event.SubjectType = domain.ResourceApproval },
	} {
		t.Run(name, func(t *testing.T) {
			value := valid
			mutate(&value)
			if err := service.Ingest(ctx, ingester, []domain.Event{value}); err == nil {
				t.Fatal("unsafe observation accepted")
			}
		})
	}
	valid.ActorID, valid.CommandID, valid.Cursor = "forged", "forged-command", 900
	if err := service.Ingest(ctx, ingester, []domain.Event{valid}); err != nil {
		t.Fatal(err)
	}
	if len(repo.events) != 1 {
		t.Fatalf("events=%d", len(repo.events))
	}
	got := repo.events[0]
	if got.SubjectID != domain.ObservedTaskID("project", "Software/agent-rooms#release") || got.ActorID != ingester.ID || got.CommandID != "" || got.Cursor != 0 {
		t.Fatalf("observation attribution=%+v", got)
	}
	if domain.ObservedTaskID("other", "Software/agent-rooms#release") == got.SubjectID {
		t.Fatal("task identity is not project scoped")
	}
}

func TestSentryProjectObservationValidation(t *testing.T) {
	at := time.Date(2026, 10, 14, 13, 0, 0, 0, time.UTC)
	repo := newMemoryRepository()
	service := NewServiceWithDependencies(repo, nil, testClock{at}, &testIDs{})
	ingester := actor("sentry-adapter", "event:ingest", "source:sentry")
	payload := map[string]any{"external_id": "host:/project", "name": "Source name", "path": "/project", "status": "Active", "collected_at": at.Format(time.RFC3339Nano), "scan_status": "complete", "task_count": 0}
	raw, _ := json.Marshal(payload)
	valid := domain.Event{ProjectID: "project", SubjectType: domain.ResourceProject, SubjectID: "untrusted", Type: "project.observed", SourceSystem: "sentry", SourceEventID: "project-scan-1", SourceSequence: 1, Payload: raw}
	for name, change := range map[string]func(map[string]any){
		"extra authority field":   func(p map[string]any) { p["capabilities"] = []string{"resource:write"} },
		"negative task count":     func(p map[string]any) { p["task_count"] = -1 },
		"missing task count":      func(p map[string]any) { delete(p, "task_count") },
		"null task count":         func(p map[string]any) { p["task_count"] = nil },
		"unknown scan status":     func(p map[string]any) { p["scan_status"] = "healthy" },
		"missing collection time": func(p map[string]any) { delete(p, "collected_at") },
		"future collection time":  func(p map[string]any) { p["collected_at"] = at.Add(6 * time.Minute).Format(time.RFC3339Nano) },
		"missing external id":     func(p map[string]any) { delete(p, "external_id") },
	} {
		t.Run(name, func(t *testing.T) {
			copyPayload := map[string]any{}
			for key, value := range payload {
				copyPayload[key] = value
			}
			change(copyPayload)
			event := valid
			event.Payload, _ = json.Marshal(copyPayload)
			if err := service.Ingest(context.Background(), ingester, []domain.Event{event}); err == nil {
				t.Fatal("invalid project observation accepted")
			}
		})
	}
	mismatched := valid
	mismatched.OccurredAt = at.Add(-time.Minute)
	if err := service.Ingest(context.Background(), ingester, []domain.Event{mismatched}); err == nil {
		t.Fatal("mismatched collection time accepted")
	}
	if err := service.Ingest(context.Background(), ingester, []domain.Event{valid}); err != nil {
		t.Fatal(err)
	}
	if len(repo.events) != 1 || repo.events[0].SubjectID != "project" || !repo.events[0].OccurredAt.Equal(at) {
		t.Fatalf("normalized project event=%+v", repo.events)
	}
}

func TestCoordinationCommandsEnforceParticipantsAndHumanEscalation(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepository()
	service := NewServiceWithDependencies(repo, nil, testClock{now}, &testIDs{})
	owner := actor("agent:owner", "coordination:create", "coordination:read", "coordination:resolve", "coordination:cancel")
	recipient := actor("agent:recipient", "coordination:respond", "coordination:read")
	outsider := actor("agent:other", "coordination:respond", "coordination:resolve", "coordination:read", "event:read")
	repo.put("project", domain.ResourceAgent, recipient.ID, domain.Agent{Base: base(recipient.ID, 1, now)})
	repo.put("project", domain.ResourceHuman, "human", domain.Human{Base: base("human", 1, now)})
	repo.put("project", domain.ResourceTask, "task", domain.Task{Base: base("task", 1, now), OwnerID: owner.ID, Status: "working"})
	repo.put("project", domain.ResourceEvidence, "evidence", domain.Evidence{Base: base("evidence", 1, now), TaskID: "task"})
	input := CreateCoordinationInput{ID: "request", Kind: "handoff", Title: "Review the diff", Body: "Recorded handoff context", Recipient: domain.CoordinationRecipient{Type: "agent", ID: recipient.ID}, TaskID: "task", EvidenceIDs: []string{"evidence"}}
	for name, change := range map[string]func(*CreateCoordinationInput){
		"id": func(i *CreateCoordinationInput) { i.ID = "" }, "kind": func(i *CreateCoordinationInput) { i.Kind = "execute" }, "title": func(i *CreateCoordinationInput) { i.Title = " " }, "body": func(i *CreateCoordinationInput) { i.Body = "" },
		"recipient type": func(i *CreateCoordinationInput) { i.Recipient.Type = "all" }, "unknown recipient": func(i *CreateCoordinationInput) { i.Recipient.ID = "missing" }, "self": func(i *CreateCoordinationInput) { i.Recipient.ID = owner.ID },
		"cross project task": func(i *CreateCoordinationInput) { i.TaskID = "missing" }, "cross project evidence": func(i *CreateCoordinationInput) { i.EvidenceIDs = []string{"missing"} }, "duplicate evidence": func(i *CreateCoordinationInput) { i.EvidenceIDs = []string{"evidence", "evidence"} },
		"too much evidence": func(i *CreateCoordinationInput) { i.EvidenceIDs = make([]string, 101) }, "agent escalation": func(i *CreateCoordinationInput) { i.EscalationReason = "Human decision" },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := input
			change(&invalid)
			if _, err := service.CreateCoordination(ctx, owner, "project", "bad-"+name, invalid, ""); err == nil {
				t.Fatal("invalid request accepted")
			}
		})
	}
	if _, err := service.CreateCoordination(ctx, actor("source:sentry", "event:ingest", "source:sentry"), "project", "source", input, ""); !errors.Is(err, ErrDenied) {
		t.Fatalf("publisher command err=%v", err)
	}
	if _, err := service.Create(ctx, actor("operator", "resource:write"), domain.ResourceCoordinationRequest, "project", "bypass", domain.CoordinationRequest{}, ""); err == nil {
		t.Fatal("generic create bypass")
	}
	created, err := service.CreateCoordination(ctx, owner, "project", "request-create", input, "")
	if err != nil {
		t.Fatal(err)
	}
	request := decode[domain.CoordinationRequest](t, created.Resource)
	if request.OwnerID != owner.ID || request.OwnerType != "agent" || request.Status != "open" || request.Version != 1 {
		t.Fatalf("request=%+v", request)
	}
	if replay, err := service.CreateCoordination(ctx, owner, "project", "request-create", input, ""); err != nil || !replay.Replayed {
		t.Fatalf("retry=%+v err=%v", replay, err)
	}
	if _, err := service.Get(ctx, outsider, "project", domain.ResourceCoordinationRequest, "request"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("private get=%v", err)
	}
	page, err := service.List(ctx, outsider, "project", domain.ResourceCoordinationRequest, "", 50)
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("private list=%+v err=%v", page, err)
	}
	page, err = service.List(ctx, recipient, "project", domain.ResourceCoordinationRequest, "", 50)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("incoming list=%+v err=%v", page, err)
	}
	if CoordinationEventVisible(outsider, created.Event) || !CoordinationEventVisible(recipient, created.Event) {
		t.Fatal("event visibility failed")
	}
	events, err := service.Events(ctx, outsider, "project", 0, 100)
	if err != nil || len(events.Items) != 0 {
		t.Fatalf("event leak=%+v err=%v", events, err)
	}
	response := CoordinationActionInput{Body: "Reviewed", EvidenceIDs: []string{"evidence"}}
	if _, err := service.ActCoordination(ctx, outsider, "project", "request", "respond", "outside", 1, response, ""); !errors.Is(err, ErrDenied) {
		t.Fatalf("outside respond=%v", err)
	}
	for _, test := range []struct {
		action  string
		version int64
		input   CoordinationActionInput
	}{{"execute", 1, response}, {"respond", 0, response}, {"respond", 1, CoordinationActionInput{}}, {"respond", 1, CoordinationActionInput{Body: "x", Note: "x"}}, {"resolve", 1, response}, {"respond", 1, CoordinationActionInput{Body: "x", EvidenceIDs: []string{"missing"}}}} {
		if _, err := service.ActCoordination(ctx, recipient, "project", "request", test.action, "invalid", test.version, test.input, ""); err == nil {
			t.Fatal("invalid action accepted")
		}
	}
	responded, err := service.ActCoordination(ctx, recipient, "project", "request", "respond", "respond", 1, response, "")
	if err != nil {
		t.Fatal(err)
	}
	if decode[domain.CoordinationRequest](t, responded.Resource).Response.ActorID != recipient.ID {
		t.Fatal("response attribution")
	}
	if replay, err := service.ActCoordination(ctx, recipient, "project", "request", "respond", "respond", 1, response, ""); err != nil || !replay.Replayed {
		t.Fatalf("response retry=%+v err=%v", replay, err)
	}
	if _, err := service.ActCoordination(ctx, outsider, "project", "request", "respond", "respond", 1, response, ""); err == nil {
		t.Fatalf("other actor retry unexpectedly succeeded=%v", err)
	}
	if _, err := service.ActCoordination(ctx, recipient, "project", "request", "respond", "respond-twice", 2, response, ""); err == nil {
		t.Fatal("second response")
	}
	if _, err := service.ActCoordination(ctx, outsider, "project", "request", "resolve", "outside-close", 2, CoordinationActionInput{}, ""); !errors.Is(err, ErrDenied) {
		t.Fatalf("outside close=%v", err)
	}
	closed, err := service.ActCoordination(ctx, owner, "project", "request", "resolve", "resolve", 2, CoordinationActionInput{Note: "Accepted response"}, "")
	if err != nil || decode[domain.CoordinationRequest](t, closed.Resource).Status != "resolved" {
		t.Fatalf("close=%+v err=%v", closed, err)
	}
	if _, err := service.ActCoordination(ctx, owner, "project", "request", "cancel", "closed", 3, CoordinationActionInput{}, ""); err == nil {
		t.Fatal("closed reopened")
	}
	input.ID = "human-request"
	input.Recipient = domain.CoordinationRecipient{Type: "human", ID: "human"}
	for _, reason := range []string{"", "  "} {
		input.EscalationReason = reason
		if _, err := service.CreateCoordination(ctx, owner, "project", "human-no-reason", input, ""); err == nil {
			t.Fatal("human request without explicit reason accepted")
		}
	}
	input.EscalationReason = "Explicit human decision"
	input.EvidenceIDs = nil
	if _, err := service.CreateCoordination(ctx, owner, "project", "human-create", input, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ActCoordination(ctx, actor("human", "resource:write"), "project", input.ID, "respond", "spoof", 1, response, ""); !errors.Is(err, ErrDenied) {
		t.Fatalf("token impersonated human=%v", err)
	}
	human := actor("actual-operator", "resource:write")
	human.Human = true
	result, err := service.ActCoordination(ctx, human, "project", input.ID, "respond", "override", 1, response, "")
	if err != nil || decode[domain.CoordinationRequest](t, result.Resource).Response.ActorID != human.ID {
		t.Fatalf("human override attribution=%+v err=%v", result, err)
	}
	if _, err := service.ActCoordination(ctx, owner, "project", input.ID, "cancel", "cancel", 2, CoordinationActionInput{}, ""); err != nil {
		t.Fatal(err)
	}
	for _, audit := range repo.audits {
		if audit.ResourceType == string(domain.ResourceCoordinationRequest) && len(audit.Details) != 0 {
			t.Fatalf("new audit retained private request contents=%s", audit.Details)
		}
	}
}

func TestTypedRuntimeObservationMetadataAndOwnership(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepository()
	service := NewServiceWithDependencies(repo, nil, testClock{at}, &testIDs{})
	worker := actor("agent:worker", "event:ingest", "source:codex")
	marshal := func(value any) json.RawMessage { raw, _ := json.Marshal(value); return raw }
	agent := domain.Event{ProjectID: "project", SubjectID: "untrusted", SubjectType: domain.ResourceAgent, Type: "agent.observed", SourceSystem: "codex", SourceEventID: "agent", SourceSequence: 1, Payload: marshal(domain.AgentObservation{AgentID: worker.ID, InstanceID: "instance", DisplayName: "Worker", Status: "idle", ObservedAt: at})}
	for name, change := range map[string]func(*domain.Event){"actor": func(e *domain.Event) {
		e.Payload = json.RawMessage(`{"agent_id":"other","instance_id":"instance","display_name":"Worker","status":"idle","observed_at":"2026-10-15T12:00:00Z"}`)
	}, "raw": func(e *domain.Event) {
		e.Payload = json.RawMessage(`{"agent_id":"agent:worker","instance_id":"i","display_name":"Worker","status":"idle","observed_at":"2026-10-15T12:00:00Z","transcript":"private"}`)
	}, "sequence": func(e *domain.Event) { e.SourceSequence = 0 }, "time": func(e *domain.Event) { e.OccurredAt = at.Add(time.Second) }, "status": func(e *domain.Event) {
		e.Payload = marshal(domain.AgentObservation{AgentID: worker.ID, InstanceID: "instance", DisplayName: "Worker", Status: "omnipresent", ObservedAt: at})
	}} {
		t.Run(name, func(t *testing.T) {
			e := agent
			change(&e)
			if err := service.Ingest(ctx, worker, []domain.Event{e}); err == nil {
				t.Fatal("invalid runtime accepted")
			}
		})
	}
	session := domain.Event{ProjectID: "project", SubjectID: "native", SubjectType: domain.ResourceSession, Type: "session.observed", SourceSystem: "codex", SourceEventID: "session", SourceSequence: 1, Payload: marshal(domain.SessionObservation{AgentID: worker.ID, SessionID: "native-thread", Status: "active", ObservedAt: at})}
	run := domain.Event{ProjectID: "project", SubjectID: "native", SubjectType: domain.ResourceRun, Type: "run.observed", SourceSystem: "codex", SourceEventID: "run", SourceSequence: 1, Payload: marshal(domain.RunObservation{AgentID: worker.ID, RunID: "explicit-run", SessionID: "native-thread", Status: "running", ObservedAt: at})}
	if err := service.Ingest(ctx, worker, []domain.Event{session}); err == nil {
		t.Fatal("session without registered self agent")
	}
	badJSON := agent
	badJSON.Payload = append(append(json.RawMessage{}, agent.Payload...), []byte(` {"raw":"trailing"}`)...)
	if err := service.Ingest(ctx, worker, []domain.Event{badJSON}); err == nil {
		t.Fatal("trailing runtime JSON accepted")
	}
	if err := service.Ingest(ctx, worker, []domain.Event{run}); err == nil {
		t.Fatal("missing session accepted")
	}
	if err := service.Ingest(ctx, worker, []domain.Event{agent, session, run}); err != nil {
		t.Fatal(err)
	}
	if len(repo.events) != 3 || repo.events[0].SubjectID != worker.ID || repo.events[1].SubjectID != domain.ObservedRuntimeID("project", "codex", worker.ID, "session", "native-thread") {
		t.Fatalf("runtime identities=%+v", repo.events)
	}
	repo.put("project", domain.ResourceTask, "owned", domain.Task{Base: base("owned", 1, at), OwnerID: worker.ID, Status: "working"})
	if _, err := service.RequestTaskReview(ctx, actor("other", "task:review"), "project", "owned", "", "wrong-review", 1, ""); !errors.Is(err, ErrDenied) {
		t.Fatalf("review ownership=%v", err)
	}
	if _, err := service.RequestTaskReview(ctx, actor(worker.ID, "task:review"), "project", "owned", "", "owned-review", 1, ""); err != nil {
		t.Fatal(err)
	}
}

func TestNarrowRuntimeSourcesRejectLegacyResourceMutations(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepository()
	service := NewServiceWithDependencies(repo, nil, testClock{at}, &testIDs{})
	for _, system := range []string{"codex", "claude", "hermes"} {
		narrow := actor("runtime-self", "event:ingest", "source:"+system)
		for _, target := range []struct {
			kind  domain.ResourceType
			event string
		}{{domain.ResourceTask, "task.completed"}, {domain.ResourceRun, "run.cancelled"}, {domain.ResourceSession, "session.closed"}, {domain.ResourceApproval, "approval.resolved"}, {domain.ResourceAttention, "attention.resolved"}, {domain.ResourceAgent, "agent.disconnected"}} {
			event := domain.Event{ProjectID: "project", SubjectType: target.kind, SubjectID: "someone-elses-resource", Type: target.event, SourceSystem: system, SourceEventID: system + "-" + target.event, SourceSequence: 1, Payload: json.RawMessage(`{}`)}
			if err := service.Ingest(ctx, narrow, []domain.Event{event}); !errors.Is(err, ErrDenied) {
				t.Fatalf("%s %s err=%v", system, target.event, err)
			}
		}
		payload, _ := json.Marshal(domain.AgentObservation{AgentID: narrow.ID, InstanceID: "instance", DisplayName: "Self", Status: "idle", ObservedAt: at})
		valid := domain.Event{ProjectID: "project", SubjectType: domain.ResourceAgent, SubjectID: "native", Type: "agent.observed", SourceSystem: system, SourceEventID: system + "-valid", SourceSequence: 1, Payload: payload}
		bad := domain.Event{ProjectID: "project", SubjectType: domain.ResourceTask, SubjectID: "human-task", Type: "task.completed", SourceSystem: system, SourceEventID: system + "-bad", SourceSequence: 2, Payload: json.RawMessage(`{}`)}
		if err := service.Ingest(ctx, narrow, []domain.Event{valid, bad}); !errors.Is(err, ErrDenied) {
			t.Fatalf("mixed runtime batch err=%v", err)
		}
	}
	if len(repo.events) != 0 {
		t.Fatalf("denied runtime events persisted=%d", len(repo.events))
	}
	legacy := domain.Event{ProjectID: "project", SubjectType: domain.ResourceRun, SubjectID: "legacy-native", Type: "run.started", SourceSystem: "codex", SourceEventID: "privileged-legacy", Payload: json.RawMessage(`{"phase":"start"}`)}
	if err := service.Ingest(ctx, actor("legacy-admin", "event:ingest", "source:admin"), []domain.Event{legacy}); err != nil {
		t.Fatalf("explicit legacy authority rejected=%v", err)
	}
	if len(repo.events) != 1 || repo.events[0].ActorID != "legacy-admin" {
		t.Fatalf("legacy attribution=%+v", repo.events)
	}
}

func TestCoordinationAuditRedactionPreservesStoredHistory(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryRepository()
	service := NewService(repo, nil)
	// This serialized fixture models an already-written candidate audit row.
	legacy := json.RawMessage(`{"id":"legacy-audit","project_id":"project","version":1,"actor_id":"agent:owner","action":"respond.coordination_request","resource_type":"coordination_request","resource_id":"private-request","outcome":"accepted","details":{"input":{"body":"Private historical response","evidence_ids":["private-evidence"]},"title":"Private historical request"},"legacy_attribute":"preserve"}`)
	repo.resources[resourceKey("project", domain.ResourceAudit, "legacy-audit")] = append(json.RawMessage{}, legacy...)
	repo.put("project", domain.ResourceAudit, "ordinary-audit", domain.AuditRecord{Base: domain.Base{ID: "ordinary-audit", ProjectID: "project", Version: 1}, ResourceType: "task", Details: json.RawMessage(`{"status":"ready"}`)})
	for _, reader := range []Actor{actor("audit-only", "audit:read"), actor("operator", "resource:read")} {
		raw, err := service.Get(ctx, reader, "project", domain.ResourceAudit, "legacy-audit")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "Private historical") || strings.Contains(string(raw), "private-evidence") || strings.Contains(string(raw), `"details"`) {
			t.Fatalf("audit get leaked=%s", raw)
		}
		if !strings.Contains(string(raw), "legacy_attribute") || !strings.Contains(string(raw), "agent:owner") {
			t.Fatalf("audit attribution lost=%s", raw)
		}
		page, err := service.List(ctx, reader, "project", domain.ResourceAudit, "", 50)
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(page)
		if strings.Contains(string(encoded), "Private historical") || !strings.Contains(string(encoded), `"status":"ready"`) {
			t.Fatalf("audit list privacy/noncoordination=%s", encoded)
		}
	}
	if string(repo.resources[resourceKey("project", domain.ResourceAudit, "legacy-audit")]) != string(legacy) {
		t.Fatal("stored history rewritten by read")
	}
}

func TestGitHubIngestionIsTypedSourceOnlyAndPreservesSentryIDs(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	repo := newMemoryRepository()
	service := NewServiceWithDependencies(repo, nil, testClock{at}, &testIDs{})
	github := actor("github-monitor", "event:ingest", "source:github")
	valid := domain.Event{ProjectID: "project", SubjectType: domain.ResourceTask, SubjectID: "forged", Type: "task.observed", SourceSystem: "github", SourceEventID: "github-task", SourceSequence: 1, Payload: json.RawMessage(`{"external_id":"repo:42:pull:7","title":"Actual PR metadata","description":"Recorded CI failure","status":"blocked","external_url":"https://github.com/owner/repo/pull/7","dependencies":["repo:42:issue:3"]}`)}
	for _, test := range []struct {
		kind  domain.ResourceType
		event string
	}{{domain.ResourceTask, "task.completed"}, {domain.ResourceApproval, "approval.approved"}, {domain.ResourceRun, "run.observed"}, {domain.ResourceAgent, "agent.observed"}, {domain.ResourceHuman, "human.updated"}, {domain.ResourceCoordinationRequest, "coordination_request.created"}} {
		event := valid
		event.SubjectType = test.kind
		event.Type = test.event
		if err := service.Ingest(ctx, github, []domain.Event{event}); err == nil {
			t.Fatalf("GitHub control mutation accepted=%s", test.event)
		}
		if err := service.Ingest(ctx, actor("privileged-source", "event:ingest", "source:admin"), []domain.Event{event}); err == nil {
			t.Fatalf("source admin bypassed GitHub type restriction=%s", test.event)
		}
	}
	for name, change := range map[string]func(*domain.Event){"sequence": func(e *domain.Event) { e.SourceSequence = 0 }, "replacement": func(e *domain.Event) { e.Payload = json.RawMessage(`{"resource":{"status":"completed"}}`) }, "privileged field": func(e *domain.Event) {
		e.Payload = json.RawMessage(`{"external_id":"id","title":"Title","status":"closed","owner_id":"human"}`)
	}, "unsafe URL": func(e *domain.Event) {
		e.Payload = json.RawMessage(`{"external_id":"id","title":"Title","status":"closed","external_url":"javascript:alert(1)"}`)
	}, "wrong source": func(e *domain.Event) { e.SourceSystem = "sentry" }} {
		t.Run(name, func(t *testing.T) {
			e := valid
			change(&e)
			if err := service.Ingest(ctx, github, []domain.Event{e}); err == nil {
				t.Fatal("invalid source observation accepted")
			}
		})
	}
	valid.ActorID = "forged"
	valid.CommandID = "forged-command"
	if err := service.Ingest(ctx, github, []domain.Event{valid}); err != nil {
		t.Fatal(err)
	}
	if len(repo.events) != 1 || repo.events[0].SubjectID != domain.ObservedTaskIDForSource("project", "github", "repo:42:pull:7") || repo.events[0].ActorID != github.ID || repo.events[0].CommandID != "" {
		t.Fatalf("GitHub identity/attribution=%+v", repo.events)
	}
	observation := domain.ProjectObservation{ExternalID: "repo:42", Name: "owner/repo", Path: "https://github.com/owner/repo", Status: "active", CollectedAt: at, ScanStatus: "complete", TaskCount: 1}
	payload, _ := json.Marshal(observation)
	project := domain.Event{ProjectID: "project", SubjectType: domain.ResourceProject, SubjectID: "project", Type: "project.observed", SourceSystem: "github", SourceEventID: "github-project", SourceSequence: 1, Payload: payload}
	if err := service.Ingest(ctx, github, []domain.Event{project}); err != nil {
		t.Fatal(err)
	}
	if repo.events[1].SubjectID != domain.ObservedProjectIDForSource("project", "github") || repo.events[1].SubjectID == "project" {
		t.Fatalf("GitHub project claimed Sentry report=%+v", repo.events[1])
	}
	observation.Path = "file:///source/project"
	project.Payload, _ = json.Marshal(observation)
	if err := service.Ingest(ctx, github, []domain.Event{project}); err == nil {
		t.Fatal("unsafe GitHub repository path accepted")
	}
	legacy := valid
	legacy.SourceSystem = "sentry"
	legacy.SourceEventID = "sentry-same-native"
	if err := service.Ingest(ctx, actor("sentry", "event:ingest", "source:sentry"), []domain.Event{legacy}); err != nil {
		t.Fatal(err)
	}
	if repo.events[2].SubjectID != domain.ObservedTaskID("project", "repo:42:pull:7") || repo.events[2].SubjectID == repo.events[0].SubjectID {
		t.Fatal("Sentry task identity changed or collided")
	}
}
