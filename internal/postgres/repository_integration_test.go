//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		// This operator fixture explicitly exercises privileged legacy event compatibility.
		"source:admin": {},
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
		ActorID: "attacker", OccurredAt: time.Now().UTC().Truncate(time.Second).Add(-time.Minute), SchemaVersion: 1,
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
	if err := service.Ingest(ctx, actor, []domain.Event{{ProjectID: "project-1", Type: "budget.rate_limit_observed", SubjectType: domain.ResourceBudget,
		SubjectID: "budget-1", SourceSystem: "meter", SourceEventID: "budget-rate-limit", Payload: json.RawMessage(`{}`)}}); err != nil {
		t.Fatal(err)
	}
	budgetRaw, err = service.Get(ctx, actor, "project-1", domain.ResourceBudget, "budget-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(budgetRaw, &budget); err != nil {
		t.Fatal(err)
	}
	if budget.Status != "exhausted" || budget.TokenUsed != 12 {
		t.Fatalf("source event cleared enforced budget: %+v", budget)
	}
	if err := service.Ingest(ctx, actor, []domain.Event{{ProjectID: "project-1", Type: "approval.requested", SubjectType: domain.ResourceApproval,
		SubjectID: "source-approval", SourceSystem: "codex", SourceEventID: "source-approval", Payload: json.RawMessage(`{}`)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, actor, "project-1", domain.ResourceApproval, "source-approval"); err != app.ErrNotFound {
		t.Fatalf("source created privileged approval: %v", err)
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
	t.Run("workspace includes every resource and isolates source observations", func(t *testing.T) {
		if _, err := repo.Pool().Exec(ctx, `INSERT INTO projects(id,name) VALUES('workspace-project','Workspace'),('empty-project','Empty')`); err != nil {
			t.Fatal(err)
		}
		sentry := app.Actor{ID: "sentry", Capabilities: map[string]struct{}{"event:ingest": {}, "source:sentry": {}}}
		events := make([]domain.Event, 260)
		for index := range events {
			payload, _ := json.Marshal(domain.TaskObservation{ExternalID: fmt.Sprintf("task-%03d", index), Title: "Observed work", Status: "completed"})
			events[index] = domain.Event{ProjectID: "workspace-project", Type: "task.observed", SubjectType: domain.ResourceTask,
				SubjectID: "untrusted", SourceSystem: "sentry", SourceEventID: fmt.Sprintf("scan-1-%d", index), SourceSequence: 1,
				OccurredAt: time.Now().UTC().Add(-time.Hour), Payload: payload}
		}
		if err := service.Ingest(ctx, sentry, events); err != nil {
			t.Fatal(err)
		}
		snapshot, err := service.Workspace(ctx, actor, "workspace-project")
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshot.Resources.Tasks) != 260 || len(snapshot.Sources) != 1 || snapshot.Sources[0].System != "sentry" || snapshot.Sources[0].LastSeen.Before(time.Now().Add(-time.Minute)) {
			t.Fatalf("incomplete workspace tasks=%d sources=%+v", len(snapshot.Resources.Tasks), snapshot.Sources)
		}
		for _, raw := range snapshot.Resources.Tasks {
			var task domain.Task
			if err := json.Unmarshal(raw, &task); err != nil {
				t.Fatal(err)
			}
			if task.ProjectID != "workspace-project" || task.Status != "inbox" || task.SourceStatus != "completed" {
				t.Fatalf("leaked or accepted observation: %+v", task)
			}
		}
		taskID := domain.ObservedTaskID("workspace-project", "task-000")
		if _, err := service.TransitionTask(ctx, actor, "workspace-project", taskID, "ready", "", "observed-ready", 1, ""); err != nil {
			t.Fatal(err)
		}
		newer := events[0]
		newer.SourceSequence, newer.SourceEventID = 3_000_000_000, "scan-3-0"
		newer.Payload = json.RawMessage(`{"external_id":"task-000","title":"New observation","status":"blocked"}`)
		if err := service.Ingest(ctx, sentry, []domain.Event{newer}); err != nil {
			t.Fatal(err)
		}
		older := newer
		older.SourceSequence, older.SourceEventID = 2, "scan-2-0"
		older.Payload = json.RawMessage(`{"external_id":"task-000","title":"Old observation","status":"completed"}`)
		if err := service.Ingest(ctx, sentry, []domain.Event{older}); err != nil {
			t.Fatal(err)
		}
		if err := service.Ingest(ctx, sentry, []domain.Event{newer}); err != nil {
			t.Fatal(err)
		}
		raw, err := service.Get(ctx, actor, "workspace-project", domain.ResourceTask, taskID)
		if err != nil {
			t.Fatal(err)
		}
		var task domain.Task
		if err := json.Unmarshal(raw, &task); err != nil {
			t.Fatal(err)
		}
		if task.Status != "ready" || task.SourceStatus != "blocked" || task.Version != 3 || task.Title != "Observed work" {
			t.Fatalf("observation changed supervisor state or regressed: %+v", task)
		}
		attentionID := "attn_" + stableID("workspace-project", sourceTaskBlocked, string(domain.ResourceTask), taskID)
		attentionRaw, err := service.Get(ctx, actor, "workspace-project", domain.ResourceAttention, attentionID)
		if err != nil {
			t.Fatal(err)
		}
		var sourceAttention domain.Attention
		if err := json.Unmarshal(attentionRaw, &sourceAttention); err != nil {
			t.Fatal(err)
		}
		if sourceAttention.Kind != "source.task.blocked" || sourceAttention.Status != "open" || sourceAttention.Title != "Source reports blocked: New observation" || sourceAttention.Version != 1 {
			t.Fatalf("source attention=%+v", sourceAttention)
		}
		if _, err := service.Create(ctx, actor, domain.ResourceAttention, "workspace-project", "human-attention-create", domain.Attention{Base: domain.Base{ID: "human-attention"}, Kind: "review", Severity: "high", Title: "Human review", ResourceType: "task", ResourceID: taskID, Status: "open"}, ""); err != nil {
			t.Fatal(err)
		}
		repeated := newer
		repeated.SourceSequence, repeated.SourceEventID = 3_000_000_001, "blocked-repeat"
		repeated.Payload = json.RawMessage(`{"external_id":"task-000","title":"New observation","status":" Blocked "}`)
		if err := service.Ingest(ctx, sentry, []domain.Event{repeated}); err != nil {
			t.Fatal(err)
		}
		attentionRaw, err = service.Get(ctx, actor, "workspace-project", domain.ResourceAttention, attentionID)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(attentionRaw, &sourceAttention); err != nil {
			t.Fatal(err)
		}
		if sourceAttention.Version != 1 || sourceAttention.Status != "open" {
			t.Fatalf("repeat duplicated source attention: %+v", sourceAttention)
		}
		unblocked := repeated
		unblocked.SourceSequence, unblocked.SourceEventID = 3_000_000_002, "source-unblocked"
		unblocked.Payload = json.RawMessage(`{"external_id":"task-000","title":"New observation","status":"Upcoming"}`)
		if err := service.Ingest(ctx, sentry, []domain.Event{unblocked}); err != nil {
			t.Fatal(err)
		}
		staleBlocked := repeated
		staleBlocked.SourceEventID = "stale-source-blocked"
		if err := service.Ingest(ctx, sentry, []domain.Event{staleBlocked}); err != nil {
			t.Fatal(err)
		}
		attentionRaw, err = service.Get(ctx, actor, "workspace-project", domain.ResourceAttention, attentionID)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(attentionRaw, &sourceAttention); err != nil {
			t.Fatal(err)
		}
		if sourceAttention.Status != "resolved" || sourceAttention.Version != 2 || sourceAttention.ResolvedAt == nil {
			t.Fatalf("source attention did not resolve or stale source reopened it: %+v", sourceAttention)
		}
		humanRaw, err := service.Get(ctx, actor, "workspace-project", domain.ResourceAttention, "human-attention")
		if err != nil {
			t.Fatal(err)
		}
		var humanAttention domain.Attention
		if err := json.Unmarshal(humanRaw, &humanAttention); err != nil {
			t.Fatal(err)
		}
		if humanAttention.Status != "open" || humanAttention.Version != 1 {
			t.Fatalf("source changed human attention: %+v", humanAttention)
		}
		taskRaw, err := service.Get(ctx, actor, "workspace-project", domain.ResourceTask, taskID)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(taskRaw, &task); err != nil {
			t.Fatal(err)
		}
		if task.Status != "ready" || task.SourceStatus != "Upcoming" {
			t.Fatalf("source attention lifecycle changed supervised task: %+v", task)
		}
		newEpisode := repeated
		newEpisode.SourceSequence, newEpisode.SourceEventID = 3_000_000_003, "new-blocked-episode"
		if err := service.Ingest(ctx, sentry, []domain.Event{newEpisode}); err != nil {
			t.Fatal(err)
		}
		attentionRaw, err = service.Get(ctx, actor, "workspace-project", domain.ResourceAttention, attentionID)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(attentionRaw, &sourceAttention); err != nil {
			t.Fatal(err)
		}
		if sourceAttention.Status != "open" || sourceAttention.Version != 3 {
			t.Fatalf("new source blockage did not reopen attention: %+v", sourceAttention)
		}
		humanResolver := app.Actor{ID: "human", Capabilities: map[string]struct{}{"attention:manage": {}}}
		if _, err := service.SetAttentionStatus(ctx, humanResolver, "workspace-project", attentionID, "resolved", "human-resolves-source-attention", 3, ""); err != nil {
			t.Fatal(err)
		}
		stillBlocked := newEpisode
		stillBlocked.SourceSequence, stillBlocked.SourceEventID = 3_000_000_004, "still-blocked-after-human-resolution"
		stillBlocked.Payload = json.RawMessage(`{"external_id":"task-000","title":"Newer source title","status":"Blocked"}`)
		if err := service.Ingest(ctx, sentry, []domain.Event{stillBlocked}); err != nil {
			t.Fatal(err)
		}
		attentionRaw, err = service.Get(ctx, actor, "workspace-project", domain.ResourceAttention, attentionID)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(attentionRaw, &sourceAttention); err != nil {
			t.Fatal(err)
		}
		if sourceAttention.Status != "resolved" || sourceAttention.Version != 4 {
			t.Fatalf("still-blocked source undid human resolution: %+v", sourceAttention)
		}
		unblocked.SourceSequence, unblocked.SourceEventID = 3_000_000_005, "source-unblocked-after-human-resolution"
		if err := service.Ingest(ctx, sentry, []domain.Event{unblocked}); err != nil {
			t.Fatal(err)
		}
		newEpisode.SourceSequence, newEpisode.SourceEventID = 3_000_000_006, "new-blocked-episode-after-human-resolution"
		if err := service.Ingest(ctx, sentry, []domain.Event{newEpisode}); err != nil {
			t.Fatal(err)
		}
		attentionRaw, err = service.Get(ctx, actor, "workspace-project", domain.ResourceAttention, attentionID)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(attentionRaw, &sourceAttention); err != nil {
			t.Fatal(err)
		}
		if sourceAttention.Status != "open" || sourceAttention.Version != 5 {
			t.Fatalf("true new blocked episode did not reopen human-resolved attention: %+v", sourceAttention)
		}
		for iteration := 0; iteration < 10; iteration++ {
			externalID := fmt.Sprintf("concurrent-first-%d", iteration)
			start := make(chan struct{})
			failures := make(chan error, 2)
			for sequence := 1; sequence <= 2; sequence++ {
				go func(sequence int) {
					<-start
					payload, _ := json.Marshal(domain.TaskObservation{ExternalID: externalID, Title: "Concurrent observation", Status: fmt.Sprintf("sequence-%d", sequence)})
					failures <- service.Ingest(ctx, sentry, []domain.Event{{ProjectID: "workspace-project", Type: "task.observed", SubjectType: domain.ResourceTask, SubjectID: "untrusted", SourceSystem: "sentry", SourceEventID: fmt.Sprintf("%s-%d", externalID, sequence), SourceSequence: int64(sequence), Payload: payload}})
				}(sequence)
			}
			close(start)
			for range 2 {
				if err := <-failures; err != nil {
					t.Fatal(err)
				}
			}
			raw, err := service.Get(ctx, actor, "workspace-project", domain.ResourceTask, domain.ObservedTaskID("workspace-project", externalID))
			if err != nil {
				t.Fatal(err)
			}
			var task domain.Task
			if err := json.Unmarshal(raw, &task); err != nil {
				t.Fatal(err)
			}
			if task.SourceStatus != "sequence-2" {
				t.Fatalf("concurrent first observation regressed: %+v", task)
			}
		}

		empty, err := service.Workspace(ctx, actor, "empty-project")
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(empty)
		var fields map[string]any
		if err := json.Unmarshal(encoded, &fields); err != nil {
			t.Fatal(err)
		}
		resources := fields["resources"].(map[string]any)
		for name, value := range resources {
			array, ok := value.([]any)
			if !ok || len(array) != 0 {
				t.Fatalf("empty %s=%v", name, value)
			}
		}
		if empty.Cursor != 0 || len(empty.Sources) != 0 || empty.GeneratedAt.IsZero() {
			t.Fatalf("empty workspace=%+v", empty)
		}
		if _, err := service.Workspace(ctx, actor, "missing-project"); err != app.ErrNotFound {
			t.Fatalf("missing project err=%v", err)
		}
		if _, err := service.Create(ctx, actor, domain.ResourceClaim, "workspace-project", "rebuild-claim", domain.Claim{Base: domain.Base{ID: "human-claim"}, TaskID: taskID, AgentID: "human", Status: "pending"}, ""); err != nil {
			t.Fatal(err)
		}
		latestTaskRaw, err := service.Get(ctx, actor, "workspace-project", domain.ResourceTask, taskID)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(latestTaskRaw, &task); err != nil {
			t.Fatal(err)
		}
		if _, err := service.TransitionTask(ctx, actor, "workspace-project", taskID, "working", "Manual update after the latest source observation", "last-task-command-before-rebuild", task.Version, ""); err != nil {
			t.Fatal(err)
		}
		if err := repo.RebuildProjections(ctx); err != nil {
			t.Fatal(err)
		}
		rebuiltRaw, err := service.Get(ctx, actor, "workspace-project", domain.ResourceTask, taskID)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(rebuiltRaw, &task); err != nil {
			t.Fatal(err)
		}
		if task.Status != "working" || task.SourceStatus != " Blocked " || task.Title != "Observed work" {
			t.Fatalf("rebuild lost manual/source task state: %+v", task)
		}
		var retainedSource string
		var retainedSequence int64
		if err := repo.Pool().QueryRow(ctx, `SELECT source_system,source_sequence FROM resources WHERE project_id='workspace-project' AND kind='task' AND id=$1`, taskID).Scan(&retainedSource, &retainedSequence); err != nil {
			t.Fatal(err)
		}
		if retainedSource != "sentry" || retainedSequence != 3_000_000_006 {
			t.Fatalf("rebuild lost source watermark: %s/%d", retainedSource, retainedSequence)
		}
		staleAfterRebuild := unblocked
		staleAfterRebuild.SourceEventID = "stale-source-after-rebuild"
		if err := service.Ingest(ctx, sentry, []domain.Event{staleAfterRebuild}); err != nil {
			t.Fatal(err)
		}
		attentionRaw, err = service.Get(ctx, actor, "workspace-project", domain.ResourceAttention, attentionID)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(attentionRaw, &sourceAttention); err != nil {
			t.Fatal(err)
		}
		if sourceAttention.Status != "open" || sourceAttention.Version != 5 {
			t.Fatalf("rebuild allowed stale source attention: %+v", sourceAttention)
		}
		humanRaw, err = service.Get(ctx, actor, "workspace-project", domain.ResourceAttention, "human-attention")
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(humanRaw, &humanAttention); err != nil {
			t.Fatal(err)
		}
		if humanAttention.Status != "open" || humanAttention.Title != "Human review" {
			t.Fatalf("rebuild lost human attention: %+v", humanAttention)
		}
		rebuiltApprovalRaw, err := service.Get(ctx, actor, "project-1", domain.ResourceApproval, approval.ID)
		if err != nil {
			t.Fatalf("rebuild lost trusted approval: %v", err)
		}
		var rebuiltApproval domain.Approval
		if err := json.Unmarshal(rebuiltApprovalRaw, &rebuiltApproval); err != nil {
			t.Fatal(err)
		}
		if rebuiltApproval.ConsumedAt == nil || rebuiltApproval.ConsumedBy != approval.ConsumedBy || rebuiltApproval.Version != approval.Version {
			t.Fatalf("rebuild lost approval consumption: %+v", rebuiltApproval)
		}
		if _, err := service.Get(ctx, actor, "workspace-project", domain.ResourceClaim, "human-claim"); err != nil {
			t.Fatalf("rebuild lost trusted claim: %v", err)
		}
	})

	t.Run("resource pagination visits every tied timestamp exactly once", func(t *testing.T) {
		const project = "pagination-project"
		if _, err := repo.Pool().Exec(ctx, `INSERT INTO projects(id,name) VALUES($1,'Pagination regression')`, project); err != nil {
			t.Fatal(err)
		}
		at := time.Now().UTC().Truncate(time.Second)
		observations := make([]domain.Event, 123)
		expected := map[string]bool{}
		for i := range observations {
			externalID := fmt.Sprintf("pagination-task-%03d", i)
			expected[domain.ObservedTaskID(project, externalID)] = true
			payload, _ := json.Marshal(domain.TaskObservation{ExternalID: externalID, Title: "Pagination task", Status: "Upcoming"})
			observations[i] = domain.Event{ProjectID: project, Type: "task.observed", SubjectType: domain.ResourceTask, SubjectID: "source", SourceSystem: "sentry", SourceEventID: externalID, SourceSequence: 1, OccurredAt: at, Payload: payload}
		}
		sentry := app.Actor{ID: "pagination-sentry", Capabilities: map[string]struct{}{"event:ingest": {}, "source:sentry": {}}}
		if err := service.Ingest(ctx, sentry, observations); err != nil {
			t.Fatal(err)
		}
		seen := map[string]int{}
		cursor := ""
		for pageNumber := 0; pageNumber < 10; pageNumber++ {
			page, err := service.List(ctx, actor, project, domain.ResourceTask, cursor, 25)
			if err != nil {
				t.Fatal(err)
			}
			for _, raw := range page.Items {
				var task domain.Task
				if err := json.Unmarshal(raw, &task); err != nil {
					t.Fatal(err)
				}
				if !expected[task.ID] {
					t.Fatalf("unexpected paginated task %q", task.ID)
				}
				seen[task.ID]++
			}
			if page.NextCursor == "" {
				break
			}
			if page.NextCursor == cursor {
				t.Fatal("pagination cursor did not advance")
			}
			cursor = page.NextCursor
		}
		if len(seen) != len(expected) {
			t.Fatalf("pagination skipped resources: got=%d want=%d", len(seen), len(expected))
		}
		for id := range expected {
			if seen[id] != 1 {
				t.Fatalf("task %s visited %d times", id, seen[id])
			}
		}
	})

	t.Run("workspace cursor and projection agree during concurrent commands", func(t *testing.T) {
		const project = "workspace-concurrent"
		if _, err := repo.Pool().Exec(ctx, `INSERT INTO projects(id,name) VALUES($1,'Concurrent workspace')`, project); err != nil {
			t.Fatal(err)
		}
		if _, err := service.Create(ctx, actor, domain.ResourceTask, project, "concurrent-task-create",
			domain.Task{Base: domain.Base{ID: "task"}, Title: "Concurrent task", Status: "ready", Priority: "normal", Dependencies: []string{}}, ""); err != nil {
			t.Fatal(err)
		}
		failures := make(chan error, 1)
		go func() {
			for i := 0; i < 40; i++ {
				status := "working"
				if i%2 == 1 {
					status = "blocked"
				}
				if _, err := service.TransitionTask(ctx, actor, project, "task", status, "Concurrency test transition", fmt.Sprintf("concurrent-transition-%d", i), int64(i+1), ""); err != nil {
					failures <- err
					return
				}
			}
			failures <- nil
		}()
		for i := 0; i < 40; i++ {
			snapshot, err := service.Workspace(ctx, actor, project)
			if err != nil {
				t.Fatal(err)
			}
			var task domain.Task
			if len(snapshot.Resources.Tasks) != 1 {
				t.Fatalf("tasks=%d", len(snapshot.Resources.Tasks))
			}
			if err := json.Unmarshal(snapshot.Resources.Tasks[0], &task); err != nil {
				t.Fatal(err)
			}
			var eventVersion int64
			if err := repo.Pool().QueryRow(ctx, `SELECT (payload->'resource'->>'version')::bigint FROM events WHERE project_id=$1 AND cursor=$2`, project, snapshot.Cursor).Scan(&eventVersion); err != nil {
				t.Fatal(err)
			}
			if eventVersion != task.Version {
				t.Fatalf("snapshot cursor=%d event version=%d task version=%d", snapshot.Cursor, eventVersion, task.Version)
			}
		}
		if err := <-failures; err != nil {
			t.Fatal(err)
		}
	})

	t.Run("operator ensures preserve canonical project and unchanged sessions", func(t *testing.T) {
		const project = "ensure-project"
		for _, input := range []struct{ id, name string }{{"", "Name"}, {"id", ""}} {
			if _, err := repo.EnsureProject(ctx, input.id, input.name); err == nil {
				t.Fatal("invalid ensure accepted")
			}
		}
		created, err := repo.EnsureProject(ctx, project, "Canonical project")
		if err != nil || !created {
			t.Fatalf("first ensure created=%v err=%v", created, err)
		}
		created, err = repo.EnsureProject(ctx, project, "Source cannot rename")
		if err != nil || created {
			t.Fatalf("repeated ensure created=%v err=%v", created, err)
		}
		var name string
		var audits int
		if err := repo.Pool().QueryRow(ctx, `SELECT name FROM projects WHERE id=$1`, project).Scan(&name); err != nil || name != "Canonical project" {
			t.Fatalf("name=%q err=%v", name, err)
		}
		if err := repo.Pool().QueryRow(ctx, `SELECT count(*) FROM audit_records WHERE project_id=$1 AND action='project.ensure'`, project).Scan(&audits); err != nil || audits != 1 {
			t.Fatalf("ensure audits=%d err=%v", audits, err)
		}
		caps := []string{"resource:read", "overview:read"}
		if err := repo.GrantMembership(ctx, project, "ensure-user", "ensure@example.test", caps); err != nil {
			t.Fatal(err)
		}
		manager, err := auth.New(ctx, repo.Pool(), []byte("membership session test secret"), false, "http://127.0.0.1", "", "", "", "")
		if err != nil {
			t.Fatal(err)
		}
		user := auth.User{ID: "oidc:ensure-user"}
		createSession := func() {
			t.Helper()
			if _, _, err := manager.CreateSession(ctx, user, "ensure-user"); err != nil {
				t.Fatal(err)
			}
		}
		assertSessions := func(want int) {
			t.Helper()
			var count int
			if err := repo.Pool().QueryRow(ctx, `SELECT count(*) FROM web_sessions WHERE user_id=$1`, user.ID).Scan(&count); err != nil || count != want {
				t.Fatalf("sessions=%d want=%d err=%v", count, want, err)
			}
		}
		createSession()
		if err := repo.GrantMembership(ctx, project, "ensure-user", "ensure@example.test", []string{"overview:read", "resource:read"}); err != nil {
			t.Fatal(err)
		}
		assertSessions(1)
		if err := repo.Pool().QueryRow(ctx, `SELECT count(*) FROM audit_records WHERE project_id=$1 AND action='membership.grant'`, project).Scan(&audits); err != nil || audits != 1 {
			t.Fatalf("no-op grant audits=%d err=%v", audits, err)
		}
		if err := repo.GrantMembership(ctx, project, "ensure-user", "ensure@example.test", []string{"resource:read"}); err != nil {
			t.Fatal(err)
		}
		assertSessions(0)
		createSession()
		if err := repo.GrantMembership(ctx, project, "ensure-user", "changed@example.test", []string{"resource:read"}); err != nil {
			t.Fatal(err)
		}
		assertSessions(0)
	})

	t.Run("zero-task source project tracks collection receipt and accepted scan order", func(t *testing.T) {
		const project = "observed-empty-project"
		if _, err := repo.EnsureProject(ctx, project, "Human project name"); err != nil {
			t.Fatal(err)
		}
		sentry := app.Actor{ID: "sentry-project-reader", Capabilities: map[string]struct{}{"event:ingest": {}, "source:sentry": {}}}
		collected := time.Now().UTC().Add(-20 * time.Hour).Truncate(time.Microsecond)
		observation := domain.ProjectObservation{ExternalID: "Software/empty-project", Name: "Source project name", Path: "/source/empty-project", Status: "Active", Reason: "Recent source activity", CollectedAt: collected, ScanStatus: "complete", TaskCount: 0}
		publish := func(sequence int64, id string, value domain.ProjectObservation) error {
			payload, err := json.Marshal(value)
			if err != nil {
				return err
			}
			return service.Ingest(ctx, sentry, []domain.Event{{ProjectID: project, Type: "project.observed", SubjectType: domain.ResourceProject, SubjectID: "untrusted", SourceSystem: "sentry", SourceEventID: id, SourceSequence: sequence, Payload: payload}})
		}
		if err := publish(10, "empty-scan-complete", observation); err != nil {
			t.Fatal(err)
		}
		initial, err := repo.Workspace(ctx, project)
		if err != nil || len(initial.Resources.Tasks) != 0 || len(initial.Sources) != 1 {
			t.Fatalf("zero-task snapshot=%+v err=%v", initial, err)
		}
		source := initial.Sources[0]
		if source.LastCollectedAt == nil || !source.LastCollectedAt.Equal(collected) || source.LastSuccessfulCollectionAt == nil || !source.LastSuccessfulCollectionAt.Equal(collected) || source.ScanStatus == nil || *source.ScanStatus != "complete" || !source.LastReceivedAt.Equal(source.LastSeen) || source.LastReceivedAt.Sub(collected) < 19*time.Hour || source.ProjectObservation.TaskCount != 0 {
			t.Fatalf("collection freshness=%+v", source)
		}
		if err := publish(10, "empty-scan-complete", observation); err != nil {
			t.Fatalf("immutable retry: %v", err)
		}
		retried, err := repo.Workspace(ctx, project)
		if err != nil || retried.Cursor != initial.Cursor || !retried.Sources[0].LastReceivedAt.Equal(source.LastReceivedAt) {
			t.Fatalf("retry changed watermark/receipt: %+v err=%v", retried, err)
		}
		partial := observation
		partial.CollectedAt, partial.ScanStatus, partial.ScanError = collected.Add(time.Hour), "partial", "Older or unreadable records retained"
		if err := publish(11, "empty-scan-partial", partial); err != nil {
			t.Fatal(err)
		}
		stale := observation
		stale.CollectedAt = collected.Add(2 * time.Hour)
		if err := publish(9, "empty-scan-stale", stale); err != nil {
			t.Fatal(err)
		}
		check := func() {
			t.Helper()
			snapshot, err := repo.Workspace(ctx, project)
			if err != nil || len(snapshot.Sources) != 1 {
				t.Fatalf("snapshot=%+v err=%v", snapshot, err)
			}
			source := snapshot.Sources[0]
			if source.ScanStatus == nil || *source.ScanStatus != "partial" || source.LastCollectedAt == nil || !source.LastCollectedAt.Equal(partial.CollectedAt) || source.LastSuccessfulCollectionAt == nil || !source.LastSuccessfulCollectionAt.Equal(collected) {
				t.Fatalf("stale observation changed accepted freshness: %+v", source)
			}
			overview, err := repo.Overview(ctx, project)
			if err != nil {
				t.Fatal(err)
			}
			freshness := overview["sources"].([]app.WorkspaceSource)
			if len(freshness) != 1 || !freshness[0].LastCollectedAt.Equal(*source.LastCollectedAt) || overview["event_cursor"] != snapshot.Cursor {
				t.Fatalf("overview inconsistent: %+v", overview)
			}
			raw, err := repo.Get(ctx, project, domain.ResourceProject, project)
			if err != nil {
				t.Fatal(err)
			}
			var projected domain.Project
			if err := json.Unmarshal(raw, &projected); err != nil {
				t.Fatal(err)
			}
			if projected.Name != "Human project name" || projected.SourceObservation.Name != "Source project name" || projected.SourceStatus != "Active" || projected.Version != 2 {
				t.Fatalf("source overwrote canonical project: %+v", projected)
			}
		}
		check()
		if err := repo.RebuildProjections(ctx); err != nil {
			t.Fatal(err)
		}
		check()
		other, err := repo.Workspace(ctx, "ensure-project")
		if err != nil || len(other.Sources) != 0 {
			t.Fatalf("cross-project freshness leak: %+v err=%v", other, err)
		}
		// A different native identity cannot take over this project's source projection.
		conflict := observation
		conflict.ExternalID = "another-project"
		if err := publish(12, "identity-conflict", conflict); err == nil {
			t.Fatal("source identity takeover accepted")
		}
	})

	t.Run("coordination commands runtime identities and recipient queries survive rebuild", func(t *testing.T) {
		const project = "coordination-project"
		if _, err := repo.EnsureProject(ctx, project, "Coordination integration"); err != nil {
			t.Fatal(err)
		}
		worker := app.Actor{ID: "agent:coord-owner", Capabilities: map[string]struct{}{"event:ingest": {}, "source:codex": {}, "coordination:create": {}, "coordination:read": {}, "coordination:resolve": {}, "coordination:cancel": {}, "overview:read": {}}}
		recipient := app.Actor{ID: "agent:coord-recipient", Capabilities: map[string]struct{}{"event:ingest": {}, "source:codex": {}, "coordination:respond": {}, "coordination:read": {}}}
		at := time.Now().UTC().Truncate(time.Microsecond)
		marshal := func(v any) json.RawMessage { raw, _ := json.Marshal(v); return raw }
		presence := func(who app.Actor) domain.Event {
			return domain.Event{ProjectID: project, SubjectType: domain.ResourceAgent, SubjectID: "native", Type: "agent.observed", SourceSystem: "codex", SourceEventID: who.ID + "-presence", SourceSequence: 10, Payload: marshal(domain.AgentObservation{AgentID: who.ID, InstanceID: "instance", DisplayName: who.ID + " Display", Status: "idle", ObservedAt: at})}
		}
		if err := service.Ingest(ctx, worker, []domain.Event{presence(worker)}); err != nil {
			t.Fatal(err)
		}
		if err := service.Ingest(ctx, recipient, []domain.Event{presence(recipient)}); err != nil {
			t.Fatal(err)
		}
		if _, err := service.Create(ctx, actor, domain.ResourceTask, project, "coord-task-create", domain.Task{Base: domain.Base{ID: "coord-task"}, Title: "Human-owned wording", Description: "Keep human description", OwnerID: worker.ID, Status: "working", Priority: "normal", Dependencies: []string{}}, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := service.Create(ctx, actor, domain.ResourceEvidence, project, "coord-evidence-create", domain.Evidence{Base: domain.Base{ID: "coord-evidence"}, TaskID: "coord-task", Kind: "test", Summary: "Fixture evidence", Source: domain.Source{System: "test"}}, ""); err != nil {
			t.Fatal(err)
		}
		session := domain.Event{ProjectID: project, SubjectType: domain.ResourceSession, SubjectID: "native", Type: "session.observed", SourceSystem: "codex", SourceEventID: "coord-session", SourceSequence: 10, Payload: marshal(domain.SessionObservation{AgentID: worker.ID, SessionID: "real-explicit-test-thread", Status: "active", ObservedAt: at})}
		run := domain.Event{ProjectID: project, SubjectType: domain.ResourceRun, SubjectID: "native", Type: "run.observed", SourceSystem: "codex", SourceEventID: "coord-run", SourceSequence: 10, Payload: marshal(domain.RunObservation{AgentID: worker.ID, RunID: "explicit-fixture-run", SessionID: "real-explicit-test-thread", TaskID: "coord-task", Status: "running", Summary: "Explicitly reported fixture run", ObservedAt: at})}
		if err := service.Ingest(ctx, worker, []domain.Event{session, run}); err != nil {
			t.Fatal(err)
		}
		stale := presence(worker)
		stale.SourceEventID = "coord-stale"
		stale.SourceSequence = 9
		stale.Payload = marshal(domain.AgentObservation{AgentID: worker.ID, InstanceID: "instance", DisplayName: "Stale overwrite", Status: "disconnected", ObservedAt: at.Add(time.Second)})
		if err := service.Ingest(ctx, worker, []domain.Event{stale}); err != nil {
			t.Fatal(err)
		}
		input := app.CreateCoordinationInput{ID: "coord-request", Kind: "review", Title: "Check evidence", Body: "Actual request context", Recipient: domain.CoordinationRecipient{Type: "agent", ID: recipient.ID}, TaskID: "coord-task", EvidenceIDs: []string{"coord-evidence"}}
		created, err := service.CreateCoordination(ctx, worker, project, "coord-create", input, "")
		if err != nil {
			t.Fatal(err)
		}
		if retried, err := service.CreateCoordination(ctx, worker, project, "coord-create", input, ""); err != nil || !retried.Replayed || retried.Event.Cursor != created.Event.Cursor {
			t.Fatalf("create retry=%+v err=%v", retried, err)
		}
		outsider := app.Actor{ID: "agent:coord-outsider", Capabilities: map[string]struct{}{"coordination:read": {}, "coordination:respond": {}, "event:read": {}, "overview:read": {}}}
		if _, err := service.Get(ctx, outsider, project, domain.ResourceCoordinationRequest, input.ID); !errors.Is(err, app.ErrNotFound) {
			t.Fatalf("private get=%v", err)
		}
		page, err := service.List(ctx, outsider, project, domain.ResourceCoordinationRequest, "", 1)
		if err != nil || len(page.Items) != 0 {
			t.Fatalf("private page=%+v err=%v", page, err)
		}
		response := app.CoordinationActionInput{Body: "Evidence reviewed", EvidenceIDs: []string{"coord-evidence"}}
		if _, err := service.ActCoordination(ctx, outsider, project, input.ID, "respond", "coord-forbidden", 1, response, ""); !errors.Is(err, app.ErrDenied) {
			t.Fatalf("wrong responder=%v", err)
		}
		responded, err := service.ActCoordination(ctx, recipient, project, input.ID, "respond", "coord-response", 1, response, "")
		if err != nil {
			t.Fatal(err)
		}
		if retry, err := service.ActCoordination(ctx, recipient, project, input.ID, "respond", "coord-response", 1, response, ""); err != nil || !retry.Replayed || retry.Event.Cursor != responded.Event.Cursor {
			t.Fatalf("response retry=%+v err=%v", retry, err)
		}
		if _, err := service.ActCoordination(ctx, outsider, project, input.ID, "respond", "coord-response", 1, response, ""); !errors.Is(err, app.ErrIdempotency) {
			t.Fatalf("foreign idempotency=%v", err)
		}
		if _, err := service.ActCoordination(ctx, worker, project, input.ID, "resolve", "coord-stale-close", 1, app.CoordinationActionInput{}, ""); !errors.Is(err, app.ErrVersionConflict) {
			t.Fatalf("version check=%v", err)
		}
		closed, err := service.ActCoordination(ctx, worker, project, input.ID, "resolve", "coord-close", 2, app.CoordinationActionInput{Note: "Accepted context only"}, "")
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 30; i++ {
			next := input
			next.ID = fmt.Sprintf("coord-open-%02d", i)
			if _, err := service.CreateCoordination(ctx, worker, project, next.ID, next, ""); err != nil {
				t.Fatal(err)
			}
		}
		// Narrow recipient filtering occurs before pagination, including tied timestamps.
		seen := map[string]bool{}
		cursor := ""
		for {
			page, err := service.List(ctx, recipient, project, domain.ResourceCoordinationRequest, cursor, 7)
			if err != nil {
				t.Fatal(err)
			}
			for _, raw := range page.Items {
				var req domain.CoordinationRequest
				if err := json.Unmarshal(raw, &req); err != nil {
					t.Fatal(err)
				}
				if seen[req.ID] || req.Recipient.ID != recipient.ID {
					t.Fatalf("duplicate or leaked item=%+v", req)
				}
				seen[req.ID] = true
			}
			if page.NextCursor == "" {
				break
			}
			cursor = page.NextCursor
		}
		if len(seen) != 31 {
			t.Fatalf("request pagination=%d", len(seen))
		}
		check := func() {
			t.Helper()
			overview, err := service.Overview(ctx, app.Actor{ID: "operator", Capabilities: map[string]struct{}{"resource:read": {}}}, project)
			if err != nil {
				t.Fatal(err)
			}
			summary := overview["coordination"].(app.CoordinationSummary)
			if summary.Counts.RunningWork != 1 || summary.Counts.AgentPresence != 2 || summary.Counts.Requests != 30 || !summary.Truncated || len(summary.Items) != 24 {
				t.Fatalf("coordination summary=%+v", summary)
			}
			for _, item := range summary.Items {
				if item.Kind == "request" && (item.OwnerName == "" || item.RecipientName == "" || item.Detail != "Actual request context") {
					t.Fatalf("missing attribution=%+v", item)
				}
			}
			raw, err := repo.Get(ctx, project, domain.ResourceCoordinationRequest, input.ID)
			if err != nil || string(raw) != string(closed.Resource) {
				var req domain.CoordinationRequest
				_ = json.Unmarshal(raw, &req)
				if err != nil || req.Status != "resolved" || req.Version != 3 || req.Response.ActorID != recipient.ID {
					t.Fatalf("rebuild=%+v err=%v", req, err)
				}
			}
			raw, err = repo.Get(ctx, project, domain.ResourceAgent, worker.ID)
			if err != nil {
				t.Fatal(err)
			}
			var agent domain.Agent
			_ = json.Unmarshal(raw, &agent)
			if agent.DisplayName == "Stale overwrite" || agent.PresenceExpiresAt == nil {
				t.Fatalf("stale changed lease=%+v", agent)
			}
		}
		check()
		if err := repo.RebuildProjections(ctx); err != nil {
			t.Fatal(err)
		}
		check()
		overview, err := service.Overview(ctx, outsider, project)
		if err != nil {
			t.Fatal(err)
		}
		summary := overview["coordination"].(app.CoordinationSummary)
		if summary.Complete || len(summary.Items) != 0 || summary.Counts.Requests != 0 {
			t.Fatalf("scoped summary leaked=%+v", summary)
		}
		if _, exists := overview["counts"].(map[string]int64)["coordination_request"]; exists {
			t.Fatal("raw private request count leaked")
		}
		if err := service.Ingest(ctx, app.Actor{ID: "sentry", Capabilities: map[string]struct{}{"event:ingest": {}, "source:sentry": {}}}, []domain.Event{{ProjectID: project, SubjectID: input.ID, SubjectType: domain.ResourceCoordinationRequest, Type: "coordination_request.created", SourceSystem: "sentry", SourceEventID: "forged", SourceSequence: 1, Payload: json.RawMessage(`{}`)}}); err == nil {
			t.Fatal("publisher coordination forgery")
		}
	})

	t.Run("narrow runtime source cannot mutate human or foreign participant records", func(t *testing.T) {
		const project = "runtime-scope-project"
		if _, err := repo.EnsureProject(ctx, project, "Runtime scope integration"); err != nil {
			t.Fatal(err)
		}
		setup := app.Actor{ID: "human-operator", Human: true, Capabilities: map[string]struct{}{"resource:write": {}}}
		targets := []struct {
			kind      domain.ResourceType
			id, event string
			value     any
		}{
			{domain.ResourceTask, "human-task", "task.completed", domain.Task{Base: domain.Base{ID: "human-task"}, Title: "Human-owned task", Description: "Keep human fields", OwnerID: "human-operator", Status: "working", Priority: "high", Dependencies: []string{}}},
			{domain.ResourceRun, "other-run", "run.cancelled", domain.Run{Base: domain.Base{ID: "other-run"}, AgentID: "agent:someone-else", Status: "running", ControlStatus: "executed", ControlAction: "pause", Capabilities: []string{}}},
			{domain.ResourceSession, "other-session", "session.closed", domain.Session{Base: domain.Base{ID: "other-session"}, AgentID: "agent:someone-else", Status: "active"}},
			{domain.ResourceApproval, "human-approval", "approval.resolved", domain.Approval{Base: domain.Base{ID: "human-approval"}, Status: "pending", RequestedBy: "human-operator", Reason: "Explicit human decision", ExpiresAt: time.Now().Add(time.Hour)}},
		}
		before := map[string]string{}
		for _, target := range targets {
			created, err := service.Create(ctx, setup, target.kind, project, "runtime-scope-create-"+target.id, target.value, "")
			if err != nil {
				t.Fatal(err)
			}
			var document any
			_ = json.Unmarshal(created.Resource, &document)
			canonical, _ := json.Marshal(document)
			before[target.id] = string(canonical)
		}
		var priorCursor int64
		if err := repo.Pool().QueryRow(ctx, `SELECT max(cursor) FROM events WHERE project_id=$1`, project).Scan(&priorCursor); err != nil {
			t.Fatal(err)
		}
		for _, system := range []string{"codex", "claude", "hermes"} {
			worker := app.Actor{ID: "agent:narrow", Capabilities: map[string]struct{}{"event:ingest": {}, "source:" + system: {}}}
			for _, target := range targets {
				event := domain.Event{ProjectID: project, SubjectType: target.kind, SubjectID: target.id, Type: target.event, SourceSystem: system, SourceEventID: system + "-scope-" + target.id, SourceSequence: 100, Payload: json.RawMessage(`{}`)}
				if err := service.Ingest(ctx, worker, []domain.Event{event}); !errors.Is(err, app.ErrDenied) {
					t.Fatalf("%s mutated %s: %v", system, target.id, err)
				}
			}
		}
		for _, target := range targets {
			raw, err := repo.Get(ctx, project, target.kind, target.id)
			if err != nil {
				t.Fatal(err)
			}
			var document any
			_ = json.Unmarshal(raw, &document)
			canonical, _ := json.Marshal(document)
			if string(canonical) != before[target.id] {
				t.Fatalf("denied runtime changed %s", target.id)
			}
		}
		var afterCursor int64
		if err := repo.Pool().QueryRow(ctx, `SELECT max(cursor) FROM events WHERE project_id=$1`, project).Scan(&afterCursor); err != nil || afterCursor != priorCursor {
			t.Fatalf("denied runtime created events: prior=%d after=%d err=%v", priorCursor, afterCursor, err)
		}
		if err := repo.RebuildProjections(ctx); err != nil {
			t.Fatal(err)
		}
		for _, target := range targets {
			raw, err := repo.Get(ctx, project, target.kind, target.id)
			if err != nil {
				t.Fatal(err)
			}
			var document any
			_ = json.Unmarshal(raw, &document)
			canonical, _ := json.Marshal(document)
			if string(canonical) != before[target.id] {
				t.Fatalf("rebuild changed protected %s", target.id)
			}
		}
	})

	t.Run("GitHub and Sentry keep independent task identities reports freshness and replay", func(t *testing.T) {
		const project = "github-shared-project"
		const external = "repo:42:pull:7"
		if _, err := repo.EnsureProject(ctx, project, "Canonical shared project"); err != nil {
			t.Fatal(err)
		}
		at := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
		providers := map[string]app.Actor{"sentry": {ID: "sentry-readonly", Capabilities: map[string]struct{}{"event:ingest": {}, "source:sentry": {}}}, "github": {ID: "github-readonly", Capabilities: map[string]struct{}{"event:ingest": {}, "source:github": {}}}}
		publishProject := func(system string, sequence int64, id string, observation domain.ProjectObservation) error {
			raw, _ := json.Marshal(observation)
			return service.Ingest(ctx, providers[system], []domain.Event{{ProjectID: project, SubjectType: domain.ResourceProject, SubjectID: "forged", Type: "project.observed", SourceSystem: system, SourceEventID: id, SourceSequence: sequence, Payload: raw}})
		}
		publishTask := func(system string, sequence int64, id, status, title string) error {
			raw, _ := json.Marshal(domain.TaskObservation{ExternalID: external, Title: title, Description: "Recorded source detail", Status: status, ExternalURL: "https://github.com/owner/repo/pull/7"})
			return service.Ingest(ctx, providers[system], []domain.Event{{ProjectID: project, SubjectType: domain.ResourceTask, SubjectID: domain.ObservedTaskID(project, external), Type: "task.observed", SourceSystem: system, SourceEventID: id, SourceSequence: sequence, OccurredAt: at, Payload: raw}})
		}
		githubReport := domain.ProjectObservation{ExternalID: "repo:42", Name: "owner/repo", Path: "https://github.com/owner/repo", Status: "active", CollectedAt: at, ScanStatus: "complete", TaskCount: 1}
		sentryReport := domain.ProjectObservation{ExternalID: "host:/source/project", Name: "Sentry name", Path: "/source/project", Status: "Active", CollectedAt: at.Add(-time.Hour), ScanStatus: "complete", TaskCount: 1}
		if err := publishProject("sentry", 5, "project-scan", sentryReport); err != nil {
			t.Fatal(err)
		}
		if err := publishProject("github", 5, "project-scan", githubReport); err != nil {
			t.Fatal(err)
		}
		if err := publishTask("sentry", 5, "task-scan", "Blocked", "Sentry task"); err != nil {
			t.Fatal(err)
		}
		if err := publishTask("github", 5, "task-scan", "blocked", "GitHub PR"); err != nil {
			t.Fatal(err)
		}
		githubID, sentryID := domain.ObservedTaskIDForSource(project, "github", external), domain.ObservedTaskID(project, external)
		if githubID == sentryID {
			t.Fatal("source task identities collided")
		}
		if _, err := service.TransitionTask(ctx, actor, project, githubID, "ready", "", "github-human-ready", 1, ""); err != nil {
			t.Fatal(err)
		}
		human := app.Actor{ID: "human-owner", Human: true, Capabilities: map[string]struct{}{"task:claim": {}}}
		if _, err := service.ClaimTask(ctx, human, project, githubID, "github-human-owner", 2, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := service.Create(ctx, actor, domain.ResourceAttention, project, "github-human-attention", domain.Attention{Base: domain.Base{ID: "github-human-note"}, Kind: "manual.review", Title: "Human-owned attention", Status: "open", ResourceType: "task", ResourceID: githubID}, ""); err != nil {
			t.Fatal(err)
		}
		if err := publishTask("github", 6, "task-merged", "merged", "Renamed latest PR title"); err != nil {
			t.Fatal(err)
		}
		if err := publishTask("github", 4, "task-stale", "blocked", "Stale PR title"); err != nil {
			t.Fatal(err)
		}
		partial := githubReport
		partial.CollectedAt = at.Add(10 * time.Minute)
		partial.ScanStatus = "partial"
		partial.ScanError = "One check result unavailable; prior records retained"
		if err := publishProject("github", 6, "project-partial", partial); err != nil {
			t.Fatal(err)
		}
		stale := githubReport
		stale.CollectedAt = at.Add(20 * time.Minute)
		if err := publishProject("github", 4, "project-stale", stale); err != nil {
			t.Fatal(err)
		}
		// A different project-state row carrying the same source marker must not
		// duplicate or replace that source's canonical observation report.
		noise := domain.Project{Base: domain.NewBase("unrelated-github-project-record", project, at.Add(30*time.Minute)), Source: domain.Source{System: "github", ExternalID: "wrong"}, SourceObservation: &domain.ProjectObservation{ExternalID: "wrong", CollectedAt: at.Add(30 * time.Minute), ScanStatus: "complete"}}
		noiseRaw, _ := json.Marshal(noise)
		if _, err := repo.Pool().Exec(ctx, `INSERT INTO resources(project_id,kind,id,version,document,created_at,updated_at,source_system,source_sequence) VALUES($1,'project',$2,1,$3,$4,$4,'github',999)`, project, noise.ID, noiseRaw, noise.CreatedAt); err != nil {
			t.Fatal(err)
		}
		check := func() {
			t.Helper()
			snapshot, err := repo.Workspace(ctx, project)
			if err != nil {
				t.Fatal(err)
			}
			if len(snapshot.Resources.Tasks) != 2 || len(snapshot.Sources) != 2 {
				t.Fatalf("source resources duplicated or omitted=%+v", snapshot)
			}
			sources := map[string]app.WorkspaceSource{}
			for _, source := range snapshot.Sources {
				if _, duplicate := sources[source.System]; duplicate {
					t.Fatal("duplicate freshness source")
				}
				sources[source.System] = source
			}
			gh, sen := sources["github"], sources["sentry"]
			if gh.ProjectObservation == nil || gh.ProjectObservation.ExternalID != "repo:42" || gh.LastCollectedAt == nil || !gh.LastCollectedAt.Equal(partial.CollectedAt) || gh.LastSuccessfulCollectionAt == nil || !gh.LastSuccessfulCollectionAt.Equal(githubReport.CollectedAt) || gh.ScanStatus == nil || *gh.ScanStatus != "partial" {
				t.Fatalf("GitHub freshness/report overwritten=%+v", gh)
			}
			if sen.ProjectObservation == nil || sen.ProjectObservation.ExternalID != sentryReport.ExternalID || sen.LastCollectedAt == nil || !sen.LastCollectedAt.Equal(sentryReport.CollectedAt) || sen.ScanStatus == nil || *sen.ScanStatus != "complete" {
				t.Fatalf("Sentry report overwritten=%+v", sen)
			}
			for system, want := range map[string]string{"sentry": project, "github": domain.ObservedProjectIDForSource(project, "github")} {
				raw, err := repo.Get(ctx, project, domain.ResourceProject, want)
				if err != nil {
					t.Fatal(err)
				}
				var report domain.Project
				_ = json.Unmarshal(raw, &report)
				if report.Source.System != system || report.Name != "Canonical shared project" {
					t.Fatalf("report identity=%+v", report)
				}
			}
			for _, raw := range snapshot.Resources.Tasks {
				var task domain.Task
				_ = json.Unmarshal(raw, &task)
				switch task.Source.System {
				case "github":
					if task.ID != githubID || task.Status != "ready" || task.OwnerID != human.ID || task.Title != "GitHub PR" || task.SourceStatus != "merged" || task.SourceObservation.Title != "Renamed latest PR title" {
						t.Fatalf("GitHub source overwrote control state or accepted stale=%+v", task)
					}
				case "sentry":
					if task.ID != sentryID || task.Status != "inbox" || task.SourceStatus != "Blocked" || task.Title != "Sentry task" {
						t.Fatalf("Sentry state changed=%+v", task)
					}
				default:
					t.Fatalf("unexpected source=%+v", task)
				}
			}
			attention := map[string]domain.Attention{}
			for _, raw := range snapshot.Resources.Attention {
				var item domain.Attention
				_ = json.Unmarshal(raw, &item)
				attention[item.ResourceID+":"+item.Kind] = item
			}
			if attention[githubID+":source.task.blocked"].Status != "resolved" || attention[sentryID+":source.task.blocked"].Status != "open" || attention[githubID+":manual.review"].Status != "open" {
				t.Fatalf("source attention crossed authority=%+v", attention)
			}
			overview, err := repo.Overview(ctx, project)
			if err != nil {
				t.Fatal(err)
			}
			if len(overview["sources"].([]app.WorkspaceSource)) != 2 {
				t.Fatal("overview omitted source")
			}
			summary := overview["coordination"].(app.CoordinationSummary)
			if summary.Counts.SourceBlockedTasks != 1 || summary.Counts.SourceUnfinishedTasks != 1 || summary.Counts.HumanEscalations != 0 || summary.Counts.RunningWork != 0 {
				t.Fatalf("source history became active worker/human escalation=%+v", summary)
			}
			var name string
			if err := repo.Pool().QueryRow(ctx, `SELECT name FROM projects WHERE id=$1`, project).Scan(&name); err != nil || name != "Canonical shared project" {
				t.Fatalf("source renamed canonical project=%s err=%v", name, err)
			}
		}
		check()
		before, err := repo.Workspace(ctx, project)
		if err != nil {
			t.Fatal(err)
		}
		if err := publishTask("github", 5, "task-scan", "blocked", "GitHub PR"); err != nil {
			t.Fatal(err)
		}
		after, err := repo.Workspace(ctx, project)
		if err != nil || after.Cursor != before.Cursor {
			t.Fatalf("immutable retry advanced cursor before=%d after=%d err=%v", before.Cursor, after.Cursor, err)
		}
		if err := repo.RebuildProjections(ctx); err != nil {
			t.Fatal(err)
		}
		check()
		// The new source is not permitted to send runtime, approval or command events.
		for _, target := range []struct {
			kind  domain.ResourceType
			event string
		}{{domain.ResourceTask, "task.completed"}, {domain.ResourceApproval, "approval.approved"}, {domain.ResourceRun, "run.started"}, {domain.ResourceCoordinationRequest, "coordination_request.created"}} {
			err := service.Ingest(ctx, providers["github"], []domain.Event{{ProjectID: project, SubjectType: target.kind, SubjectID: githubID, Type: target.event, SourceSystem: "github", SourceEventID: "forged-" + target.event, SourceSequence: 100, Payload: json.RawMessage(`{}`)}})
			if err == nil {
				t.Fatalf("GitHub command forgery accepted=%s", target.event)
			}
		}
		wrong := githubReport
		wrong.ExternalID = "repo:another"
		if err := publishProject("github", 7, "project-takeover", wrong); err == nil {
			t.Fatal("different native repository claimed GitHub report")
		}
		check()
		// A predictable source-report namespace is not permission to adopt a
		// human-authored resource already occupying that identity.
		const collisionProject = "github-human-report-collision"
		if _, err := repo.EnsureProject(ctx, collisionProject, "Human project"); err != nil {
			t.Fatal(err)
		}
		reportID := domain.ObservedProjectIDForSource(collisionProject, "github")
		if _, err := service.Create(ctx, actor, domain.ResourceProject, collisionProject, "human-report-create", domain.Project{Base: domain.Base{ID: reportID}, Name: "Human context", Slug: "human-context", Status: "supervised"}, ""); err != nil {
			t.Fatal(err)
		}
		beforeCollision, err := repo.Get(ctx, collisionProject, domain.ResourceProject, reportID)
		if err != nil {
			t.Fatal(err)
		}
		payload, _ := json.Marshal(githubReport)
		if err := service.Ingest(ctx, providers["github"], []domain.Event{{ProjectID: collisionProject, SubjectType: domain.ResourceProject, SubjectID: "untrusted", Type: "project.observed", SourceSystem: "github", SourceEventID: "human-report-collision", SourceSequence: 1, Payload: payload}}); err == nil {
			t.Fatal("GitHub adopted human project-state resource")
		}
		afterCollision, err := repo.Get(ctx, collisionProject, domain.ResourceProject, reportID)
		if err != nil || string(afterCollision) != string(beforeCollision) {
			t.Fatalf("collision overwrote human resource=%s err=%v", afterCollision, err)
		}
		collisionSnapshot, err := repo.Workspace(ctx, collisionProject)
		if err != nil || len(collisionSnapshot.Sources) != 0 {
			t.Fatalf("denied collision created source report=%+v err=%v", collisionSnapshot, err)
		}
		other, err := repo.Workspace(ctx, "project-1")
		if err != nil {
			t.Fatal(err)
		}
		for _, source := range other.Sources {
			if source.System == "github" {
				t.Fatal("GitHub report leaked across project")
			}
		}
	})

}

type integrationController struct {
	calls atomic.Int64
}

func (*integrationController) Supports(string, string) bool { return true }
func (c *integrationController) Execute(context.Context, string, string, string) error {
	c.calls.Add(1)
	return nil
}
