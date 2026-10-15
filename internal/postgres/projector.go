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
	"github.com/msitarzewski/agent-room/internal/app"
	"github.com/msitarzewski/agent-room/internal/domain"
)

func applyEventProjection(ctx context.Context, tx pgx.Tx, event domain.Event) error {
	sourceProject := event.SourceSystem == "sentry" && event.Type == "project.observed" && event.SubjectType == domain.ResourceProject
	if event.SubjectID == "" || (!projectable(event.SubjectType) && !sourceProject && len(commandResource(event)) == 0) {
		return nil
	}
	// External approval requests may raise attention, but only authenticated
	// commands create or decide an approval. Non-usage budget observations
	// likewise cannot clear an enforced budget's exhaustion state.
	if event.SourceSystem != "" && (event.SubjectType == domain.ResourceApproval ||
		(event.SubjectType == domain.ResourceBudget && event.Type != "budget.usage_observed")) {
		return deriveAttention(ctx, tx, event)
	}
	// A row lock cannot serialize the first observations of a missing resource.
	// Use the same transaction-scoped identity lock before testing existence so
	// concurrent first deliveries cannot overwrite a newer source sequence.
	identity := event.ProjectID + "\x1f" + string(event.SubjectType) + "\x1f" + event.SubjectID
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, identity); err != nil {
		return err
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
		return nil
	}
	var document json.RawMessage
	if sourceProject {
		var canonicalName string
		if err := tx.QueryRow(ctx, `SELECT name FROM projects WHERE id=$1`, event.ProjectID).Scan(&canonicalName); err != nil {
			return err
		}
		document, err = reduceProjectObservation(currentRaw, currentVersion, event, canonicalName)
	} else {
		document, err = reduceResource(currentRaw, currentVersion, event)
	}
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
		VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),NULLIF($9::bigint,0))
		ON CONFLICT(project_id,kind,id) DO UPDATE SET version=excluded.version,document=excluded.document,
		updated_at=excluded.updated_at,source_system=COALESCE(excluded.source_system,resources.source_system),
		source_sequence=CASE WHEN excluded.source_system IS NULL THEN resources.source_sequence ELSE excluded.source_sequence END`,
		event.ProjectID, event.SubjectType, event.SubjectID, base.Version, document, base.CreatedAt, base.UpdatedAt, event.SourceSystem, event.SourceSequence)
	if err != nil {
		return err
	}
	if event.SourceSystem == "sentry" && event.Type == "task.observed" && event.SubjectType == domain.ResourceTask {
		var prior domain.Task
		if len(currentRaw) > 0 {
			if err := json.Unmarshal(currentRaw, &prior); err != nil {
				return err
			}
		}
		return reconcileSourceTaskAttention(ctx, tx, event, prior.SourceStatus)
	}
	return deriveAttention(ctx, tx, event)
}

// commandResource is reserved for stored server-authored command events.
// Private ingestion requires a source_system and clears supplied command IDs.
func commandResource(event domain.Event) json.RawMessage {
	if event.SourceSystem != "" || event.CommandID == "" {
		return nil
	}
	var payload struct {
		Resource json.RawMessage `json:"resource"`
	}
	if json.Unmarshal(event.Payload, &payload) != nil {
		return nil
	}
	return payload.Resource
}

func reduceResource(current json.RawMessage, currentVersion int64, event domain.Event) (json.RawMessage, error) {
	if resource := commandResource(event); len(resource) > 0 {
		if _, err := domain.DecodeResource(event.SubjectType, resource); err != nil {
			return nil, err
		}
		var base domain.Base
		if err := json.Unmarshal(resource, &base); err != nil {
			return nil, err
		}
		if base.ID != event.SubjectID || base.ProjectID != event.ProjectID || base.Validate() != nil {
			return nil, errors.New("command replay resource identity mismatch")
		}
		return resource, nil
	}
	if event.SubjectType == domain.ResourceBudget && event.Type == "budget.usage_observed" {
		return reduceBudgetUsage(current, event)
	}
	if event.SourceSystem == "sentry" && event.Type == "task.observed" && event.SubjectType == domain.ResourceTask {
		return reduceTaskObservation(current, currentVersion, event)
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
		document["source_status"] = statusFromEvent(event.Type, "working")
		if _, ok := document["status"]; !ok {
			document["status"] = "inbox"
		}
	case domain.ResourceBudget:
		document["status"] = "observed"
	}
	return json.Marshal(document)
}

func reduceProjectObservation(current json.RawMessage, currentVersion int64, event domain.Event, canonicalName string) (json.RawMessage, error) {
	var observation domain.ProjectObservation
	if err := json.Unmarshal(event.Payload, &observation); err != nil {
		return nil, err
	}
	if event.SubjectID != event.ProjectID || observation.ExternalID == "" {
		return nil, app.Invalid("project observation identity mismatch")
	}
	var project domain.Project
	if len(current) == 0 {
		project = domain.Project{Base: domain.NewBase(event.ProjectID, event.ProjectID, event.OccurredAt), Name: canonicalName, Slug: event.ProjectID, Status: "observed"}
	} else {
		if err := json.Unmarshal(current, &project); err != nil {
			return nil, err
		}
		if (project.Source.System != "" && project.Source.System != "sentry") || (project.Source.ExternalID != "" && project.Source.ExternalID != observation.ExternalID) {
			return nil, app.Invalid("project observation cannot claim another source's identity")
		}
		project.Version = currentVersion + 1
		project.UpdatedAt = event.OccurredAt
	}
	project.Source = domain.Source{System: "sentry", ExternalID: observation.ExternalID}
	project.SourceStatus = observation.Status
	observedAt := observation.CollectedAt.UTC()
	project.SourceObservedAt = &observedAt
	project.SourceObservation = &observation
	if observation.ScanStatus == "complete" {
		project.SourceLastSuccessfulCollectionAt = &observedAt
	}
	return json.Marshal(project)
}

func reduceTaskObservation(current json.RawMessage, currentVersion int64, event domain.Event) (json.RawMessage, error) {
	var observation domain.TaskObservation
	if err := json.Unmarshal(event.Payload, &observation); err != nil {
		return nil, err
	}
	if observation.ExternalID == "" || event.SubjectID != domain.ObservedTaskID(event.ProjectID, observation.ExternalID) {
		return nil, errors.New("task observation identity mismatch")
	}
	var task domain.Task
	if len(current) == 0 {
		task = domain.Task{
			Base:  domain.NewBase(event.SubjectID, event.ProjectID, event.OccurredAt),
			Title: observation.Title, Description: observation.Description, Status: "inbox", Priority: "normal",
			Dependencies: []string{}, Source: domain.Source{System: "sentry", ExternalID: observation.ExternalID, ExternalURL: observation.ExternalURL},
		}
		for _, dependency := range observation.Dependencies {
			task.Dependencies = append(task.Dependencies, domain.ObservedTaskID(event.ProjectID, dependency))
		}
	} else {
		if err := json.Unmarshal(current, &task); err != nil {
			return nil, err
		}
		if task.Source.System != "sentry" || task.Source.ExternalID != observation.ExternalID {
			return nil, errors.New("task observation cannot claim another source's task")
		}
		task.Version = currentVersion + 1
		task.UpdatedAt = event.OccurredAt
	}
	task.SourceStatus = observation.Status
	observedAt := event.OccurredAt.UTC()
	task.SourceObservedAt = &observedAt
	task.SourceObservation = &observation
	return json.Marshal(task)
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

const sourceTaskBlocked = "source.task.blocked"

func reconcileSourceTaskAttention(ctx context.Context, tx pgx.Tx, event domain.Event, previousStatus string) error {
	var observation domain.TaskObservation
	if err := json.Unmarshal(event.Payload, &observation); err != nil {
		return err
	}
	fingerprint := stableID(event.ProjectID, sourceTaskBlocked, string(domain.ResourceTask), event.SubjectID)
	attentionID := "attn_" + fingerprint
	raw, err := resourceForUpdate(ctx, tx, event.ProjectID, domain.ResourceAttention, attentionID)
	if err != nil {
		return err
	}
	var current *domain.Attention
	if len(raw) > 0 {
		current = &domain.Attention{}
		if err := json.Unmarshal(raw, current); err != nil {
			return err
		}
	}
	next, changed := reduceSourceTaskAttention(current, event.ProjectID, event.SubjectID, observation, previousStatus, event.OccurredAt)
	if !changed {
		return nil
	}
	return upsertProjection(ctx, tx, domain.ResourceAttention, next.Base, next)
}

// Source attention has a separate deterministic identity and kind. A source can
// resolve only this derived item, leaving human attention and task status alone.
func reduceSourceTaskAttention(current *domain.Attention, projectID, taskID string, observation domain.TaskObservation, previousStatus string, at time.Time) (domain.Attention, bool) {
	fingerprint := stableID(projectID, sourceTaskBlocked, string(domain.ResourceTask), taskID)
	id := "attn_" + fingerprint
	if current != nil && (current.ID != id || current.ProjectID != projectID || current.Kind != sourceTaskBlocked ||
		current.ResourceType != string(domain.ResourceTask) || current.ResourceID != taskID) {
		return *current, false
	}
	blocked := strings.EqualFold(strings.TrimSpace(observation.Status), "blocked")
	if !blocked {
		if current == nil || current.Status == "resolved" {
			return domain.Attention{}, false
		}
		next := *current
		next.Status, next.ResolvedAt = "resolved", &at
		next.Version++
		next.UpdatedAt = at
		return next, true
	}
	title := "Source reports blocked: " + observation.Title
	if current == nil {
		return domain.Attention{
			Base: domain.NewBase(id, projectID, at), Kind: sourceTaskBlocked, Severity: "normal",
			Title: title, Detail: observation.Description, ResourceType: string(domain.ResourceTask), ResourceID: taskID, Status: "open",
		}, true
	}
	// A human may have resolved this item while the source was still blocked.
	// Only a new observed nonblocked-to-blocked episode reopens that decision.
	if current.Status == "resolved" && strings.EqualFold(strings.TrimSpace(previousStatus), "blocked") {
		return *current, false
	}
	if current.Title == title && current.Detail == observation.Description && current.Status != "resolved" {
		return *current, false
	}
	next := *current
	next.Title, next.Detail, next.Status = title, observation.Description, "open"
	next.ResolvedAt, next.AcknowledgedAt = nil, nil
	next.Version++
	next.UpdatedAt = at
	return next, true
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
