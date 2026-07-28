package security_test

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/msitarzewski/agent-room/internal/app"
	"github.com/msitarzewski/agent-room/internal/domain"
)

type approvalFailureRepository struct {
	approvalConsumed bool
}

func (*approvalFailureRepository) Get(context.Context, string, domain.ResourceType, string) (json.RawMessage, error) {
	return nil, app.ErrNotFound
}

func (*approvalFailureRepository) List(context.Context, string, domain.ResourceType, string, int) (app.Page, error) {
	return app.Page{}, nil
}

func (*approvalFailureRepository) Execute(context.Context, domain.Command, func(json.RawMessage) (app.Mutation, error)) (app.CommandResult, error) {
	return app.CommandResult{}, app.ErrVersionConflict
}

func (*approvalFailureRepository) Ingest(context.Context, []domain.Event) error {
	return nil
}

func (*approvalFailureRepository) Events(context.Context, string, int64, int) (app.EventPage, error) {
	return app.EventPage{}, nil
}

func (*approvalFailureRepository) Overview(context.Context, string) (map[string]any, error) {
	return nil, nil
}

func (r *approvalFailureRepository) ConsumeApproval(context.Context, string, string, string, string, time.Time, domain.Event, domain.AuditRecord) error {
	r.approvalConsumed = true
	return nil
}

func (*approvalFailureRepository) Health(context.Context) error { return nil }
func (*approvalFailureRepository) Close()                       {}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type sequenceIDs int

func (s *sequenceIDs) New() string {
	*s++
	return "security-" + strconv.Itoa(int(*s))
}

func TestApprovalIsNotConsumedWhenDestructiveActionFails(t *testing.T) {
	repository := &approvalFailureRepository{}
	ids := sequenceIDs(0)
	service := app.NewServiceWithDependencies(
		repository,
		nil,
		fixedClock{now: time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)},
		&ids,
	)
	actor := app.Actor{ID: "operator", Capabilities: map[string]struct{}{"run:intervene": {}}}

	_, err := service.RunAction(
		context.Background(),
		actor,
		"project-one",
		"run-one",
		"cancel",
		"approved cancellation",
		"approval-one",
		"idempotency-one",
		7,
		"security-test",
	)
	if !errors.Is(err, app.ErrDenied) {
		t.Fatalf("RunAction error = %v, want generic denial", err)
	}
	if repository.approvalConsumed {
		t.Fatal("one-shot approval was consumed even though the destructive action failed")
	}
}

type commitFailureRepository struct {
	approvalFailureRepository
	current json.RawMessage
}

func (r *commitFailureRepository) Execute(
	_ context.Context,
	_ domain.Command,
	build func(json.RawMessage) (app.Mutation, error),
) (app.CommandResult, error) {
	if _, err := build(r.current); err != nil {
		return app.CommandResult{}, err
	}
	return app.CommandResult{}, app.ErrVersionConflict
}

type countingController struct{ calls int }

func (*countingController) Supports(string, string) bool { return true }

func (c *countingController) Execute(context.Context, string, string, string) error {
	c.calls++
	return nil
}

func TestRuntimeSideEffectDoesNotOccurBeforeCommandCommit(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	current, err := json.Marshal(domain.Run{
		Base:   domain.NewBase("run-one", "project-one", now),
		Status: "running",
	})
	if err != nil {
		t.Fatal(err)
	}
	repository := &commitFailureRepository{current: current}
	controller := &countingController{}
	service := app.NewServiceWithController(repository, nil, controller)
	actor := app.Actor{ID: "operator", Capabilities: map[string]struct{}{"run:pause": {}}}

	_, err = service.RunAction(
		context.Background(),
		actor,
		"project-one",
		"run-one",
		"pause",
		"pause for review",
		"",
		"idempotency-pause",
		1,
		"security-test",
	)
	if !errors.Is(err, app.ErrVersionConflict) {
		t.Fatalf("RunAction error = %v, want version conflict", err)
	}
	if controller.calls != 0 {
		t.Fatalf("runtime side effect occurred %d time(s) before the command committed", controller.calls)
	}
}
