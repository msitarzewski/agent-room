package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/msitarzewski/agent-room/internal/app"
	"github.com/msitarzewski/agent-room/internal/domain"
	"github.com/msitarzewski/agent-room/internal/postgres/sqlcgen"
)

type Repository struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
}

func Open(ctx context.Context, databaseURL string) (*Repository, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	config.MaxConns = 20
	config.MinConns = 2
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute
	config.HealthCheckPeriod = 30 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &Repository{pool: pool, queries: sqlcgen.New(pool)}, nil
}

func (r *Repository) Pool() *pgxpool.Pool { return r.pool }
func (r *Repository) Close()              { r.pool.Close() }
func (r *Repository) Health(ctx context.Context) error {
	_, err := r.queries.ResourceHealth(ctx)
	return err
}

func (r *Repository) ArtifactDigestReferenced(ctx context.Context, digest string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM resources WHERE kind=$1 AND document->>'digest'=$2
	)`, domain.ResourceArtifact, digest).Scan(&exists)
	return exists, err
}

func (r *Repository) ComponentHealth(ctx context.Context) (map[string]any, error) {
	result := map[string]any{}
	if err := r.Health(ctx); err != nil {
		return nil, err
	}
	status, err := MigrationStatus(ctx, r.pool)
	if err != nil {
		return nil, err
	}
	schemaCurrent := true
	for _, migration := range status {
		if migration["status"] != "applied" {
			schemaCurrent = false
			break
		}
	}
	result["schema"] = map[string]any{"status": statusText(schemaCurrent), "migrations": len(status)}
	var eventPending, controlPending int64
	var oldestEventSeconds float64
	if err := r.pool.QueryRow(ctx, `SELECT count(*),COALESCE(EXTRACT(EPOCH FROM now()-min(available_at)),0)
		FROM event_outbox WHERE published_at IS NULL`).Scan(&eventPending, &oldestEventSeconds); err != nil {
		return nil, err
	}
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM run_control_outbox
		WHERE status IN ('pending','executing')`).Scan(&controlPending); err != nil {
		return nil, err
	}
	result["event_outbox"] = map[string]any{"status": statusText(oldestEventSeconds < 30), "pending": eventPending, "oldest_seconds": oldestEventSeconds}
	result["control_outbox"] = map[string]any{"status": statusText(controlPending == 0), "pending": controlPending}
	rows, err := r.pool.Query(ctx, `SELECT source_system,max(occurred_at) FROM events
		WHERE source_system IS NOT NULL GROUP BY source_system ORDER BY source_system`)
	if err != nil {
		return nil, err
	}
	lastSeen := map[string]time.Time{}
	for rows.Next() {
		var source string
		var at time.Time
		if err := rows.Scan(&source, &at); err != nil {
			rows.Close()
			return nil, err
		}
		lastSeen[source] = at
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result["adapters"] = map[string]any{"status": "ok", "last_seen": lastSeen}
	return result, nil
}

func (r *Repository) BriefCursor(ctx context.Context, projectID, actorID string) (int64, error) {
	var cursor int64
	err := r.pool.QueryRow(ctx, `SELECT last_cursor FROM brief_cursors
		WHERE project_id=$1 AND actor_id=$2`, projectID, actorID).Scan(&cursor)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return cursor, err
}

func (r *Repository) AcknowledgeBrief(ctx context.Context, projectID, actorID, commandID, idempotencyKey string, expected, through int64, at time.Time) (bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var replayThrough int64
	err = tx.QueryRow(ctx, `SELECT through_cursor FROM brief_acknowledgements
		WHERE project_id=$1 AND actor_id=$2 AND idempotency_key=$3`,
		projectID, actorID, idempotencyKey).Scan(&replayThrough)
	if err == nil {
		if replayThrough != through {
			return false, app.ErrIdempotency
		}
		return true, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	var current int64
	err = tx.QueryRow(ctx, `SELECT last_cursor FROM brief_cursors
		WHERE project_id=$1 AND actor_id=$2 FOR UPDATE`, projectID, actorID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		current = 0
	} else if err != nil {
		return false, err
	}
	if expected != current || through < current {
		return false, app.ErrVersionConflict
	}
	var latest int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(cursor),0) FROM events WHERE project_id=$1`, projectID).Scan(&latest); err != nil {
		return false, err
	}
	if through > latest {
		return false, app.Invalid("through_cursor exceeds the latest project event")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO brief_cursors(project_id,actor_id,last_cursor,updated_at)
		VALUES($1,$2,$3,$4) ON CONFLICT(project_id,actor_id)
		DO UPDATE SET last_cursor=excluded.last_cursor,updated_at=excluded.updated_at`,
		projectID, actorID, through, at); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO brief_acknowledgements(
		project_id,actor_id,idempotency_key,command_id,expected_cursor,through_cursor,accepted_at
	) VALUES($1,$2,$3,$4,$5,$6,$7)`,
		projectID, actorID, idempotencyKey, commandID, expected, through, at); err != nil {
		return false, err
	}
	details, _ := json.Marshal(map[string]int64{"expected_cursor": expected, "through_cursor": through})
	auditID := commandID + ":audit"
	if _, err := tx.Exec(ctx, `INSERT INTO audit_records(
		id,project_id,actor_id,action,resource_type,resource_id,command_id,outcome,details,occurred_at
	) VALUES($1,$2,$3,'acknowledge.brief','brief_cursor',$3,$4,'accepted',$5,$6)`,
		auditID, projectID, actorID, commandID, details, at); err != nil {
		return false, err
	}
	return false, tx.Commit(ctx)
}

func (r *Repository) RecordDecision(ctx context.Context, decision app.Decision) error {
	details, _ := json.Marshal(map[string]string{
		"reason_class": decision.ReasonClass, "correlation_id": decision.CorrelationID,
		"idempotency_key": decision.IdempotencyKey,
	})
	auditID := "decision_" + stableID(decision.ProjectID, decision.ActorID, decision.Action, decision.ResourceType, decision.ResourceID, decision.IdempotencyKey, decision.ReasonClass)
	audit := domain.AuditRecord{
		Base: domain.NewBase(auditID, decision.ProjectID, decision.OccurredAt), ActorID: decision.ActorID,
		Action: decision.Action, ResourceType: decision.ResourceType, ResourceID: decision.ResourceID,
		Outcome: decision.Outcome, Details: details,
	}
	raw, _ := json.Marshal(audit)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `INSERT INTO audit_records(
		id,project_id,actor_id,action,resource_type,resource_id,outcome,details,occurred_at
	) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(id) DO NOTHING`,
		auditID, decision.ProjectID, decision.ActorID, decision.Action, decision.ResourceType,
		decision.ResourceID, decision.Outcome, details, decision.OccurredAt); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO resources(project_id,kind,id,version,document,created_at,updated_at)
		VALUES($1,$2,$3,1,$4,$5,$5) ON CONFLICT(project_id,kind,id) DO NOTHING`,
		decision.ProjectID, domain.ResourceAudit, auditID, raw, decision.OccurredAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func statusText(ok bool) string {
	if ok {
		return "ok"
	}
	return "degraded"
}

func (r *Repository) Get(ctx context.Context, projectID string, kind domain.ResourceType, id string) (json.RawMessage, error) {
	raw, err := r.queries.GetResource(ctx, sqlcgen.GetResourceParams{ProjectID: projectID, Kind: string(kind), ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, app.ErrNotFound
	}
	return raw, err
}

func (r *Repository) List(ctx context.Context, projectID string, kind domain.ResourceType, cursor string, limit int) (app.Page, error) {
	return r.listResources(ctx, projectID, kind, cursor, limit, "", "")
}

func (r *Repository) ListCoordination(ctx context.Context, projectID string, actor app.Actor, cursor string, limit int) (app.Page, error) {
	participantType := "agent"
	if actor.Human {
		participantType = "human"
	}
	return r.listResources(ctx, projectID, domain.ResourceCoordinationRequest, cursor, limit, actor.ID, participantType)
}

func (r *Repository) listResources(ctx context.Context, projectID string, kind domain.ResourceType, cursor string, limit int, participantID, participantType string) (app.Page, error) {
	cursorTime := time.Now().UTC().Add(time.Hour)
	cursorID := "\uffff"
	if cursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return app.Page{}, fmt.Errorf("invalid cursor: %w", err)
		}
		parts := strings.SplitN(string(decoded), "|", 2)
		if len(parts) != 2 {
			return app.Page{}, errors.New("invalid cursor")
		}
		cursorTime, err = time.Parse(time.RFC3339Nano, parts[0])
		if err != nil {
			return app.Page{}, errors.New("invalid cursor timestamp")
		}
		cursorID = parts[1]
	}
	rows, err := r.pool.Query(ctx, `SELECT document, updated_at, id
		FROM resources WHERE project_id=$1 AND kind=$2
		AND (updated_at,id) < ($3,$4)
		AND ($6='' OR (document->>'owner_id'=$6 AND document->>'owner_type'=$7)
		 OR (document->'recipient'->>'id'=$6 AND document->'recipient'->>'type'=$7))
		ORDER BY updated_at DESC,id DESC LIMIT $5`, projectID, kind, cursorTime, cursorID, limit+1, participantID, participantType)
	if err != nil {
		return app.Page{}, err
	}
	defer rows.Close()
	var page app.Page
	var lastTime time.Time
	var lastID string
	for rows.Next() {
		var raw []byte
		var updatedAt time.Time
		var id string
		if err := rows.Scan(&raw, &updatedAt, &id); err != nil {
			return app.Page{}, err
		}
		page.Items = append(page.Items, raw)
		if len(page.Items) <= limit {
			lastTime, lastID = updatedAt, id
		}
	}
	if err := rows.Err(); err != nil {
		return app.Page{}, err
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(lastTime.Format(time.RFC3339Nano) + "|" + lastID))
	}
	return page, nil
}

func (r *Repository) Execute(ctx context.Context, cmd domain.Command, build func(json.RawMessage) (app.Mutation, error)) (result app.CommandResult, retErr error) {
	defer func() {
		var pgError *pgconn.PgError
		if errors.As(retErr, &pgError) && pgError.Code == "40001" {
			retErr = app.ErrVersionConflict
		}
	}()
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return app.CommandResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	fingerprint := commandFingerprint(cmd)
	var storedFingerprint, storedResource, storedEvent []byte
	err = tx.QueryRow(ctx, `SELECT fingerprint,resource,event FROM command_results
		WHERE project_id=$1 AND idempotency_key=$2 FOR UPDATE`, cmd.ProjectID, cmd.IdempotencyKey).
		Scan(&storedFingerprint, &storedResource, &storedEvent)
	if err == nil {
		if !equalBytes(storedFingerprint, fingerprint[:]) {
			return app.CommandResult{}, app.ErrIdempotency
		}
		var event domain.Event
		if err := json.Unmarshal(storedEvent, &event); err != nil {
			return app.CommandResult{}, err
		}
		return app.CommandResult{Resource: storedResource, Event: event, Replayed: true}, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return app.CommandResult{}, err
	}
	if budgetID, exceeded, err := enforcedBudgetExceeded(ctx, tx, cmd); err != nil {
		return app.CommandResult{}, err
	} else if exceeded {
		if err := recordBudgetDenial(ctx, tx, cmd, budgetID); err != nil {
			return app.CommandResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return app.CommandResult{}, err
		}
		return app.CommandResult{}, app.ErrBudgetExceeded
	}
	if cmd.ApprovalID != "" {
		if err := consumeApprovalTx(ctx, tx, cmd); err != nil {
			return app.CommandResult{}, err
		}
	}
	var current []byte
	var currentVersion int64
	err = tx.QueryRow(ctx, `SELECT document,version FROM resources
		WHERE project_id=$1 AND kind=$2 AND id=$3 FOR UPDATE`, cmd.ProjectID, cmd.SubjectType, cmd.SubjectID).
		Scan(&current, &currentVersion)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return app.CommandResult{}, err
	}
	if cmd.ExpectedVersion > 0 && cmd.ExpectedVersion != currentVersion {
		return app.CommandResult{}, app.ErrVersionConflict
	}
	mutation, err := build(current)
	if err != nil {
		return app.CommandResult{}, err
	}
	var resourceBase struct {
		ID        string    `json:"id"`
		ProjectID string    `json:"project_id"`
		Version   int64     `json:"version"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	if err := json.Unmarshal(mutation.Resource, &resourceBase); err != nil {
		return app.CommandResult{}, fmt.Errorf("decode resource base: %w", err)
	}
	if resourceBase.ID != cmd.SubjectID || resourceBase.ProjectID != cmd.ProjectID {
		return app.CommandResult{}, errors.New("resource identity does not match command")
	}
	if currentVersion == 0 && resourceBase.Version != 1 {
		return app.CommandResult{}, errors.New("new resource must start at version 1")
	}
	if currentVersion > 0 && resourceBase.Version != currentVersion+1 {
		return app.CommandResult{}, errors.New("resource mutation must increment version exactly once")
	}
	_, err = tx.Exec(ctx, `INSERT INTO resources(project_id,kind,id,version,document,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT(project_id,kind,id) DO UPDATE SET version=excluded.version,document=excluded.document,updated_at=excluded.updated_at`,
		cmd.ProjectID, cmd.SubjectType, cmd.SubjectID, resourceBase.Version, mutation.Resource, resourceBase.CreatedAt, resourceBase.UpdatedAt)
	if err != nil {
		return app.CommandResult{}, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO events(id,project_id,event_type,subject_type,subject_id,actor_id,command_id,correlation_id,causation_id,occurred_at,schema_version,source_system,source_event_id,source_sequence,payload)
		VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),NULLIF($9,''),$10,$11,NULLIF($12,''),NULLIF($13,''),NULLIF($14::bigint,0),$15) RETURNING cursor`,
		mutation.Event.ID, mutation.Event.ProjectID, mutation.Event.Type, mutation.Event.SubjectType, mutation.Event.SubjectID,
		mutation.Event.ActorID, mutation.Event.CommandID, mutation.Event.CorrelationID, mutation.Event.CausationID,
		mutation.Event.OccurredAt, mutation.Event.SchemaVersion, mutation.Event.SourceSystem, mutation.Event.SourceEventID, mutation.Event.SourceSequence, mutation.Event.Payload).Scan(&mutation.Event.Cursor)
	if err != nil {
		return app.CommandResult{}, err
	}
	auditRaw, _ := json.Marshal(mutation.Audit)
	_, err = tx.Exec(ctx, `INSERT INTO audit_records(id,project_id,actor_id,action,resource_type,resource_id,command_id,outcome,remote_addr,details,occurred_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		mutation.Audit.ID, mutation.Audit.ProjectID, mutation.Audit.ActorID, mutation.Audit.Action, mutation.Audit.ResourceType,
		mutation.Audit.ResourceID, mutation.Audit.CommandID, mutation.Audit.Outcome, mutation.Audit.RemoteAddr, mutation.Audit.Details, mutation.Audit.CreatedAt)
	if err != nil {
		return app.CommandResult{}, err
	}
	if mutation.Control != nil {
		control := mutation.Control
		if control.ID == "" || control.ProjectID != cmd.ProjectID || control.RunID != cmd.SubjectID ||
			control.ActorID != cmd.ActorID || control.CommandID != cmd.ID {
			return app.CommandResult{}, errors.New("run control intent does not match command")
		}
		_, err = tx.Exec(ctx, `INSERT INTO run_control_outbox(
			id,project_id,run_id,actor_id,command_id,action,message,requested_at
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
			control.ID, control.ProjectID, control.RunID, control.ActorID, control.CommandID,
			control.Action, control.Message, control.RequestedAt)
		if err != nil {
			return app.CommandResult{}, err
		}
	}
	eventRaw, _ := json.Marshal(mutation.Event)
	_, err = tx.Exec(ctx, `INSERT INTO event_outbox(event_id,cursor,project_id,event)
		VALUES($1,$2,$3,$4)`, mutation.Event.ID, mutation.Event.Cursor, mutation.Event.ProjectID, eventRaw)
	if err != nil {
		return app.CommandResult{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO command_results(project_id,idempotency_key,fingerprint,command_id,resource,event,accepted_at)
		VALUES($1,$2,$3,$4,$5,$6,$7)`, cmd.ProjectID, cmd.IdempotencyKey, fingerprint[:], cmd.ID, mutation.Resource, eventRaw, cmd.RequestedAt)
	if err != nil {
		return app.CommandResult{}, err
	}
	// Audit records are also exposed through the common projection API.
	_, err = tx.Exec(ctx, `INSERT INTO resources(project_id,kind,id,version,document,created_at,updated_at)
		VALUES($1,$2,$3,1,$4,$5,$5)`, cmd.ProjectID, domain.ResourceAudit, mutation.Audit.ID, auditRaw, mutation.Audit.CreatedAt)
	if err != nil {
		return app.CommandResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return app.CommandResult{}, err
	}
	return app.CommandResult{Resource: mutation.Resource, Event: mutation.Event}, nil
}

func enforcedBudgetExceeded(ctx context.Context, tx pgx.Tx, cmd domain.Command) (string, bool, error) {
	if cmd.Type != "transition.task" && cmd.Type != "create.run" && !strings.HasSuffix(cmd.Type, ".run") {
		return "", false, nil
	}
	var budgetID string
	err := tx.QueryRow(ctx, `SELECT id FROM resources
		WHERE project_id=$1 AND kind=$2
		AND document->>'enforcement_mode'='enforced'
		AND document->>'status'='exhausted'
		AND (
			(document->>'scope_type'='project' AND document->>'scope_id'=$1)
			OR (document->>'scope_type'=$3 AND document->>'scope_id'=$4)
		)
		ORDER BY id LIMIT 1 FOR UPDATE`,
		cmd.ProjectID, domain.ResourceBudget, string(cmd.SubjectType), cmd.SubjectID).Scan(&budgetID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return budgetID, err == nil, err
}

func recordBudgetDenial(ctx context.Context, tx pgx.Tx, cmd domain.Command, budgetID string) error {
	now := cmd.RequestedAt
	details, _ := json.Marshal(map[string]any{"budget_id": budgetID, "command_type": cmd.Type})
	auditID := cmd.ID + ":budget-denied"
	if _, err := tx.Exec(ctx, `INSERT INTO audit_records(
		id,project_id,actor_id,action,resource_type,resource_id,command_id,outcome,remote_addr,details,occurred_at
	) VALUES($1,$2,$3,$4,$5,$6,$7,'denied',$8,$9,$10) ON CONFLICT(id) DO NOTHING`,
		auditID, cmd.ProjectID, cmd.ActorID, cmd.Type, cmd.SubjectType, cmd.SubjectID,
		cmd.ID, cmd.RemoteAddr, details, now); err != nil {
		return err
	}
	audit := domain.AuditRecord{
		Base: domain.NewBase(auditID, cmd.ProjectID, now), ActorID: cmd.ActorID, Action: cmd.Type,
		ResourceType: string(cmd.SubjectType), ResourceID: cmd.SubjectID, CommandID: cmd.ID,
		Outcome: "denied", RemoteAddr: cmd.RemoteAddr, Details: details,
	}
	auditRaw, _ := json.Marshal(audit)
	if _, err := tx.Exec(ctx, `INSERT INTO resources(project_id,kind,id,version,document,created_at,updated_at)
		VALUES($1,$2,$3,1,$4,$5,$5) ON CONFLICT(project_id,kind,id) DO NOTHING`,
		cmd.ProjectID, domain.ResourceAudit, auditID, auditRaw, now); err != nil {
		return err
	}
	attentionID := "attn_budget_" + stableID(cmd.ProjectID, budgetID)
	attention := domain.Attention{
		Base: domain.NewBase(attentionID, cmd.ProjectID, now), Kind: "budget.exhausted", Severity: "high",
		Title: "Budget exhausted; command denied", Detail: "An enforced budget blocked " + cmd.Type,
		ResourceType: string(domain.ResourceBudget), ResourceID: budgetID, Status: "open",
	}
	raw, err := resourceForUpdate(ctx, tx, cmd.ProjectID, domain.ResourceAttention, attentionID)
	if err != nil {
		return err
	}
	if len(raw) > 0 {
		var prior domain.Attention
		if err := json.Unmarshal(raw, &prior); err != nil {
			return err
		}
		attention.Base = prior.Base
		attention.Version++
		attention.UpdatedAt = now
	}
	return upsertProjection(ctx, tx, domain.ResourceAttention, attention.Base, attention)
}

func (r *Repository) Ingest(ctx context.Context, events []domain.Event) error {
	if len(events) == 0 {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, event := range events {
		if event.SourceSystem == "" || event.SourceEventID == "" {
			return app.Invalid("ingested events require source_system and source_event_id")
		}
		var existingType, existingSubject string
		var existingPayload []byte
		err := tx.QueryRow(ctx, `SELECT event_type,subject_id,payload FROM events
			WHERE project_id=$1 AND source_system=$2 AND source_event_id=$3`,
			event.ProjectID, event.SourceSystem, event.SourceEventID).Scan(&existingType, &existingSubject, &existingPayload)
		if err == nil {
			if existingType != event.Type || existingSubject != event.SubjectID || !jsonEqual(existingPayload, event.Payload) {
				return app.ErrIdempotency
			}
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var cursor int64
		err = tx.QueryRow(ctx, `INSERT INTO events(id,project_id,event_type,subject_type,subject_id,actor_id,command_id,correlation_id,causation_id,occurred_at,schema_version,source_system,source_event_id,source_sequence,payload)
			VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),$10,$11,$12,$13,NULLIF($14::bigint,0),$15)
			RETURNING cursor`, event.ID, event.ProjectID, event.Type, event.SubjectType, event.SubjectID, event.ActorID, event.CommandID, event.CorrelationID, event.CausationID, event.OccurredAt, event.SchemaVersion, event.SourceSystem, event.SourceEventID, event.SourceSequence, event.Payload).Scan(&cursor)
		if err != nil {
			return err
		}
		event.Cursor = cursor
		if err := applyEventProjection(ctx, tx, event); err != nil {
			return err
		}
		raw, _ := json.Marshal(event)
		if _, err := tx.Exec(ctx, `INSERT INTO event_outbox(event_id,cursor,project_id,event)
			VALUES($1,$2,$3,$4) ON CONFLICT(event_id) DO NOTHING`, event.ID, cursor, event.ProjectID, raw); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func jsonEqual(left, right []byte) bool {
	var a, b any
	if json.Unmarshal(left, &a) != nil || json.Unmarshal(right, &b) != nil {
		return false
	}
	normalizedA, _ := json.Marshal(a)
	normalizedB, _ := json.Marshal(b)
	return equalBytes(normalizedA, normalizedB)
}

func (r *Repository) Events(ctx context.Context, projectID string, after int64, limit int) (app.EventPage, error) {
	rows, err := r.pool.Query(ctx, `SELECT cursor,id,project_id,event_type,subject_type,subject_id,actor_id,
		COALESCE(command_id,''),COALESCE(correlation_id,''),COALESCE(causation_id,''),occurred_at,schema_version,
		COALESCE(source_system,''),COALESCE(source_event_id,''),COALESCE(source_sequence,0),payload
		FROM events WHERE project_id=$1 AND cursor>$2 ORDER BY cursor LIMIT $3`, projectID, after, limit+1)
	if err != nil {
		return app.EventPage{}, err
	}
	defer rows.Close()
	var page app.EventPage
	for rows.Next() {
		var event domain.Event
		if err := rows.Scan(&event.Cursor, &event.ID, &event.ProjectID, &event.Type, &event.SubjectType, &event.SubjectID,
			&event.ActorID, &event.CommandID, &event.CorrelationID, &event.CausationID, &event.OccurredAt, &event.SchemaVersion,
			&event.SourceSystem, &event.SourceEventID, &event.SourceSequence, &event.Payload); err != nil {
			return app.EventPage{}, err
		}
		page.Items = append(page.Items, event)
	}
	if err := rows.Err(); err != nil {
		return app.EventPage{}, err
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.NextCursor = page.Items[len(page.Items)-1].Cursor
	}
	return page, nil
}

// Workspace reads the projection, event watermark, and source freshness from
// one repeatable-read snapshot, so pagination and concurrent ingestion cannot
// produce a graph assembled from different versions of the project.
func (r *Repository) Workspace(ctx context.Context, projectID string) (app.WorkspaceSnapshot, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return app.WorkspaceSnapshot{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	value := app.WorkspaceSnapshot{
		ProjectID: projectID, Sources: []app.WorkspaceSource{},
		Resources: app.WorkspaceResources{
			Tasks: []json.RawMessage{}, Agents: []json.RawMessage{}, Runs: []json.RawMessage{},
			Attention: []json.RawMessage{}, Evidence: []json.RawMessage{}, Artifacts: []json.RawMessage{},
			Claims: []json.RawMessage{}, Approvals: []json.RawMessage{}, Requests: []json.RawMessage{},
		},
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM projects WHERE id=$1),
		COALESCE((SELECT max(cursor) FROM events WHERE project_id=$1),0), transaction_timestamp()`,
		projectID).Scan(&exists, &value.Cursor, &value.GeneratedAt); err != nil {
		return app.WorkspaceSnapshot{}, err
	}
	if !exists {
		return app.WorkspaceSnapshot{}, app.ErrNotFound
	}
	collections := map[string]*[]json.RawMessage{
		"task": &value.Resources.Tasks, "agent": &value.Resources.Agents, "run": &value.Resources.Runs,
		"attention": &value.Resources.Attention, "evidence": &value.Resources.Evidence,
		"artifact": &value.Resources.Artifacts, "claim": &value.Resources.Claims, "approval": &value.Resources.Approvals, "coordination_request": &value.Resources.Requests,
	}
	rows, err := tx.Query(ctx, `SELECT kind,document FROM resources
		WHERE project_id=$1 AND kind=ANY($2::text[]) ORDER BY kind,id`, projectID,
		[]string{"task", "agent", "run", "attention", "evidence", "artifact", "claim", "approval", "coordination_request"})
	if err != nil {
		return app.WorkspaceSnapshot{}, err
	}
	for rows.Next() {
		var kind string
		var raw json.RawMessage
		if err := rows.Scan(&kind, &raw); err != nil {
			rows.Close()
			return app.WorkspaceSnapshot{}, err
		}
		collection := collections[kind]
		*collection = append(*collection, raw)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return app.WorkspaceSnapshot{}, err
	}
	value.Sources, err = projectSources(ctx, tx, projectID)
	if err != nil {
		return app.WorkspaceSnapshot{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return app.WorkspaceSnapshot{}, err
	}
	return value, nil
}

func projectSources(ctx context.Context, tx pgx.Tx, projectID string) ([]app.WorkspaceSource, error) {
	rows, err := tx.Query(ctx, `SELECT e.source_system,max(o.available_at),max(e.occurred_at),
		(p.document->>'source_last_successful_collection_at')::timestamptz,
		p.document->'source_observation'
		FROM events e JOIN event_outbox o ON o.event_id=e.id
		LEFT JOIN resources p ON p.project_id=e.project_id AND p.kind='project' AND p.id=e.project_id AND p.source_system=e.source_system
		WHERE e.project_id=$1 AND e.source_system IS NOT NULL
		GROUP BY e.source_system,p.document ORDER BY e.source_system`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sources := []app.WorkspaceSource{}
	for rows.Next() {
		var source app.WorkspaceSource
		var observation json.RawMessage
		if err := rows.Scan(&source.System, &source.LastReceivedAt, &source.LastCollectedAt, &source.LastSuccessfulCollectionAt, &observation); err != nil {
			return nil, err
		}
		source.LastSeen = source.LastReceivedAt
		if len(observation) > 0 && string(observation) != "null" {
			var project domain.ProjectObservation
			if err := json.Unmarshal(observation, &project); err != nil {
				return nil, err
			}
			source.ProjectObservation = &project
			source.LastCollectedAt = &project.CollectedAt
			source.ScanStatus = &project.ScanStatus
		}
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

func (r *Repository) Overview(ctx context.Context, projectID string) (map[string]any, error) {
	return r.OverviewForActor(ctx, projectID, app.Actor{Capabilities: map[string]struct{}{"resource:read": {}}})
}

func (r *Repository) OverviewForActor(ctx context.Context, projectID string, actor app.Actor) (map[string]any, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `SELECT kind,count(*) FROM resources WHERE project_id=$1 GROUP BY kind`, projectID)
	if err != nil {
		return nil, err
	}
	counts := map[string]int64{}
	for rows.Next() {
		var kind string
		var count int64
		if err := rows.Scan(&kind, &count); err != nil {
			rows.Close()
			return nil, err
		}
		counts[kind] = count
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var cursor int64
	var generatedAt time.Time
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(cursor),0),transaction_timestamp() FROM events WHERE project_id=$1`, projectID).Scan(&cursor, &generatedAt); err != nil {
		return nil, err
	}
	sources, err := projectSources(ctx, tx, projectID)
	if err != nil {
		return nil, err
	}
	documents := map[domain.ResourceType][]json.RawMessage{}
	rows, err = tx.Query(ctx, `SELECT kind,document FROM resources WHERE project_id=$1 AND kind=ANY($2::text[]) ORDER BY kind,id`, projectID, []string{"task", "agent", "run", "coordination_request", "claim", "approval", "attention"})
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var kind domain.ResourceType
		var raw json.RawMessage
		if err := rows.Scan(&kind, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		documents[kind] = append(documents[kind], raw)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	coordination, err := summarizeCoordination(documents, actor, generatedAt)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, raw := range documents[domain.ResourceAgent] {
		var agent domain.Agent
		if json.Unmarshal(raw, &agent) == nil {
			names[agent.ID] = agent.DisplayName
		}
	}
	ids := []string{}
	for _, item := range coordination.Items {
		if item.OwnerID != "" {
			ids = append(ids, item.OwnerID)
		}
		if item.Recipient != nil {
			ids = append(ids, item.Recipient.ID)
		}
	}
	if len(ids) > 0 {
		people, err := tx.Query(ctx, `SELECT id,display_name FROM user_accounts WHERE id=ANY($1::text[]) AND NOT disabled`, ids)
		if err != nil {
			return nil, err
		}
		for people.Next() {
			var id, name string
			if err := people.Scan(&id, &name); err != nil {
				people.Close()
				return nil, err
			}
			names[id] = name
		}
		people.Close()
		if err := people.Err(); err != nil {
			return nil, err
		}
	}
	for i := range coordination.Items {
		item := &coordination.Items[i]
		item.OwnerName = boundedCoordinationText(names[item.OwnerID], 256)
		if item.Recipient != nil {
			item.RecipientName = boundedCoordinationText(names[item.Recipient.ID], 256)
		}
	}
	if !actor.Can("resource:read") {
		delete(counts, "coordination_request")
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return map[string]any{"project_id": projectID, "counts": counts, "event_cursor": cursor, "generated_at": generatedAt, "sources": sources, "coordination": coordination}, nil
}

// Recipient registration is checked against canonical project membership or
// persistent agent records; source activity alone never provisions a participant.
func (r *Repository) CoordinationRecipientExists(ctx context.Context, projectID string, recipient domain.CoordinationRecipient) (bool, error) {
	var exists bool
	if recipient.Type == "human" {
		err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_accounts u WHERE u.id=$2 AND NOT u.disabled
		 AND (EXISTS(SELECT 1 FROM project_memberships m WHERE m.project_id=$1 AND m.user_id=u.id) OR 'project:all'=ANY(u.capabilities)))`, projectID, recipient.ID).Scan(&exists)
		return exists, err
	}
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM resources WHERE project_id=$1 AND kind='agent' AND id=$2)`, projectID, recipient.ID).Scan(&exists)
	return exists, err
}

func boundedCoordinationText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func summarizeCoordination(documents map[domain.ResourceType][]json.RawMessage, actor app.Actor, now time.Time) (app.CoordinationSummary, error) {
	result := app.CoordinationSummary{Items: []app.CoordinationItem{}, Complete: true}
	add := func(item app.CoordinationItem) {
		item.Title = boundedCoordinationText(item.Title, 256)
		item.Detail = boundedCoordinationText(item.Detail, 2048)
		result.Items = append(result.Items, item)
	}
	for _, kind := range []domain.ResourceType{domain.ResourceRun, domain.ResourceAgent, domain.ResourceTask, domain.ResourceCoordinationRequest, domain.ResourceClaim, domain.ResourceApproval, domain.ResourceAttention} {
		readable := actor.Can("resource:read") || actor.Can(string(kind)+":read") || (kind == domain.ResourceCoordinationRequest && actor.Can("coordination:read"))
		if !readable {
			result.Complete = false
			continue
		}
		if kind == domain.ResourceCoordinationRequest && !actor.Can("resource:read") {
			result.Complete = false
		}
		for _, raw := range documents[kind] {
			item := app.CoordinationItem{ResourceType: kind}
			var base domain.Base
			if err := json.Unmarshal(raw, &base); err != nil {
				return result, err
			}
			item.ID, item.UpdatedAt = base.ID, base.UpdatedAt
			switch kind {
			case domain.ResourceRun:
				var run domain.Run
				if err := json.Unmarshal(raw, &run); err != nil {
					return result, err
				}
				if run.Status != "running" {
					continue
				}
				result.Counts.RunningWork++
				item.Kind, item.Title, item.Status, item.OwnerID, item.TaskID, item.SourceSystem = "run", run.Summary, run.Status, run.AgentID, run.TaskID, run.Source.System
				if item.Title == "" {
					item.Title = "Recorded running work"
				}
			case domain.ResourceAgent:
				var agent domain.Agent
				if err := json.Unmarshal(raw, &agent); err != nil {
					return result, err
				}
				if agent.PresenceExpiresAt == nil || !agent.PresenceExpiresAt.After(now) || (agent.Status != "active" && agent.Status != "idle" && agent.Status != "blocked") {
					continue
				}
				result.Counts.AgentPresence++
				item.Kind, item.Title, item.Status, item.OwnerID, item.SourceSystem = "agent", agent.DisplayName, agent.Status, agent.ID, agent.Source.System
			case domain.ResourceTask:
				var task domain.Task
				if err := json.Unmarshal(raw, &task); err != nil {
					return result, err
				}
				canonicalBlocked, sourceBlocked := task.Status == "blocked", strings.EqualFold(strings.TrimSpace(task.SourceStatus), "blocked")
				if task.Source.System == "sentry" {
					switch strings.ToLower(strings.TrimSpace(task.SourceStatus)) {
					case "upcoming", "working", "blocked":
						result.Counts.SourceUnfinishedTasks++
					}
				}
				unfinished := task.Status != "completed" && task.Status != "archived" && task.Status != "cancelled"
				// An unclaimed imported inbox card is source context, not an active worker.
				if task.Status == "inbox" && task.OwnerID == "" && task.Source.System != "" {
					unfinished = false
				}
				if unfinished {
					result.Counts.UnfinishedTasks++
				}
				if canonicalBlocked {
					result.Counts.CanonicalBlockedTasks++
				}
				if sourceBlocked {
					result.Counts.SourceBlockedTasks++
				}
				if canonicalBlocked || sourceBlocked {
					result.Counts.BlockedTasks++
				}
				item.Kind = "task"
				if canonicalBlocked || sourceBlocked {
					item.Kind = "blocker"
					item.Detail = task.BlockedReason
					if sourceBlocked && !canonicalBlocked && task.SourceObservation != nil {
						item.Detail = task.SourceObservation.Description
					}
				} else if task.Status == "review" {
					item.Kind = "review"
					result.Counts.Reviews++
				} else if !unfinished {
					continue
				}
				item.Title, item.Status, item.OwnerID, item.TaskID, item.SourceSystem = task.Title, task.Status, task.OwnerID, task.ID, task.Source.System
				if sourceBlocked && !canonicalBlocked {
					item.Title = "Source reports blocked: " + task.Title
					item.Status = "source_blocked"
				}
			case domain.ResourceCoordinationRequest:
				if !app.CoordinationVisible(actor, raw) {
					continue
				}
				var request domain.CoordinationRequest
				if err := json.Unmarshal(raw, &request); err != nil {
					return result, err
				}
				if request.Status != "open" && request.Status != "responded" {
					continue
				}
				result.Counts.Requests++
				if request.Kind == "review" {
					result.Counts.Reviews++
				}
				item.Kind, item.Title, item.Status, item.OwnerID, item.TaskID, item.EvidenceIDs = "request", request.Title, request.Status, request.OwnerID, request.TaskID, request.EvidenceIDs
				item.Recipient = &request.Recipient
				item.RequestKind = request.Kind
				item.Detail = request.Body
				if request.EscalationReason != "" {
					item.Detail = request.EscalationReason
				}
				item.HumanEscalation = request.Recipient.Type == "human" && request.Status == "open"
				if item.HumanEscalation {
					result.Counts.HumanEscalations++
				}
			case domain.ResourceClaim:
				var claim domain.Claim
				if err := json.Unmarshal(raw, &claim); err != nil {
					return result, err
				}
				if claim.Status != "pending" {
					continue
				}
				result.Counts.Reviews++
				item.Kind, item.Title, item.Status, item.OwnerID, item.TaskID = "review", "Completion claim awaits review", claim.Status, claim.AgentID, claim.TaskID
			case domain.ResourceAttention:
				var attention domain.Attention
				if err := json.Unmarshal(raw, &attention); err != nil {
					return result, err
				}
				if attention.Status != "open" && attention.Status != "acknowledged" {
					continue
				}
				result.Counts.OpenAttention++
				duplicate := false
				if attention.Kind == "source.task.blocked" || attention.Kind == "task.blocked" {
					for _, prior := range result.Items {
						if prior.Kind == "blocker" && string(prior.ResourceType) == attention.ResourceType && prior.ID == attention.ResourceID {
							duplicate = true
							break
						}
					}
				}
				if duplicate {
					continue
				}
				item.Kind, item.Title, item.Detail, item.Status, item.OwnerID = "attention", attention.Title, attention.Detail, attention.Status, attention.OwnerID
				item.TargetType, item.TargetID = attention.ResourceType, attention.ResourceID
				if attention.ResourceType == "task" {
					item.TaskID = attention.ResourceID
				}
			case domain.ResourceApproval:
				var approval domain.Approval
				if err := json.Unmarshal(raw, &approval); err != nil {
					return result, err
				}
				if approval.Status != "pending" || approval.ExpiresAt.IsZero() || !approval.ExpiresAt.After(now) {
					continue
				}
				result.Counts.HumanApprovals++
				item.Kind, item.Title, item.Status, item.OwnerID, item.HumanEscalation = "human_approval", approval.Reason, approval.Status, approval.RequestedBy, true
			}
			add(item)
		}
	}
	priority := func(item app.CoordinationItem) int {
		if item.HumanEscalation {
			return 0
		}
		switch item.Kind {
		case "blocker", "attention":
			return 1
		case "request", "review":
			return 2
		case "run":
			return 3
		case "agent":
			return 4
		default:
			return 5
		}
	}
	sort.Slice(result.Items, func(i, j int) bool {
		a, b := result.Items[i], result.Items[j]
		if priority(a) != priority(b) {
			return priority(a) < priority(b)
		}
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.After(b.UpdatedAt)
		}
		if a.ResourceType != b.ResourceType {
			return a.ResourceType < b.ResourceType
		}
		return a.ID < b.ID
	})
	if len(result.Items) > 24 {
		result.Truncated = true
		result.Items = result.Items[:24]
	}
	return result, nil
}

func commandFingerprint(cmd domain.Command) [32]byte {
	return sha256.Sum256([]byte(strings.Join([]string{
		cmd.Type, string(cmd.SubjectType), cmd.SubjectID, strconv.FormatInt(cmd.ExpectedVersion, 10), cmd.ApprovalID, cmd.ApprovalDigest, string(cmd.Payload),
	}, "\x00")))
}

func consumeApprovalTx(ctx context.Context, tx pgx.Tx, cmd domain.Command) error {
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT document FROM resources WHERE project_id=$1 AND kind=$2 AND id=$3 FOR UPDATE`,
		cmd.ProjectID, domain.ResourceApproval, cmd.ApprovalID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	if err != nil {
		return err
	}
	var approval domain.Approval
	if err := json.Unmarshal(raw, &approval); err != nil {
		return err
	}
	switch {
	case approval.Status != "approved":
		return app.ErrDenied
	case approval.RequestedBy != cmd.ActorID:
		return app.ErrDenied
	case approval.CommandVersion != 1 || approval.CommandDigest != cmd.ApprovalDigest:
		return app.ErrDenied
	case approval.ExpectedTargetVersion != cmd.ExpectedVersion:
		return app.ErrDenied
	case approval.ExpiresAt.IsZero() || !cmd.RequestedAt.Before(approval.ExpiresAt):
		return app.ErrDenied
	case approval.ConsumedAt != nil:
		return app.ErrDenied
	}
	approval.ConsumedAt, approval.ConsumedBy = &cmd.RequestedAt, cmd.ActorID
	approval.Version++
	approval.UpdatedAt = cmd.RequestedAt
	updated, _ := json.Marshal(approval)
	if _, err := tx.Exec(ctx, `UPDATE resources SET version=$1,document=$2,updated_at=$3
		WHERE project_id=$4 AND kind=$5 AND id=$6`, approval.Version, updated, cmd.RequestedAt, cmd.ProjectID, domain.ResourceApproval, cmd.ApprovalID); err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{"resource": json.RawMessage(updated), "command_digest": cmd.ApprovalDigest})
	event := domain.Event{ID: cmd.ID + ":approval", ProjectID: cmd.ProjectID, Type: "approval.consumed", SubjectType: domain.ResourceApproval, SubjectID: cmd.ApprovalID, ActorID: cmd.ActorID, CommandID: cmd.ID, OccurredAt: cmd.RequestedAt, SchemaVersion: 1, Payload: payload}
	if err := tx.QueryRow(ctx, `INSERT INTO events(id,project_id,event_type,subject_type,subject_id,actor_id,command_id,occurred_at,schema_version,payload)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING cursor`,
		event.ID, event.ProjectID, event.Type, event.SubjectType, event.SubjectID, event.ActorID, event.CommandID, event.OccurredAt, event.SchemaVersion, event.Payload).Scan(&event.Cursor); err != nil {
		return err
	}
	eventRaw, _ := json.Marshal(event)
	if _, err := tx.Exec(ctx, `INSERT INTO event_outbox(event_id,cursor,project_id,event) VALUES($1,$2,$3,$4)`,
		event.ID, event.Cursor, event.ProjectID, eventRaw); err != nil {
		return err
	}
	auditID := cmd.ID + ":approval-audit"
	if _, err := tx.Exec(ctx, `INSERT INTO audit_records(id,project_id,actor_id,action,resource_type,resource_id,command_id,outcome,remote_addr,details,occurred_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, auditID, cmd.ProjectID, cmd.ActorID, "consume.approval", domain.ResourceApproval, cmd.ApprovalID, cmd.ID, "accepted", cmd.RemoteAddr, payload, cmd.RequestedAt); err != nil {
		return err
	}
	return nil
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var result byte
	for i := range a {
		result |= a[i] ^ b[i]
	}
	return result == 0
}
