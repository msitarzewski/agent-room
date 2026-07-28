package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/msitarzewski/agent-room/internal/domain"
)

func applyEventProjection(ctx context.Context, tx pgx.Tx, event domain.Event) error {
	if event.SubjectID == "" || !projectable(event.SubjectType) {
		return nil
	}
	var currentRaw []byte
	var currentVersion, currentSequence int64
	var currentSource string
	err := tx.QueryRow(ctx, `SELECT document,version,COALESCE(source_system,''),COALESCE(source_sequence,0)
		FROM resources WHERE project_id=$1 AND kind=$2 AND id=$3 FOR UPDATE`,
		event.ProjectID, event.SubjectType, event.SubjectID).Scan(&currentRaw, &currentVersion, &currentSource, &currentSequence)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if event.SourceSequence > 0 && currentSource == event.SourceSystem && currentSequence >= event.SourceSequence {
		return deriveAttention(ctx, tx, event)
	}
	document, err := reduceResource(currentRaw, currentVersion, event)
	if err != nil {
		return err
	}
	var base struct {
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Version   int64     `json:"version"`
	}
	if err := json.Unmarshal(document, &base); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO resources(project_id,kind,id,version,document,created_at,updated_at,source_system,source_sequence)
		VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),NULLIF($9,0))
		ON CONFLICT(project_id,kind,id) DO UPDATE SET version=excluded.version,document=excluded.document,
		updated_at=excluded.updated_at,source_system=excluded.source_system,source_sequence=excluded.source_sequence`,
		event.ProjectID, event.SubjectType, event.SubjectID, base.Version, document, base.CreatedAt, base.UpdatedAt, event.SourceSystem, event.SourceSequence)
	if err != nil {
		return err
	}
	return deriveAttention(ctx, tx, event)
}

func reduceResource(current json.RawMessage, currentVersion int64, event domain.Event) (json.RawMessage, error) {
	if event.SubjectType == domain.ResourceBudget && event.Type == "budget.usage_observed" {
		return reduceBudgetUsage(current, event)
	}
	var payload struct {
		Resource json.RawMessage `json:"resource"`
	}
	if json.Unmarshal(event.Payload, &payload) == nil && len(payload.Resource) > 0 {
		return payload.Resource, nil
	}
	document := map[string]any{}
	if len(current) > 0 {
		if err := json.Unmarshal(current, &document); err != nil {
			return nil, err
		}
	}
	if currentVersion == 0 {
		document["id"], document["project_id"], document["created_at"] = event.SubjectID, event.ProjectID, event.OccurredAt
		currentVersion = 0
	}
	document["version"], document["updated_at"] = currentVersion+1, event.OccurredAt
	document["source"] = domain.Source{System: event.SourceSystem, ExternalID: event.SourceEventID}
	document["last_event_type"] = event.Type
	switch event.SubjectType {
	case domain.ResourceRun:
		document["status"] = statusFromEvent(event.Type, "running")
		if _, ok := document["capabilities"]; !ok {
			document["capabilities"] = []string{}
		}
	case domain.ResourceSession:
		document["status"] = statusFromEvent(event.Type, "active")
	case domain.ResourceTask:
		document["status"] = statusFromEvent(event.Type, "working")
	case domain.ResourceBudget:
		document["status"] = "observed"
	}
	return json.Marshal(document)
}

func reduceBudgetUsage(current json.RawMessage, event domain.Event) (json.RawMessage, error) {
	if len(current) == 0 {
		return nil, errors.New("budget usage cannot be observed before the budget exists")
	}
	var budget domain.Budget
	if err := json.Unmarshal(current, &budget); err != nil {
		return nil, err
	}
	var usage struct {
		TokenDelta      int64 `json:"token_delta"`
		CostDeltaCents  int64 `json:"cost_delta_cents"`
		TimeDeltaSec    int64 `json:"time_delta_seconds"`
		ConcurrentDelta int64 `json:"concurrent_delta"`
	}
	if err := json.Unmarshal(event.Payload, &usage); err != nil {
		return nil, err
	}
	if usage.TokenDelta < 0 || usage.CostDeltaCents < 0 || usage.TimeDeltaSec < 0 {
		return nil, errors.New("cumulative budget deltas cannot be negative")
	}
	budget.TokenUsed += usage.TokenDelta
	budget.CostUsedCents += usage.CostDeltaCents
	budget.TimeUsedSec += usage.TimeDeltaSec
	budget.ConcurrentUsed += usage.ConcurrentDelta
	if budget.ConcurrentUsed < 0 {
		return nil, errors.New("concurrent budget usage cannot become negative")
	}
	budget.Status = "available"
	if exceeds(budget.TokenUsed, budget.TokenLimit) || exceeds(budget.CostUsedCents, budget.CostLimitCents) ||
		exceeds(budget.TimeUsedSec, budget.TimeLimitSec) || exceeds(budget.ConcurrentUsed, budget.ConcurrentLimit) {
		budget.Status = "exhausted"
	}
	if budget.EnforcementMode == "" {
		budget.EnforcementMode = "observed"
	}
	budget.Version++
	budget.UpdatedAt = event.OccurredAt
	return json.Marshal(budget)
}

func exceeds(used, limit int64) bool {
	return limit > 0 && used >= limit
}

func statusFromEvent(eventType, fallback string) string {
	switch {
	case strings.HasSuffix(eventType, ".started"), strings.HasSuffix(eventType, ".progressed"):
		return fallback
	case strings.HasSuffix(eventType, ".completed"):
		return "completed"
	case strings.HasSuffix(eventType, ".failed"):
		return "failed"
	case eventType == "task.blocked":
		return "blocked"
	default:
		return fallback
	}
}

func deriveAttention(ctx context.Context, tx pgx.Tx, event domain.Event) error {
	actionable := strings.HasSuffix(event.Type, ".failed") || event.Type == "task.blocked" ||
		event.Type == "approval.requested" || event.Type == "budget.rate_limit_observed"
	if event.SubjectType == domain.ResourceBudget && event.Type == "budget.usage_observed" {
		raw, err := resourceForUpdate(ctx, tx, event.ProjectID, domain.ResourceBudget, event.SubjectID)
		if err != nil {
			return err
		}
		var budget domain.Budget
		if json.Unmarshal(raw, &budget) == nil && budget.Status == "exhausted" {
			actionable = true
		}
	}
	if !actionable {
		return nil
	}
	fingerprint := stableID(event.ProjectID, event.Type, string(event.SubjectType), event.SubjectID)
	material := sha256.Sum256(event.Payload)
	now := event.OccurredAt
	situationID, attentionID := "sit_"+fingerprint, "attn_"+fingerprint
	var current domain.Situation
	var currentPtr *domain.Situation
	raw, err := resourceForUpdate(ctx, tx, event.ProjectID, domain.ResourceSituation, situationID)
	if err != nil {
		return err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &current); err != nil {
			return err
		}
		currentPtr = &current
	}
	observed := domain.Situation{
		Base: domain.NewBase(situationID, event.ProjectID, now), Kind: event.Type, Fingerprint: fingerprint,
		Severity: severityFor(event.Type), Summary: event.Type + " requires attention",
		MaterialHash: "sha256:" + hex.EncodeToString(material[:]),
	}
	next, changed, err := domain.ReconcileSituation(currentPtr, observed, now)
	if err != nil {
		return err
	}
	if err := upsertProjection(ctx, tx, domain.ResourceSituation, next.Base, next); err != nil {
		return err
	}
	if currentPtr != nil && !changed {
		return nil
	}
	attention := domain.Attention{
		Base: domain.NewBase(attentionID, event.ProjectID, now), Kind: event.Type, Severity: observed.Severity,
		Title: observed.Summary, ResourceType: string(event.SubjectType), ResourceID: event.SubjectID, Status: "open",
	}
	existing, err := resourceForUpdate(ctx, tx, event.ProjectID, domain.ResourceAttention, attentionID)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		var prior domain.Attention
		if err := json.Unmarshal(existing, &prior); err != nil {
			return err
		}
		attention.Base = prior.Base
		attention.Version, attention.UpdatedAt = prior.Version+1, now
	}
	return upsertProjection(ctx, tx, domain.ResourceAttention, attention.Base, attention)
}

func resourceForUpdate(ctx context.Context, tx pgx.Tx, projectID string, kind domain.ResourceType, id string) ([]byte, error) {
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT document FROM resources WHERE project_id=$1 AND kind=$2 AND id=$3 FOR UPDATE`, projectID, kind, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return raw, err
}

func upsertProjection(ctx context.Context, tx pgx.Tx, kind domain.ResourceType, base domain.Base, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO resources(project_id,kind,id,version,document,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT(project_id,kind,id) DO UPDATE SET version=excluded.version,document=excluded.document,updated_at=excluded.updated_at`,
		base.ProjectID, kind, base.ID, base.Version, raw, base.CreatedAt, base.UpdatedAt)
	return err
}

func stableID(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:16])
}

func severityFor(eventType string) string {
	if strings.HasSuffix(eventType, ".failed") {
		return "high"
	}
	return "normal"
}

func projectable(kind domain.ResourceType) bool {
	switch kind {
	case domain.ResourceAgent, domain.ResourceAgentInstance, domain.ResourceRun, domain.ResourceSession,
		domain.ResourceTask, domain.ResourceEvidence, domain.ResourceArtifact, domain.ResourceApproval,
		domain.ResourceBudget, domain.ResourceDeployment:
		return true
	default:
		return false
	}
}
