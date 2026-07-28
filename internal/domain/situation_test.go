package domain

import (
	"testing"
	"time"
)

func TestSituationDedupeAndMaterialChangeReopen(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	observed := Situation{Base: NewBase("s1", "p1", now), Kind: "run_failed", Fingerprint: "run:r1:failed", Severity: "high", Summary: "Run failed", MaterialHash: "sha256:a"}
	current, opened, err := ReconcileSituation(nil, observed, now)
	if err != nil || !opened || current.Occurrences != 1 || current.Status != "open" {
		t.Fatalf("current=%+v opened=%v err=%v", current, opened, err)
	}
	duplicate, changed, err := ReconcileSituation(&current, observed, now.Add(time.Minute))
	if err != nil || changed || duplicate.Occurrences != 2 {
		t.Fatalf("duplicate=%+v changed=%v err=%v", duplicate, changed, err)
	}
	resolved, err := ResolveSituation(duplicate, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	observed.MaterialHash = "sha256:b"
	observed.Detail = "new failure signature"
	reopened, changed, err := ReconcileSituation(&resolved, observed, now.Add(3*time.Minute))
	if err != nil || !changed || reopened.Status != "open" || reopened.ResolvedAt != nil {
		t.Fatalf("reopened=%+v changed=%v err=%v", reopened, changed, err)
	}
}

func TestTaskTransitionEntityValidatesGraph(t *testing.T) {
	transition := TaskTransition{Base: NewBase("tr1", "p1", time.Now()), TaskID: "t1", From: "ready", To: "working", ActorID: "u1", CommandID: "c1"}
	if err := transition.Validate(); err != nil {
		t.Fatal(err)
	}
	transition.To = "archived"
	if err := transition.Validate(); err == nil {
		t.Fatal("invalid task transition accepted")
	}
}

func TestSituationRejectsInvalidLifecycleOperations(t *testing.T) {
	t.Parallel()
	now := time.Now()
	for _, observed := range []Situation{
		{Kind: "", Fingerprint: "f", MaterialHash: "m"},
		{Kind: "k", Fingerprint: "", MaterialHash: "m"},
		{Kind: "k", Fingerprint: "f", MaterialHash: ""},
	} {
		if _, _, err := ReconcileSituation(nil, observed, now); err == nil {
			t.Fatalf("invalid observed situation accepted: %+v", observed)
		}
	}
	if _, err := ResolveSituation(Situation{Status: "resolved"}, now); err == nil {
		t.Fatal("resolved situation resolved twice")
	}
	transition := TaskTransition{Base: Base{ID: "", ProjectID: "p", Version: 1}}
	if err := transition.Validate(); err == nil {
		t.Fatal("transition with invalid base accepted")
	}
	transition.Base = NewBase("tr", "p", now)
	if err := transition.Validate(); err == nil {
		t.Fatal("transition without required references accepted")
	}
}
