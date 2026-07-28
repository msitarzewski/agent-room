//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/msitarzewski/agent-room/internal/app"
	"github.com/msitarzewski/agent-room/internal/auth"
	"github.com/msitarzewski/agent-room/internal/domain"
)

func TestRepositoryCommandEventProjectionRoundTrip(t *testing.T) {
	databaseURL := os.Getenv("AGENTROOM_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("AGENTROOM_TEST_DATABASE_URL is required for integration tests")
	}
	ctx := context.Background()
	repo, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	lockConnection, err := repo.Pool().Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lockConnection.Exec(ctx, "SELECT pg_advisory_lock(671327099)"); err != nil {
		lockConnection.Release()
		t.Fatal(err)
	}
	defer func() {
		_, _ = lockConnection.Exec(context.Background(), "SELECT pg_advisory_unlock(671327099)")
		lockConnection.Release()
	}()
	if err := Migrate(ctx, repo.Pool()); err != nil {
		t.Fatal(err)
	}
	var databaseName string
	if err := repo.Pool().QueryRow(ctx, "SELECT current_database()").Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	if len(databaseName) < 5 || databaseName[len(databaseName)-5:] != "_test" {
		t.Fatalf("refusing destructive integration setup against non-test database %q", databaseName)
	}
	_, err = repo.Pool().Exec(ctx, "TRUNCATE service_tokens,projects RESTART IDENTITY CASCADE")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Pool().Exec(ctx, "INSERT INTO projects(id,name) VALUES('project-1','Integration test')"); err != nil {
		t.Fatal(err)
	}
	service := app.NewService(repo, nil)
	actor := app.Actor{ID: "operator", Capabilities: map[string]struct{}{
		"resource:write": {}, "resource:read": {}, "task:transition": {}, "event:read": {}, "event:ingest": {},
		"approval:request": {}, "approval:decide": {}, "run:cancel": {},
		"source:codex": {}, "source:meter": {},
	}}
	task := domain.Task{Base: domain.Base{ID: "task-1"}, Title: "Verify release", Status: "ready", Priority: "high", Source: domain.Source{System: "test"}}
	created, err := service.Create(ctx, actor, domain.ResourceTask, "project-1", "create-1", task, "")
	if err != nil {
		t.Fatal(err)
	}
	if created.Event.Cursor != 1 {
		t.Fatalf("cursor=%d", created.Event.Cursor)
	}
	transitioned, err := service.TransitionTask(ctx, actor, "project-1", "task-1", "working", "", "transition-1", 1, "")
	if err != nil {
		t.Fatal(err)
	}
	var got domain.Task
	if err := json.Unmarshal(transitioned.Resource, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "working" || got.Version != 2 {
		t.Fatalf("task=%+v", got)
	}
	replayed, err := service.TransitionTask(ctx, actor, "project-1", "task-1", "working", "", "transition-1", 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed {
		t.Fatal("idempotent command was not replayed")
	}
	if _, err := service.TransitionTask(ctx, actor, "project-1", "task-1", "review", "", "transition-2", 1, ""); err != app.ErrVersionConflict {
		t.Fatalf("expected version conflict, got %v", err)
	}
	page, err := service.Events(ctx, actor, "project-1", 0, 10)
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("events=%d err=%v", len(page.Items), err)
	}
	failed := domain.Event{
		ProjectID: "project-1", Type: "run.failed", SubjectType: domain.ResourceRun, SubjectID: "run-native-1",
		ActorID: "attacker", OccurredAt: time.Date(2026, 7, 27, 12, 1, 0, 0, time.UTC), SchemaVersion: 1,
		SourceSystem: "codex", SourceEventID: "native-2", SourceSequence: 2, Payload: json.RawMessage(`{"error_code":"exit_1"}`),
	}
	if err := service.Ingest(ctx, actor, []domain.Event{failed}); err != nil {
		t.Fatal(err)
	}
	started := failed
	started.Type, started.SourceEventID, started.SourceSequence = "run.started", "native-1", 1
	started.Payload = json.RawMessage(`{"phase":"start"}`)
	if err := service.Ingest(ctx, actor, []domain.Event{started}); err != nil {
		t.Fatal(err)
	}
	if err := service.Ingest(ctx, actor, []domain.Event{failed}); err != nil {
		t.Fatalf("duplicate source event should replay cleanly: %v", err)
	}
	runRaw, err := service.Get(ctx, actor, "project-1", domain.ResourceRun, "run-native-1")
	if err != nil {
		t.Fatal(err)
	}
	var run map[string]any
	if err := json.Unmarshal(runRaw, &run); err != nil {
		t.Fatal(err)
	}
	if run["status"] != "failed" {
		t.Fatalf("out-of-order event regressed run projection: %v", run)
	}
	attention, err := service.List(ctx, actor, "project-1", domain.ResourceAttention, "", 10)
	if err != nil || len(attention.Items) != 1 {
		t.Fatalf("attention=%d err=%v", len(attention.Items), err)
	}
	var attributed domain.Event
	if err := repo.Pool().QueryRow(ctx, `SELECT actor_id FROM events WHERE source_system='codex' AND source_event_id='native-2'`).Scan(&attributed.ActorID); err != nil {
		t.Fatal(err)
	}
	if attributed.ActorID != actor.ID {
		t.Fatalf("ingest attribution=%q", attributed.ActorID)
	}

	managedRun := domain.Run{Base: domain.Base{ID: "managed-run"}, Status: "running", Capabilities: []string{"cancel"}}
	if _, err := service.Create(ctx, actor, domain.ResourceRun, "project-1", "run-create", managedRun, ""); err != nil {
		t.Fatal(err)
	}
	requested, err := service.RequestRunApproval(ctx, actor, "project-1", "managed-run", "cancel", "stop", "approval-request", "", 1, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var approval domain.Approval
	if err := json.Unmarshal(requested.Resource, &approval); err != nil {
		t.Fatal(err)
	}
	decided, err := service.DecideApproval(ctx, actor, "project-1", approval.ID, "approved", "", "approval-decision", 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(decided.Resource, &approval); err != nil {
		t.Fatal(err)
	}
	controller := &integrationController{}
	service = app.NewServiceWithController(repo, nil, controller)
	if _, err := service.RunAction(ctx, actor, "project-1", "managed-run", "cancel", "stop", approval.ID, "run-cancel", 1, ""); err != nil {
		t.Fatal(err)
	}
	if controller.calls.Load() != 0 {
		t.Fatal("control side effect ran before committed outbox dispatch")
	}
	found, err := repo.DispatchControlOutbox(ctx, controller)
	if err != nil || !found || controller.calls.Load() != 1 {
		t.Fatalf("control dispatch found=%v calls=%d err=%v", found, controller.calls.Load(), err)
	}
	if found, err = repo.DispatchControlOutbox(ctx, controller); err != nil || found || controller.calls.Load() != 1 {
		t.Fatalf("control replay found=%v calls=%d err=%v", found, controller.calls.Load(), err)
	}
	approvalRaw, err := service.Get(ctx, actor, "project-1", domain.ResourceApproval, approval.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(approvalRaw, &approval); err != nil {
		t.Fatal(err)
	}
	if approval.ConsumedAt == nil || approval.ConsumedBy != actor.ID {
		t.Fatalf("approval was not atomically consumed: %+v", approval)
	}

	claimResults := make(chan error, 2)
	for _, claimant := range []string{"agent-a", "agent-b"} {
		go func(claimant string) {
			claimActor := app.Actor{ID: claimant, Capabilities: map[string]struct{}{"task:claim": {}}}
			_, err := service.ClaimTask(ctx, claimActor, "project-1", "task-1", "claim-"+claimant, 2, "mcp")
			claimResults <- err
		}(claimant)
	}
	successes, conflicts := 0, 0
	for range 2 {
		switch err := <-claimResults; {
		case err == nil:
			successes++
		case err == app.ErrVersionConflict:
			conflicts++
		default:
			t.Fatalf("unexpected concurrent claim result: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent claim outcomes successes=%d conflicts=%d", successes, conflicts)
	}

	brief, err := service.Brief(ctx, actor, "project-1", -1)
	if err != nil {
		t.Fatal(err)
	}
	through, ok := brief["through_cursor"].(int64)
	if !ok || through == 0 {
		t.Fatalf("brief through cursor=%v", brief["through_cursor"])
	}
	briefReplayed, err := service.AcknowledgeBrief(ctx, actor, "project-1", "brief-ack", 0, through)
	if err != nil || briefReplayed {
		t.Fatalf("brief acknowledge replayed=%v err=%v", briefReplayed, err)
	}
	briefReplayed, err = service.AcknowledgeBrief(ctx, actor, "project-1", "brief-ack", 0, through)
	if err != nil || !briefReplayed {
		t.Fatalf("brief acknowledgement replay replayed=%v err=%v", briefReplayed, err)
	}
	brief, err = service.Brief(ctx, actor, "project-1", -1)
	if err != nil || brief["from_cursor"] != through || brief["reviewed_cursor"] != through {
		t.Fatalf("server brief cursor was not actor scoped: brief=%v err=%v", brief, err)
	}

	budget := domain.Budget{
		Base: domain.Base{ID: "budget-1"}, Name: "Project token budget",
		ScopeType: "project", ScopeID: "project-1", EnforcementMode: "enforced",
		Status: "available", TokenLimit: 10,
	}
	if _, err := service.Create(ctx, actor, domain.ResourceBudget, "project-1", "budget-create", budget, ""); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	failures := make(chan error, 2)
	for index := 1; index <= 2; index++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			usage := domain.Event{
				ProjectID: "project-1", Type: "budget.usage_observed", SubjectType: domain.ResourceBudget,
				SubjectID: "budget-1", SourceSystem: "meter", SourceEventID: "usage-" + string(rune('0'+index)),
				Payload: json.RawMessage(`{"token_delta":6}`),
			}
			failures <- service.Ingest(ctx, actor, []domain.Event{usage})
		}(index)
	}
	group.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	budgetRaw, err := service.Get(ctx, actor, "project-1", domain.ResourceBudget, "budget-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(budgetRaw, &budget); err != nil {
		t.Fatal(err)
	}
	if budget.TokenUsed != 12 || budget.Status != "exhausted" {
		t.Fatalf("concurrent budget accounting lost usage: %+v", budget)
	}
	if _, err := service.TransitionTask(ctx, actor, "project-1", "task-1", "review", "", "budget-denied", 3, ""); err != app.ErrBudgetExceeded {
		t.Fatalf("enforced exhausted budget did not deny command: %v", err)
	}

	createdToken, err := auth.CreateServiceTokenWithMetadata(ctx, repo.Pool(), "adapter-token", "Codex adapter", "adapter-codex",
		[]string{"project-1"}, []string{"event:ingest", "source:codex"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if createdToken.Token == "" || createdToken.ExpiresAt.Before(time.Now().Add(29*24*time.Hour)) ||
		createdToken.ExpiresAt.After(time.Now().Add(31*24*time.Hour)) {
		t.Fatalf("service token metadata has invalid bounded default expiry: %+v", createdToken)
	}
	metadata, err := auth.ListServiceTokens(ctx, repo.Pool())
	if err != nil || len(metadata) != 1 || metadata[0].ID != createdToken.ID {
		t.Fatalf("service token metadata=%+v err=%v", metadata, err)
	}
	revokeReplayed, err := auth.RevokeServiceToken(ctx, repo.Pool(), createdToken.ID)
	if err != nil || revokeReplayed {
		t.Fatalf("first service-token revocation replayed=%v err=%v", revokeReplayed, err)
	}
	revokeReplayed, err = auth.RevokeServiceToken(ctx, repo.Pool(), createdToken.ID)
	if err != nil || !revokeReplayed {
		t.Fatalf("idempotent service-token revocation replayed=%v err=%v", revokeReplayed, err)
	}
}

type integrationController struct {
	calls atomic.Int64
}

func (*integrationController) Supports(string, string) bool { return true }
func (c *integrationController) Execute(context.Context, string, string, string) error {
	c.calls.Add(1)
	return nil
}
