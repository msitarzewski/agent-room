package postgres

import (
	"encoding/json"
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
