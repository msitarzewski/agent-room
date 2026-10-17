package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTaskTransitionGraph(t *testing.T) {
	t.Parallel()
	allowed := [][2]string{
		{"inbox", "ready"}, {"ready", "working"}, {"working", "review"},
		{"review", "completed"}, {"completed", "archived"}, {"completed", "reopened"},
		{"reopened", "ready"}, {"blocked", "working"}, {"failed", "reopened"},
	}
	for _, transition := range allowed {
		if err := ValidateTaskTransition(transition[0], transition[1]); err != nil {
			t.Errorf("expected %s -> %s to be allowed: %v", transition[0], transition[1], err)
		}
	}
	for _, transition := range [][2]string{{"inbox", "completed"}, {"archived", "ready"}, {"cancelled", "working"}, {"working", "reopened"}} {
		if err := ValidateTaskTransition(transition[0], transition[1]); err == nil {
			t.Errorf("expected %s -> %s to be rejected", transition[0], transition[1])
		}
	}
}

func TestDestructiveActionsRequireApproval(t *testing.T) {
	t.Parallel()
	for _, action := range []string{"delete", "destroy", "force_push", "publish", "deploy_production", "rotate_secret"} {
		if !RequiresApproval(action) {
			t.Errorf("%s should require approval", action)
		}
	}
	if RequiresApproval("pause") {
		t.Error("pause should remain reversible without approval")
	}
}

func TestCommandAndBaseValidationFailures(t *testing.T) {
	t.Parallel()
	valid := Command{
		ID: "c", ProjectID: "p", Type: "task.transition", SubjectID: "t",
		ActorID: "u", IdempotencyKey: "key",
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	cases := []func(*Command){
		func(c *Command) { c.ID = " " },
		func(c *Command) { c.ProjectID = "" },
		func(c *Command) { c.Type = "" },
		func(c *Command) { c.SubjectID = "" },
		func(c *Command) { c.ActorID = "" },
		func(c *Command) { c.IdempotencyKey = "" },
		func(c *Command) { c.ExpectedVersion = -1 },
	}
	for index, mutate := range cases {
		command := valid
		mutate(&command)
		if err := command.Validate(); err == nil {
			t.Fatalf("invalid command case %d accepted", index)
		}
	}
	base := NewBase("id", "project", time.Date(2026, 7, 27, 12, 0, 0, 123, time.FixedZone("test", 3600)))
	if err := base.Validate(); err != nil || base.Version != 1 || base.CreatedAt.Location() != time.UTC {
		t.Fatalf("base=%+v err=%v", base, err)
	}
	for _, invalid := range []Base{
		{ID: "", ProjectID: "p", Version: 1},
		{ID: "id", ProjectID: "", Version: 1},
		{ID: "id", ProjectID: "p", Version: 0},
	} {
		if err := invalid.Validate(); err == nil {
			t.Fatalf("invalid base accepted: %+v", invalid)
		}
	}
	if err := ValidateTaskTransition("unknown", "ready"); err == nil {
		t.Fatal("unknown current task state accepted")
	}
	if !RequiresApproval("cancel") {
		t.Fatal("cancel should require approval")
	}
}

func TestDecodeEveryResourceTypeAndRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	kinds := []ResourceType{
		ResourceAgent, ResourceRun, ResourceSession, ResourceTask, ResourceAttention,
		ResourceEvidence, ResourceArtifact, ResourceApproval, ResourceIntervention,
		ResourceChatMessage, ResourceBudget, ResourceClaim, ResourceAudit,
		ResourceOrganization, ResourceHuman, ResourceProject, ResourceHost,
		ResourceAgentInstance, ResourceTaskTransition, ResourceSituation, ResourcePolicy,
		ResourceDeployment,
	}
	for _, kind := range kinds {
		if decoded, err := DecodeResource(kind, json.RawMessage(`{}`)); err != nil || decoded == nil {
			t.Fatalf("decode %s=%T err=%v", kind, decoded, err)
		}
	}
	if _, err := DecodeResource("unknown", json.RawMessage(`{}`)); err == nil {
		t.Fatal("unknown resource decoded")
	}
	if _, err := DecodeResource(ResourceTask, json.RawMessage(`{`)); err == nil {
		t.Fatal("invalid JSON decoded")
	}
}

func TestSourceObservationIdentitiesPreserveSentryAndIsolateGitHub(t *testing.T) {
	const legacy = "sentry_task_0b2f41d69cb1d54fea831c504ebaaada"
	if got := ObservedTaskID("project", "native"); got != legacy {
		t.Fatalf("legacy Sentry identity changed=%s", got)
	}
	if ObservedTaskIDForSource("project", "sentry", "native") != legacy {
		t.Fatal("source helper changed legacy Sentry identity")
	}
	github := ObservedTaskIDForSource("project", "github", "native")
	if github == legacy || github == ObservedTaskIDForSource("other-project", "github", "native") || github != ObservedTaskIDForSource("project", "github", "native") {
		t.Fatal("GitHub task identity is not stable/project/source scoped")
	}
	if ObservedProjectIDForSource("project", "sentry") != "project" {
		t.Fatal("legacy Sentry project identity changed")
	}
	githubProject := ObservedProjectIDForSource("project", "github")
	if githubProject == "project" || githubProject == ObservedProjectIDForSource("other-project", "github") || githubProject != ObservedProjectIDForSource("project", "github") {
		t.Fatal("source project reports are not isolated")
	}
	for _, source := range []string{"sentry", "github"} {
		if !IsTaskObservationSource(source) {
			t.Fatalf("supported source rejected=%s", source)
		}
	}
	for _, source := range []string{"", "codex", "claude", "hermes", "future-source"} {
		if IsTaskObservationSource(source) {
			t.Fatalf("unapproved source generalized=%s", source)
		}
	}
	for _, test := range []struct {
		system, status              string
		unfinished, review, blocked bool
	}{
		{"sentry", "Upcoming", true, false, false}, {"sentry", "Done", false, false, false}, {"sentry", "Unknown", false, false, false},
		{"github", "open", true, false, false}, {"github", "draft", true, false, false}, {"github", " REVIEW_REQUESTED ", true, true, false}, {"github", "review", true, true, false}, {"github", " Blocked ", true, false, true}, {"github", "closed", false, false, false}, {"github", "merged", false, false, false}, {"github", "unknown", false, false, false}, {"other", "open", false, false, false},
	} {
		if SourceTaskUnfinished(test.system, test.status) != test.unfinished || SourceTaskReview(test.system, test.status) != test.review || SourceTaskBlocked(test.status) != test.blocked {
			t.Fatalf("status classification=%+v", test)
		}
	}
}
