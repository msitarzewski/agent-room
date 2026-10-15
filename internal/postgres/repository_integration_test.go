//go:build integration

package postgres

import (
	"context"
	"encoding/json"
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

}

type integrationController struct {
	calls atomic.Int64
}

func (*integrationController) Supports(string, string) bool { return true }
func (c *integrationController) Execute(context.Context, string, string, string) error {
	c.calls.Add(1)
	return nil
}
