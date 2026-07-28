package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/msitarzewski/agent-room/db"
)

type Migration struct {
	Version int64
	Name    string
	Digest  string
	SQL     string
}

func LoadMigrations() ([]Migration, error) {
	entries, err := fs.ReadDir(db.Migrations, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}
	var migrations []Migration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid migration name %q", entry.Name())
		}
		version, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid migration version %q: %w", parts[0], err)
		}
		content, err := fs.ReadFile(db.Migrations, path.Join("migrations", entry.Name()))
		if err != nil {
			return nil, err
		}
		up, _, _ := strings.Cut(string(content), "-- +goose Down")
		up = strings.TrimSpace(strings.TrimPrefix(up, "-- +goose Up"))
		digest := sha256.Sum256(content)
		migrations = append(migrations, Migration{Version: version, Name: entry.Name(), Digest: hex.EncodeToString(digest[:]), SQL: up})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	return migrations, nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	migrations, err := LoadMigrations()
	if err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version bigint PRIMARY KEY, name text NOT NULL, digest text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("bootstrap migration table: %w", err)
	}
	for _, migration := range migrations {
		var digest string
		err := pool.QueryRow(ctx, "SELECT digest FROM schema_migrations WHERE version=$1", migration.Version).Scan(&digest)
		if err == nil {
			if digest != migration.Digest {
				return fmt.Errorf("migration %d digest mismatch: database=%s binary=%s", migration.Version, digest, migration.Digest)
			}
			continue
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, migration.SQL); err == nil {
			_, err = tx.Exec(ctx, "INSERT INTO schema_migrations(version,name,digest) VALUES($1,$2,$3)", migration.Version, migration.Name, migration.Digest)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %d: %w", migration.Version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %d: %w", migration.Version, err)
		}
	}
	return nil
}

func MigrationStatus(ctx context.Context, pool *pgxpool.Pool) ([]map[string]any, error) {
	migrations, err := LoadMigrations()
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(migrations))
	for _, migration := range migrations {
		var digest string
		err := pool.QueryRow(ctx, "SELECT digest FROM schema_migrations WHERE version=$1", migration.Version).Scan(&digest)
		status := "pending"
		if err == nil {
			status = "applied"
			if digest != migration.Digest {
				status = "digest_mismatch"
			}
		}
		result = append(result, map[string]any{"version": migration.Version, "name": migration.Name, "status": status})
	}
	return result, nil
}
