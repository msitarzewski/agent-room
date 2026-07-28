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
	result := CommandResult{Resource: mutation.Resource, Event: mutation.Event}
	r.results[idempotencyScope], r.fingerprints[idempotencyScope] = result, fingerprint
	return result, nil
}

func strconv64(value int64) string { return fmt.Sprintf("%d", value) }
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
	repo.put("project", domain.ResourceTask, "task", domain.Task{Base: base("task", 1, now), Status: "ready"})
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
	ingester := actor("adapter", "event:ingest", "source:hermes")
	valid := domain.Event{ProjectID: "project", Type: "run.updated", SubjectType: domain.ResourceRun, SubjectID: "run", SourceSystem: "hermes", SourceEventID: "native", Payload: json.RawMessage(`{"ok":true}`)}

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
