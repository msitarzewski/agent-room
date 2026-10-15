package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
)

var capabilityPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*:[a-z][a-z0-9-]*$`)

func (r *Repository) Bootstrap(ctx context.Context, projectID, projectName, oidcSubject, email string, capabilities []string) error {
	if projectID == "" || projectName == "" || oidcSubject == "" || email == "" {
		return errors.New("project id/name and OIDC subject/email are required")
	}
	if err := validateCapabilities(capabilities); err != nil {
		return err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	userID := "oidc:" + oidcSubject
	if _, err := tx.Exec(ctx, `INSERT INTO projects(id,name) VALUES($1,$2)`, projectID, projectName); err != nil {
		return fmt.Errorf("create project: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO user_accounts(id,username,display_name,capabilities)
		VALUES($1,$2,$2,'{}') ON CONFLICT(id) DO UPDATE SET username=excluded.username,display_name=excluded.display_name,updated_at=now()`,
		userID, email); err != nil {
		return fmt.Errorf("create operator identity: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO project_memberships(project_id,user_id,capabilities) VALUES($1,$2,$3)`,
		projectID, userID, capabilities); err != nil {
		return fmt.Errorf("grant operator membership: %w", err)
	}
	return auditAdmin(ctx, tx, "bootstrap", "project", projectID, map[string]any{"user_id": userID, "capabilities": capabilities})
}

func (r *Repository) CreateProject(ctx context.Context, id, name string) error {
	if id == "" || name == "" {
		return errors.New("project id and name are required")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "INSERT INTO projects(id,name) VALUES($1,$2)", id, name); err != nil {
		return err
	}
	return auditAdmin(ctx, tx, "project.create", "project", id, map[string]string{"name": name})
}

// EnsureProject creates an operator-provisioned project if it is absent. Existing
// canonical names are preserved, and an unchanged retry creates no audit noise.
func (r *Repository) EnsureProject(ctx context.Context, id, name string) (bool, error) {
	if id == "" || name == "" {
		return false, errors.New("project id and name are required")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var inserted string
	err = tx.QueryRow(ctx, `INSERT INTO projects(id,name) VALUES($1,$2) ON CONFLICT(id) DO NOTHING RETURNING id`, id, name).Scan(&inserted)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, tx.Commit(ctx)
	}
	if err != nil {
		return false, err
	}
	if err := auditAdmin(ctx, tx, "project.ensure", "project", id, map[string]string{"name": name}); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) GrantMembership(ctx context.Context, projectID, oidcSubject, email string, capabilities []string) error {
	if err := validateCapabilities(capabilities); err != nil {
		return err
	}
	userID := "oidc:" + oidcSubject
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	identityChange, err := tx.Exec(ctx, `INSERT INTO user_accounts(id,username,display_name,capabilities)
		VALUES($1,$2,$2,'{}') ON CONFLICT(id) DO UPDATE SET username=excluded.username,updated_at=now()
		WHERE user_accounts.username IS DISTINCT FROM excluded.username`, userID, email)
	if err != nil {
		return err
	}
	var unchanged bool
	err = tx.QueryRow(ctx, `SELECT capabilities @> $3::text[] AND capabilities <@ $3::text[]
		FROM project_memberships WHERE project_id=$1 AND user_id=$2 FOR UPDATE`, projectID, userID, capabilities).Scan(&unchanged)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err == nil && unchanged && identityChange.RowsAffected() == 0 {
		return tx.Commit(ctx)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO project_memberships(project_id,user_id,capabilities) VALUES($1,$2,$3)
		ON CONFLICT(project_id,user_id) DO UPDATE SET capabilities=excluded.capabilities`, projectID, userID, capabilities); err != nil {
		return err
	}
	// Privilege changes rotate all sessions for the affected identity.
	if _, err := tx.Exec(ctx, "DELETE FROM web_sessions WHERE user_id=$1", userID); err != nil {
		return err
	}
	return auditAdmin(ctx, tx, "membership.grant", "project", projectID, map[string]any{"user_id": userID, "capabilities": capabilities})
}

func validateCapabilities(capabilities []string) error {
	if len(capabilities) == 0 {
		return errors.New("at least one capability is required")
	}
	seen := map[string]struct{}{}
	for _, capability := range capabilities {
		if !capabilityPattern.MatchString(capability) {
			return fmt.Errorf("invalid capability %q", capability)
		}
		if _, duplicate := seen[capability]; duplicate {
			return fmt.Errorf("duplicate capability %q", capability)
		}
		seen[capability] = struct{}{}
	}
	return nil
}

func auditAdmin(ctx context.Context, tx pgx.Tx, action, resourceType, resourceID string, details any) error {
	raw, _ := json.Marshal(details)
	now := time.Now().UTC()
	_, err := tx.Exec(ctx, `INSERT INTO audit_records(id,project_id,actor_id,action,resource_type,resource_id,outcome,details,occurred_at)
		VALUES($1,$2,$3,$4,$5,$6,'accepted',$7,$8)`,
		adminID(), projectForAudit(resourceType, resourceID), "local-operator", action, resourceType, resourceID, raw, now)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func projectForAudit(resourceType, resourceID string) string {
	if resourceType == "project" {
		return resourceID
	}
	return "_system"
}

func adminID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}
