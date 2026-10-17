package postgres

import (
	"encoding/json"
	"fmt"
	"github.com/msitarzewski/agent-room/internal/app"
	"strings"
	"testing"
	"time"

	"github.com/msitarzewski/agent-room/internal/domain"
)

func TestTaskObservationPreservesSupervisedFields(t *testing.T) {
	at := time.Date(2026, 10, 13, 20, 0, 0, 0, time.UTC)
	event := domain.Event{ProjectID: "project", SubjectID: domain.ObservedTaskID("project", "task-1"),
		SubjectType: domain.ResourceTask, Type: "task.observed", SourceSystem: "sentry", OccurredAt: at,
		Payload: json.RawMessage(`{"external_id":"task-1","title":"Source title","status":"completed","dependencies":["task-2"]}`)}
	raw, err := reduceResource(nil, 0, event)
	if err != nil {
		t.Fatal(err)
	}
	var task domain.Task
	if err := json.Unmarshal(raw, &task); err != nil {
		t.Fatal(err)
	}
	if task.Status != "inbox" || task.SourceStatus != "completed" || task.Title != "Source title" || task.Version != 1 || len(task.Dependencies) != 1 || task.Dependencies[0] != domain.ObservedTaskID("project", "task-2") {
		t.Fatalf("initial task=%+v", task)
	}
	task.Title, task.Description, task.Status, task.Priority = "Operator title", "Operator description", "review", "critical"
	task.OwnerID, task.BudgetID, task.ReviewState, task.ApprovalState = "human", "budget", "accepted", "pending"
	task.BlockedReason, task.Dependencies = "Operator note", []string{"operator-dependency"}
	task.Version = 7
	current, _ := json.Marshal(task)
	event.Payload = json.RawMessage(`{"external_id":"task-1","title":"Changed source title","status":"blocked","description":"New source description"}`)
	event.OccurredAt = at.Add(time.Minute)
	raw, err = reduceResource(current, 7, event)
	if err != nil {
		t.Fatal(err)
	}
	var next domain.Task
	if err := json.Unmarshal(raw, &next); err != nil {
		t.Fatal(err)
	}
	if next.Status != task.Status || next.Title != task.Title || next.Description != task.Description || next.Priority != task.Priority || next.OwnerID != task.OwnerID || next.BudgetID != task.BudgetID || next.ReviewState != task.ReviewState || next.ApprovalState != task.ApprovalState || next.BlockedReason != task.BlockedReason || next.Dependencies[0] != task.Dependencies[0] {
		t.Fatalf("observation overwrote supervisory fields: %+v", next)
	}
	if next.Version != 8 || next.SourceStatus != "blocked" || next.SourceObservation.Title != "Changed source title" || !next.SourceObservedAt.Equal(event.OccurredAt) {
		t.Fatalf("observation not updated: %+v", next)
	}
	task.Source.System = "another-source"
	current, _ = json.Marshal(task)
	if _, err := reduceResource(current, 7, event); err == nil {
		t.Fatal("observation claimed another source's task")
	}
}

func TestExternalTaskCompletionDoesNotAcceptSupervisedCompletion(t *testing.T) {
	current := json.RawMessage(`{"id":"task","project_id":"project","version":3,"status":"review","title":"Review me"}`)
	event := domain.Event{ProjectID: "project", SubjectID: "task", SubjectType: domain.ResourceTask, Type: "task.completed", SourceSystem: "codex", OccurredAt: time.Now(), Payload: json.RawMessage(`{}`)}
	raw, err := reduceResource(current, 3, event)
	if err != nil {
		t.Fatal(err)
	}
	var task domain.Task
	if err := json.Unmarshal(raw, &task); err != nil {
		t.Fatal(err)
	}
	if task.Status != "review" || task.SourceStatus != "completed" {
		t.Fatalf("external observation accepted completion: %+v", task)
	}
}

func TestExternalPayloadCannotReplaceProjectedResource(t *testing.T) {
	event := domain.Event{ProjectID: "project", SubjectID: "run", SubjectType: domain.ResourceRun, Type: "run.started", SourceSystem: "codex", OccurredAt: time.Now(), Payload: json.RawMessage(`{"resource":{"id":"other","project_id":"other","status":"cancelled","version":99}}`)}
	raw, err := reduceResource(nil, 0, event)
	if err != nil {
		t.Fatal(err)
	}
	var run domain.Run
	if err := json.Unmarshal(raw, &run); err != nil {
		t.Fatal(err)
	}
	if run.ID != "run" || run.ProjectID != "project" || run.Version != 1 || run.Status != "running" {
		t.Fatalf("untrusted resource replaced projection: %+v", run)
	}
}

func TestSourceTaskAttentionLifecyclePreservesHumanItems(t *testing.T) {
	at := time.Date(2026, 10, 14, 5, 0, 0, 0, time.UTC)
	observation := domain.TaskObservation{ExternalID: "external", Title: "Actual source task", Status: " Blocked ", Description: "Source detail"}
	first, changed := reduceSourceTaskAttention(nil, "project", "task", observation, "", at)
	if !changed || first.Kind != "source.task.blocked" || first.Title != "Source reports blocked: Actual source task" || first.ResourceID != "task" || first.Status != "open" || first.Version != 1 {
		t.Fatalf("first=%+v changed=%v", first, changed)
	}
	first.Status, first.OwnerID = "acknowledged", "human"
	observation.Status = "bLoCkEd"
	repeated, changed := reduceSourceTaskAttention(&first, "project", "task", observation, "Blocked", at.Add(time.Minute))
	if changed || repeated.Version != 1 || repeated.Status != "acknowledged" || repeated.ID != first.ID {
		t.Fatalf("repeat=%+v changed=%v", repeated, changed)
	}
	observation.Status = "Upcoming"
	resolved, changed := reduceSourceTaskAttention(&repeated, "project", "task", observation, "Blocked", at.Add(2*time.Minute))
	if !changed || resolved.Status != "resolved" || resolved.ResolvedAt == nil || resolved.Version != 2 || resolved.OwnerID != "human" {
		t.Fatalf("resolved=%+v changed=%v", resolved, changed)
	}
	if _, changed := reduceSourceTaskAttention(&resolved, "project", "task", observation, "Upcoming", at.Add(3*time.Minute)); changed {
		t.Fatal("repeat nonblocked observation changed resolved attention")
	}
	if _, changed := reduceSourceTaskAttention(nil, "project", "task", observation, "Upcoming", at); changed {
		t.Fatal("nonblocked source created attention")
	}
	human := first
	human.Kind = "task.blocked"
	preserved, changed := reduceSourceTaskAttention(&human, "project", "task", observation, "Blocked", at)
	if changed || preserved.Kind != "task.blocked" || preserved.Status != human.Status {
		t.Fatal("source resolved human attention")
	}
	observation.Status = "Blocked"
	stillResolved, changed := reduceSourceTaskAttention(&resolved, "project", "task", observation, "Blocked", at.Add(4*time.Minute))
	if changed || stillResolved.Status != "resolved" || stillResolved.Version != resolved.Version {
		t.Fatal("still-blocked source undid human resolution")
	}
	observation.Title = "Newer title in the same blocked episode"
	stillResolved, changed = reduceSourceTaskAttention(&resolved, "project", "task", observation, "Blocked", at.Add(5*time.Minute))
	if changed || stillResolved.Status != "resolved" {
		t.Fatal("newer still-blocked source undid human resolution")
	}
	observation.Title = "Actual source task"
	reopened, changed := reduceSourceTaskAttention(&resolved, "project", "task", observation, "Upcoming", at.Add(4*time.Minute))
	if !changed || reopened.Status != "open" || reopened.ResolvedAt != nil || reopened.Version != 3 || reopened.ID != first.ID {
		t.Fatalf("reopened=%+v changed=%v", reopened, changed)
	}
}

func TestTrustedCommandReplayPreservesCompleteResource(t *testing.T) {
	event := domain.Event{ProjectID: "project", SubjectType: domain.ResourceTask, SubjectID: "task", Type: "task.ready", CommandID: "server-command", Payload: json.RawMessage(`{"resource":{"id":"task","project_id":"project","version":7,"title":"Operator title","status":"ready","owner_id":"human","source":{"system":"sentry","external_id":"external"},"source_status":"Upcoming"}}`)}
	raw, err := reduceResource(nil, 0, event)
	if err != nil {
		t.Fatal(err)
	}
	var task domain.Task
	if err := json.Unmarshal(raw, &task); err != nil {
		t.Fatal(err)
	}
	if task.Version != 7 || task.OwnerID != "human" || task.Status != "ready" || task.SourceStatus != "Upcoming" {
		t.Fatalf("command replay lost state: %+v", task)
	}
	event.SourceSystem = "sentry"
	if len(commandResource(event)) != 0 {
		t.Fatal("source event trusted as command replay")
	}
	event.SourceSystem = ""
	event.SubjectID = "other-task"
	if _, err := reduceResource(nil, 0, event); err == nil {
		t.Fatal("mismatched replay identity accepted")
	}
}

func TestProjectObservationKeepsCanonicalFieldsAndSuccessfulScan(t *testing.T) {
	at := time.Date(2026, 10, 14, 13, 0, 0, 0, time.UTC)
	observation := domain.ProjectObservation{ExternalID: "host:/source/project", Name: "Source display", Path: "/source/project", Status: "Active", CollectedAt: at, ScanStatus: "complete", TaskCount: 0}
	payload, _ := json.Marshal(observation)
	event := domain.Event{ProjectID: "project", SubjectType: domain.ResourceProject, SubjectID: "project", Type: "project.observed", SourceSystem: "sentry", SourceSequence: 1, OccurredAt: at, Payload: payload}
	raw, err := reduceProjectObservation(nil, 0, event, "Canonical name")
	if err != nil {
		t.Fatal(err)
	}
	var project domain.Project
	if err := json.Unmarshal(raw, &project); err != nil {
		t.Fatal(err)
	}
	if project.Name != "Canonical name" || project.SourceStatus != "Active" || project.SourceObservation.TaskCount != 0 || project.SourceLastSuccessfulCollectionAt == nil || !project.SourceLastSuccessfulCollectionAt.Equal(at) {
		t.Fatalf("project=%+v", project)
	}
	project.OrganizationID, project.Slug, project.Status = "operator-org", "operator-slug", "operator-status"
	prior, _ := json.Marshal(project)
	observation.Name, observation.Status, observation.ScanStatus = "Changed source name", "Quiet", "partial"
	observation.CollectedAt = at.Add(time.Hour)
	observation.ScanError = "One path unreadable"
	event.OccurredAt = observation.CollectedAt
	event.Payload, _ = json.Marshal(observation)
	raw, err = reduceProjectObservation(prior, 1, event, "Ignored canonical argument")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &project); err != nil {
		t.Fatal(err)
	}
	if project.Name != "Canonical name" || project.OrganizationID != "operator-org" || project.Slug != "operator-slug" || project.Status != "operator-status" || project.SourceStatus != "Quiet" || project.Version != 2 || !project.SourceLastSuccessfulCollectionAt.Equal(at) {
		t.Fatalf("source changed canonical fields or successful scan: %+v", project)
	}
	project.Source.System = "other-source"
	prior, _ = json.Marshal(project)
	if _, err := reduceProjectObservation(prior, 2, event, "Canonical"); err == nil {
		t.Fatal("source claimed another project identity")
	}
}

func TestCoordinationSummaryUsesActionableFactsAndScopedRequests(t *testing.T) {
	at := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	future := at.Add(time.Minute)
	past := at.Add(-time.Minute)
	documents := map[domain.ResourceType][]json.RawMessage{}
	put := func(kind domain.ResourceType, value any) {
		raw, _ := json.Marshal(value)
		documents[kind] = append(documents[kind], raw)
	}
	put(domain.ResourceRun, domain.Run{Base: domain.NewBase("run", "p", at), Status: "running", AgentID: "worker", Summary: "Real explicit running work"})
	put(domain.ResourceRun, domain.Run{Base: domain.NewBase("old-run", "p", at), Status: "completed"})
	put(domain.ResourceAgent, domain.Agent{Base: domain.NewBase("worker", "p", at), DisplayName: "Worker", Status: "idle", PresenceExpiresAt: &future})
	put(domain.ResourceAgent, domain.Agent{Base: domain.NewBase("stale", "p", at), Status: "active", PresenceExpiresAt: &past})
	put(domain.ResourceAgent, domain.Agent{Base: domain.NewBase("legacy", "p", at), Status: "active"})
	put(domain.ResourceTask, domain.Task{Base: domain.NewBase("blocked", "p", at), Title: "Blocked owned work", Status: "blocked", OwnerID: "worker", BlockedReason: "Actual canonical reason", Source: domain.Source{System: "sentry"}, SourceStatus: " Blocked "})
	put(domain.ResourceTask, domain.Task{Base: domain.NewBase("source-only", "p", at), Title: "Source blocked", Status: "inbox", Source: domain.Source{System: "sentry"}, SourceStatus: "Blocked", SourceObservation: &domain.TaskObservation{Description: "Recorded source detail"}})
	put(domain.ResourceTask, domain.Task{Base: domain.NewBase("done-source", "p", at), Status: "inbox", Source: domain.Source{System: "sentry"}, SourceStatus: "Done"})
	put(domain.ResourceTask, domain.Task{Base: domain.NewBase("unknown-source", "p", at), Status: "inbox", Source: domain.Source{System: "sentry"}, SourceStatus: "Unrecognized"})
	put(domain.ResourceTask, domain.Task{Base: domain.NewBase("review-task", "p", at), Title: "Review", Status: "review", OwnerID: "worker"})
	put(domain.ResourceCoordinationRequest, domain.CoordinationRequest{Base: domain.NewBase("request", "p", at), Kind: "help", Title: "Need context", Body: "Actual ask", Status: "open", OwnerID: "worker", OwnerType: "agent", Recipient: domain.CoordinationRecipient{Type: "human", ID: "operator"}})
	put(domain.ResourceCoordinationRequest, domain.CoordinationRequest{Base: domain.NewBase("done-request", "p", at), Kind: "review", Status: "resolved"})
	put(domain.ResourceClaim, domain.Claim{Base: domain.NewBase("claim", "p", at), Status: "pending", AgentID: "worker"})
	put(domain.ResourceApproval, domain.Approval{Base: domain.NewBase("approval", "p", at), Status: "pending", ExpiresAt: future, Reason: "Explicit approval"})
	put(domain.ResourceApproval, domain.Approval{Base: domain.NewBase("expired", "p", at), Status: "pending", ExpiresAt: past})
	put(domain.ResourceApproval, domain.Approval{Base: domain.NewBase("approved", "p", at), Status: "approved", ExpiresAt: future})
	all := app.Actor{Capabilities: map[string]struct{}{"resource:read": {}}}
	summary, err := summarizeCoordination(documents, all, at)
	if err != nil {
		t.Fatal(err)
	}
	want := app.CoordinationCounts{RunningWork: 1, AgentPresence: 1, UnfinishedTasks: 2, SourceUnfinishedTasks: 2, BlockedTasks: 2, CanonicalBlockedTasks: 1, SourceBlockedTasks: 2, Requests: 1, Reviews: 2, HumanEscalations: 1, HumanApprovals: 1}
	if summary.Counts != want || !summary.Complete || summary.Truncated {
		t.Fatalf("summary=%+v", summary)
	}
	for _, item := range summary.Items {
		if item.ID == "blocked" && item.Detail != "Actual canonical reason" {
			t.Fatalf("blocker reason=%+v", item)
		}
		if item.ID == "source-only" && (item.Status != "source_blocked" || item.Detail != "Recorded source detail") {
			t.Fatalf("source blocker=%+v", item)
		}
	}
	narrow := app.Actor{ID: "unrelated", Capabilities: map[string]struct{}{"coordination:read": {}}}
	scoped, err := summarizeCoordination(documents, narrow, at)
	if err != nil || scoped.Complete || len(scoped.Items) != 0 || scoped.Counts.Requests != 0 {
		t.Fatalf("scoped summary=%+v err=%v", scoped, err)
	}
	narrow.ID = "worker"
	scoped, err = summarizeCoordination(documents, narrow, at)
	if err != nil || scoped.Counts.Requests != 1 || len(scoped.Items) != 1 {
		t.Fatalf("owned summary=%+v err=%v", scoped, err)
	}
	for i := 0; i < 30; i++ {
		put(domain.ResourceCoordinationRequest, domain.CoordinationRequest{Base: domain.NewBase(fmt.Sprintf("extra-%02d", i), "p", at), Kind: "context", Title: strings.Repeat("x", 400), Body: strings.Repeat("y", 3000), Status: "open", OwnerID: "worker", OwnerType: "agent", Recipient: domain.CoordinationRecipient{Type: "agent", ID: "other"}})
	}
	bounded, err := summarizeCoordination(documents, all, at)
	if err != nil || !bounded.Truncated || len(bounded.Items) != 24 || bounded.Counts.Requests != 31 {
		t.Fatalf("bounded summary=%+v err=%v", bounded, err)
	}
	for _, item := range bounded.Items {
		if len([]rune(item.Title)) > 256 || len([]rune(item.Detail)) > 2048 {
			t.Fatal("unbounded summary content")
		}
	}
}

func TestRuntimeObservationsPreserveControlsAndExpirePresence(t *testing.T) {
	at := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	event := domain.Event{ProjectID: "project", SubjectType: domain.ResourceAgent, SubjectID: "agent:worker", SourceSystem: "codex", Type: "agent.observed", OccurredAt: at, Payload: json.RawMessage(`{"agent_id":"agent:worker","instance_id":"instance","display_name":"Worker","status":"idle","observed_at":"2026-10-15T12:00:00Z"}`)}
	raw, err := reduceRuntimeObservation(nil, 0, event)
	if err != nil {
		t.Fatal(err)
	}
	var agent domain.Agent
	if err := json.Unmarshal(raw, &agent); err != nil {
		t.Fatal(err)
	}
	if agent.PresenceExpiresAt == nil || !agent.PresenceExpiresAt.Equal(at.Add(5*time.Minute)) || agent.Status != "idle" || agent.ID != "agent:worker" {
		t.Fatalf("lease=%+v", agent)
	}
	current := domain.Run{Base: domain.NewBase("run", "project", at), AgentID: "agent:worker", ControlStatus: "pending", ControlAction: "cancel", Capabilities: []string{"run:cancel"}, Status: "running"}
	raw, _ = json.Marshal(current)
	event.SubjectType = domain.ResourceRun
	event.SubjectID = "run"
	event.Type = "run.observed"
	event.Payload = json.RawMessage(`{"agent_id":"agent:worker","run_id":"explicit","session_id":"native-thread","status":"completed","observed_at":"2026-10-15T12:00:00Z"}`)
	next, err := reduceRuntimeObservation(raw, 1, event)
	if err != nil {
		t.Fatal(err)
	}
	var run domain.Run
	if err := json.Unmarshal(next, &run); err != nil {
		t.Fatal(err)
	}
	if run.ControlStatus != current.ControlStatus || run.ControlAction != current.ControlAction || len(run.Capabilities) != 1 || run.Version != 2 || run.Status != "completed" || run.SessionID != domain.ObservedRuntimeID("project", "codex", "agent:worker", "session", "native-thread") {
		t.Fatalf("control overwrite=%+v", run)
	}
}

func TestCoordinationSummaryFailureAttentionWithoutBlockedTask(t *testing.T) {
	at := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	failure := domain.Attention{Base: domain.NewBase("failure", "project", at), Kind: "run.failed", Title: "Worker failed", Detail: "Actual failure detail", Status: "open", ResourceType: "run", ResourceID: "failed-run"}
	raw, _ := json.Marshal(failure)
	summary, err := summarizeCoordination(map[domain.ResourceType][]json.RawMessage{domain.ResourceAttention: {raw}}, app.Actor{Capabilities: map[string]struct{}{"resource:read": {}}}, at)
	if err != nil || summary.Counts.OpenAttention != 1 || summary.Counts.BlockedTasks != 0 || len(summary.Items) != 1 || summary.Items[0].Kind != "attention" || summary.Items[0].TargetID != "failed-run" || summary.Items[0].Detail != failure.Detail {
		t.Fatalf("failure urgency omitted=%+v err=%v", summary, err)
	}
	blocked := domain.Task{Base: domain.NewBase("blocked", "project", at), Status: "blocked", Title: "Blocked task"}
	taskRaw, _ := json.Marshal(blocked)
	failure.Kind, failure.ResourceType, failure.ResourceID = "task.blocked", "task", "blocked"
	raw, _ = json.Marshal(failure)
	summary, err = summarizeCoordination(map[domain.ResourceType][]json.RawMessage{domain.ResourceAttention: {raw}, domain.ResourceTask: {taskRaw}}, app.Actor{Capabilities: map[string]struct{}{"resource:read": {}}}, at)
	if err != nil || summary.Counts.OpenAttention != 1 || summary.Counts.BlockedTasks != 1 || len(summary.Items) != 1 || summary.Items[0].Kind != "blocker" {
		t.Fatalf("duplicate blocker preview=%+v err=%v", summary, err)
	}
}

func TestGitHubObservationsPreserveHumanFieldsAndSourceIsolation(t *testing.T) {
	at := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	external := "repo:42:pull:7"
	event := domain.Event{ProjectID: "project", SubjectType: domain.ResourceTask, SubjectID: domain.ObservedTaskIDForSource("project", "github", external), SourceSystem: "github", Type: "task.observed", OccurredAt: at, Payload: json.RawMessage(`{"external_id":"repo:42:pull:7","title":"PR title","status":"blocked","description":"Observed check failed","dependencies":["repo:42:issue:3"]}`)}
	raw, err := reduceResource(nil, 0, event)
	if err != nil {
		t.Fatal(err)
	}
	var task domain.Task
	if err := json.Unmarshal(raw, &task); err != nil {
		t.Fatal(err)
	}
	if task.Status != "inbox" || task.Source.System != "github" || task.SourceStatus != "blocked" || task.Dependencies[0] != domain.ObservedTaskIDForSource("project", "github", "repo:42:issue:3") {
		t.Fatalf("GitHub projection=%+v", task)
	}
	task.Title, task.Description, task.OwnerID, task.Status, task.Priority = "Human title", "Human body", "human", "review", "critical"
	task.Dependencies = []string{"human-dependency"}
	raw, _ = json.Marshal(task)
	event.Payload = json.RawMessage(`{"external_id":"repo:42:pull:7","title":"Latest PR title","status":"merged","description":"Observed merged state"}`)
	event.OccurredAt = at.Add(time.Minute)
	next, err := reduceResource(raw, 1, event)
	if err != nil {
		t.Fatal(err)
	}
	var updated domain.Task
	_ = json.Unmarshal(next, &updated)
	if updated.Title != task.Title || updated.Description != task.Description || updated.OwnerID != task.OwnerID || updated.Status != task.Status || updated.Priority != task.Priority || updated.Dependencies[0] != task.Dependencies[0] || updated.SourceStatus != "merged" || updated.SourceObservation.Title != "Latest PR title" {
		t.Fatalf("GitHub overwrote human fields=%+v", updated)
	}
	forged := event
	forged.SubjectID = domain.ObservedTaskID("project", external)
	if _, err := reduceResource(nil, 0, forged); err == nil {
		t.Fatal("GitHub claimed legacy Sentry task")
	}
	projectObservation := domain.ProjectObservation{ExternalID: "repo:42", Name: "owner/repo", Path: "https://github.com/owner/repo", Status: "active", CollectedAt: at, ScanStatus: "complete", TaskCount: 1}
	payload, _ := json.Marshal(projectObservation)
	project := domain.Event{ProjectID: "project", SubjectType: domain.ResourceProject, SubjectID: domain.ObservedProjectIDForSource("project", "github"), SourceSystem: "github", Type: "project.observed", OccurredAt: at, Payload: payload}
	raw, err = reduceProjectObservation(nil, 0, project, "Canonical human project")
	if err != nil {
		t.Fatal(err)
	}
	var report domain.Project
	_ = json.Unmarshal(raw, &report)
	if report.ID == "project" || report.Name != "Canonical human project" || report.Source.System != "github" || report.SourceObservation.Name != "owner/repo" {
		t.Fatalf("GitHub report identity=%+v", report)
	}
	project.SubjectID = "project"
	if _, err := reduceProjectObservation(nil, 0, project, "Canonical human project"); err == nil {
		t.Fatal("GitHub claimed Sentry project report")
	}
}

func TestGitHubReviewAndClosedHistoryDoNotImplySupervisedWork(t *testing.T) {
	at := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	documents := map[domain.ResourceType][]json.RawMessage{}
	for index, status := range []string{"review", "review_requested", "open", "draft", "closed", "merged", "unknown", "blocked"} {
		task := domain.Task{Base: domain.NewBase(fmt.Sprintf("github-%d", index), "project", at), Title: "Actual provider record", Status: "inbox", Source: domain.Source{System: "github"}, SourceStatus: status, SourceObservation: &domain.TaskObservation{Description: "Recorded review or CI context"}}
		raw, _ := json.Marshal(task)
		documents[domain.ResourceTask] = append(documents[domain.ResourceTask], raw)
	}
	summary, err := summarizeCoordination(documents, app.Actor{Capabilities: map[string]struct{}{"resource:read": {}}}, at)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Counts.SourceUnfinishedTasks != 5 || summary.Counts.UnfinishedTasks != 0 || summary.Counts.Reviews != 2 || summary.Counts.SourceBlockedTasks != 1 || summary.Counts.HumanEscalations != 0 || summary.Counts.RunningWork != 0 || summary.Counts.AgentPresence != 0 {
		t.Fatalf("source context falsely changed supervision/liveness=%+v", summary)
	}
	for _, item := range summary.Items {
		if item.Kind == "review" && (item.Status != "source_review" || item.Detail == "" || item.HumanEscalation) {
			t.Fatalf("source review attribution=%+v", item)
		}
	}
}
