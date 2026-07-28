package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/msitarzewski/agent-room/internal/app"
	"github.com/msitarzewski/agent-room/internal/domain"
)

type EventPublisher interface {
	Publish(domain.Event)
}

func (r *Repository) DispatchOutbox(ctx context.Context, publisher EventPublisher, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `SELECT event_id,event FROM event_outbox
		WHERE published_at IS NULL AND available_at<=now()
		ORDER BY cursor LIMIT $1 FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return 0, err
	}
	type queued struct {
		id    string
		event domain.Event
	}
	var batch []queued
	for rows.Next() {
		var item queued
		var raw []byte
		if err := rows.Scan(&item.id, &raw); err != nil {
			rows.Close()
			return 0, err
		}
		if err := json.Unmarshal(raw, &item.event); err != nil {
			rows.Close()
			return 0, fmt.Errorf("decode outbox event %s: %w", item.id, err)
		}
		batch = append(batch, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, item := range batch {
		publisher.Publish(item.event)
		if _, err := tx.Exec(ctx, `UPDATE event_outbox SET published_at=now(),attempts=attempts+1 WHERE event_id=$1`, item.id); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(batch), nil
}

func (r *Repository) RunOutbox(ctx context.Context, publisher EventPublisher, report func(error)) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for {
				count, err := r.DispatchOutbox(ctx, publisher, 100)
				if err != nil {
					if ctx.Err() == nil {
						report(err)
					}
					break
				}
				if count < 100 {
					break
				}
			}
		}
	}
}

type claimedControl struct {
	ID, ProjectID, RunID, ActorID, CommandID, Action, Message string
	RequestedAt                                               time.Time
}

// DispatchControlOutbox claims committed runtime-control intents before
// invoking the external process boundary. An accepted command is therefore
// durable even if the process action or the outcome write fails.
func (r *Repository) DispatchControlOutbox(ctx context.Context, controller app.RunController) (bool, error) {
	item, ok, err := r.claimControl(ctx)
	if err != nil || !ok {
		return ok, err
	}
	executeErr := controller.Execute(ctx, item.RunID, item.Action, item.Message)
	if err := r.recordControlOutcome(ctx, item, executeErr); err != nil {
		return true, err
	}
	return true, nil
}

func (r *Repository) claimControl(ctx context.Context) (claimedControl, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return claimedControl{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var item claimedControl
	err = tx.QueryRow(ctx, `SELECT id,project_id,run_id,actor_id,command_id,action,message,requested_at
		FROM run_control_outbox
		WHERE status='pending'
		ORDER BY requested_at LIMIT 1 FOR UPDATE SKIP LOCKED`).
		Scan(&item.ID, &item.ProjectID, &item.RunID, &item.ActorID, &item.CommandID, &item.Action, &item.Message, &item.RequestedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return claimedControl{}, false, nil
	}
	if err != nil {
		return claimedControl{}, false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE run_control_outbox
		SET status='executing',attempts=attempts+1,claimed_at=now(),last_error=NULL WHERE id=$1`, item.ID); err != nil {
		return claimedControl{}, false, err
	}
	return item, true, tx.Commit(ctx)
}

func (r *Repository) recordControlOutcome(ctx context.Context, item claimedControl, executeErr error) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var current []byte
	if err := tx.QueryRow(ctx, `SELECT document FROM resources
		WHERE project_id=$1 AND kind=$2 AND id=$3 FOR UPDATE`, item.ProjectID, domain.ResourceRun, item.RunID).Scan(&current); err != nil {
		return err
	}
	var run domain.Run
	if err := json.Unmarshal(current, &run); err != nil {
		return err
	}
	now := time.Now().UTC()
	outcome, eventType, detail := "executed", "run.control_executed", ""
	if executeErr != nil {
		outcome, eventType, detail = "failed", "run.control_failed", executeErr.Error()
		run.ControlStatus = "failed"
		run.ControlError = detail
	} else {
		run.ControlStatus = "executed"
		run.ControlError = ""
		switch item.Action {
		case "pause":
			run.Status = "paused"
		case "resume":
			run.Status = "running"
		case "cancel":
			run.Status = "cancelled"
		}
	}
	run.Version++
	run.UpdatedAt = now
	resource, _ := json.Marshal(run)
	if _, err := tx.Exec(ctx, `UPDATE resources SET version=$1,document=$2,updated_at=$3
		WHERE project_id=$4 AND kind=$5 AND id=$6`,
		run.Version, resource, now, item.ProjectID, domain.ResourceRun, item.RunID); err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{
		"resource": json.RawMessage(resource), "action": item.Action, "message": item.Message,
		"outcome": outcome, "error": detail,
	})
	event := domain.Event{
		ID: item.ID + ":outcome", ProjectID: item.ProjectID, Type: eventType,
		SubjectType: domain.ResourceRun, SubjectID: item.RunID, ActorID: item.ActorID,
		CommandID: item.CommandID, OccurredAt: now, SchemaVersion: 1, Payload: payload,
	}
	if err := tx.QueryRow(ctx, `INSERT INTO events(
		id,project_id,event_type,subject_type,subject_id,actor_id,command_id,occurred_at,schema_version,payload
	) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING cursor`,
		event.ID, event.ProjectID, event.Type, event.SubjectType, event.SubjectID, event.ActorID,
		event.CommandID, event.OccurredAt, event.SchemaVersion, event.Payload).Scan(&event.Cursor); err != nil {
		return err
	}
	eventRaw, _ := json.Marshal(event)
	if _, err := tx.Exec(ctx, `INSERT INTO event_outbox(event_id,cursor,project_id,event)
		VALUES($1,$2,$3,$4)`, event.ID, event.Cursor, event.ProjectID, eventRaw); err != nil {
		return err
	}
	auditID := item.ID + ":outcome-audit"
	if _, err := tx.Exec(ctx, `INSERT INTO audit_records(
		id,project_id,actor_id,action,resource_type,resource_id,command_id,outcome,details,occurred_at
	) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		auditID, item.ProjectID, item.ActorID, "execute."+item.Action+".run", domain.ResourceRun,
		item.RunID, item.CommandID, outcome, payload, now); err != nil {
		return err
	}
	audit := domain.AuditRecord{
		Base: domain.NewBase(auditID, item.ProjectID, now), ActorID: item.ActorID,
		Action: "execute." + item.Action + ".run", ResourceType: string(domain.ResourceRun),
		ResourceID: item.RunID, CommandID: item.CommandID, Outcome: outcome, Details: payload,
	}
	auditRaw, _ := json.Marshal(audit)
	if _, err := tx.Exec(ctx, `INSERT INTO resources(project_id,kind,id,version,document,created_at,updated_at)
		VALUES($1,$2,$3,1,$4,$5,$5)`,
		item.ProjectID, domain.ResourceAudit, auditID, auditRaw, now); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE run_control_outbox
		SET status=$1,completed_at=$2,last_error=NULLIF($3,'') WHERE id=$4 AND status='executing'`,
		outcome, now, detail, item.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) RunControlOutbox(ctx context.Context, controller app.RunController, report func(error)) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for {
				found, err := r.DispatchControlOutbox(ctx, controller)
				if err != nil {
					if ctx.Err() == nil {
						report(err)
					}
					break
				}
				if !found {
					break
				}
			}
		}
	}
}

// ReconcileControlOutbox closes intents left in "executing" by a previous
// daemon. It deliberately does not replay an external side effect whose
// outcome is unknown; this preserves the no-duplicate-execution invariant.
func (r *Repository) ReconcileControlOutbox(ctx context.Context) error {
	rows, err := r.pool.Query(ctx, `SELECT id,project_id,run_id,actor_id,command_id,action,message,requested_at
		FROM run_control_outbox WHERE status='executing' ORDER BY requested_at`)
	if err != nil {
		return err
	}
	var items []claimedControl
	for rows.Next() {
		var item claimedControl
		if err := rows.Scan(&item.ID, &item.ProjectID, &item.RunID, &item.ActorID, &item.CommandID,
			&item.Action, &item.Message, &item.RequestedAt); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range items {
		if err := r.recordControlOutcome(ctx, item, errors.New("runtime control outcome unknown after daemon restart; action was not retried")); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) RebuildProjections(ctx context.Context) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(671327001)"); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM resources WHERE kind<>$1", domain.ResourceAudit); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT cursor,id,project_id,event_type,subject_type,subject_id,actor_id,
		COALESCE(command_id,''),COALESCE(correlation_id,''),COALESCE(causation_id,''),occurred_at,schema_version,
		COALESCE(source_system,''),COALESCE(source_event_id,''),COALESCE(source_sequence,0),payload
		FROM events ORDER BY cursor`)
	if err != nil {
		return err
	}
	var events []domain.Event
	for rows.Next() {
		var event domain.Event
		if err := rows.Scan(&event.Cursor, &event.ID, &event.ProjectID, &event.Type, &event.SubjectType, &event.SubjectID,
			&event.ActorID, &event.CommandID, &event.CorrelationID, &event.CausationID, &event.OccurredAt,
			&event.SchemaVersion, &event.SourceSystem, &event.SourceEventID, &event.SourceSequence, &event.Payload); err != nil {
			rows.Close()
			return err
		}
		events = append(events, event)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, event := range events {
		if err := applyEventProjection(ctx, tx, event); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
