package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/msitarzewski/agent-room/internal/adapters"
	"github.com/msitarzewski/agent-room/internal/auth"
	"github.com/msitarzewski/agent-room/internal/config"
	"github.com/msitarzewski/agent-room/internal/postgres"
	"github.com/msitarzewski/agent-room/internal/version"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "agentroomctl:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) > 1 && os.Args[1] == "report" {
		return report(os.Args[2:])
	}
	for _, arg := range os.Args[1:] {
		if arg == "version" {
			fmt.Printf("agentroomctl %s commit=%s built=%s\n", version.Version, version.Commit, version.BuildTime)
			return nil
		}
	}
	cfg, command, err := config.Load(os.Args[1:])
	if err != nil {
		return err
	}
	if len(command) == 0 {
		return errors.New("command required: report, migrate, bootstrap, project, membership, service-token, projection, doctor, version")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	repo, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer repo.Close()
	switch command[0] {
	case "migrate":
		if len(command) != 2 {
			return errors.New("usage: agentroomctl migrate up|status|verify")
		}
		switch command[1] {
		case "up":
			return postgres.Migrate(ctx, repo.Pool())
		case "status":
			status, err := postgres.MigrationStatus(ctx, repo.Pool())
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(status)
		case "verify":
			status, err := postgres.MigrationStatus(ctx, repo.Pool())
			if err != nil {
				return err
			}
			for _, item := range status {
				if item["status"] != "applied" {
					return fmt.Errorf("migration %v is %v", item["version"], item["status"])
				}
			}
			fmt.Println("schema verified")
			return nil
		default:
			return errors.New("usage: agentroomctl migrate up|status|verify")
		}
	case "doctor":
		if err := repo.Health(ctx); err != nil {
			return err
		}
		status, err := postgres.MigrationStatus(ctx, repo.Pool())
		if err != nil {
			return err
		}
		fmt.Printf("database: ok\nmigrations: %d known\nartifact_dir: %s\n", len(status), cfg.ArtifactDir)
		return nil
	case "projection":
		if len(command) != 2 || command[1] != "rebuild" {
			return errors.New("usage: agentroomctl projection rebuild")
		}
		if err := repo.RebuildProjections(ctx); err != nil {
			return err
		}
		fmt.Println("projections rebuilt")
		return nil
	case "bootstrap":
		if len(command) != 5 {
			return errors.New("usage: agentroomctl bootstrap <project-id> <project-name> <oidc-subject> <email>")
		}
		capabilities := []string{"resource:read", "resource:write", "event:read", "event:ingest", "overview:read", "task:transition", "attention:manage", "approval:request", "approval:decide", "claim:review", "run:pause", "run:resume", "run:cancel", "run:message", "run:redirect"}
		return repo.Bootstrap(ctx, command[1], command[2], command[3], command[4], capabilities)
	case "project":
		if len(command) != 4 || (command[1] != "create" && command[1] != "ensure") {
			return errors.New("usage: agentroomctl project create|ensure <project-id> <project-name>")
		}
		if command[1] == "ensure" {
			created, err := repo.EnsureProject(ctx, command[2], command[3])
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"project_id": command[2], "created": created})
		}
		return repo.CreateProject(ctx, command[2], command[3])
	case "membership":
		if len(command) != 6 || command[1] != "grant" {
			return errors.New("usage: agentroomctl membership grant <project-id> <oidc-subject> <email> <capability,...>")
		}
		return repo.GrantMembership(ctx, command[2], command[3], command[4], splitCapabilities(command[5]))
	case "service-token":
		switch {
		case len(command) == 2 && command[1] == "list":
			items, err := auth.ListServiceTokens(ctx, repo.Pool())
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"items": items})
		case len(command) == 3 && command[1] == "revoke":
			replayed, err := auth.RevokeServiceToken(ctx, repo.Pool(), command[2])
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{"id": command[2], "revoked": true, "replayed": replayed})
		case (len(command) == 7 || len(command) == 8) && command[1] == "create":
			var expiry *time.Time
			if len(command) == 8 {
				days, err := strconv.Atoi(command[7])
				if err != nil || days < 1 || days > 90 {
					return errors.New("expiry-days must be an integer from 1 to 90")
				}
				value := time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour)
				expiry = &value
			}
			created, err := auth.CreateServiceTokenWithMetadata(ctx, repo.Pool(), command[2], command[3], command[4], splitCapabilities(command[5]), splitCapabilities(command[6]), expiry)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(created)
		default:
			return errors.New("usage: agentroomctl service-token create <id> <name> <actor-id> <project-id[,project-id...]> <capability,...> [expiry-days] | list | revoke <id>")
		}
	default:
		return fmt.Errorf("unknown command %q", command[0])
	}
}

func report(args []string) error {
	var options adapters.ReportOptions
	flags := flag.NewFlagSet("report", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&options.ProjectID, "project-id", "", "authorized project ID")
	flags.StringVar(&options.Runtime, "runtime", "codex", "codex, claude, or hermes")
	flags.StringVar(&options.AgentID, "agent-id", "", "authenticated token actor ID")
	flags.StringVar(&options.InstanceID, "instance-id", "", "stable reporter instance ID")
	flags.StringVar(&options.DisplayName, "display-name", "", "agent display name")
	flags.StringVar(&options.SessionID, "session-id", "", "actual native session ID; Codex defaults to CODEX_THREAD_ID")
	flags.StringVar(&options.AgentStatus, "status", "active", "active, idle, blocked, or disconnected")
	flags.StringVar(&options.SessionStatus, "session-status", "", "active, idle, or closed")
	flags.StringVar(&options.RunID, "run-id", "", "optional explicit native run ID")
	flags.StringVar(&options.RunStatus, "run-status", "", "running, idle, blocked, completed, failed, or cancelled")
	flags.StringVar(&options.TaskID, "task-id", "", "canonical task ID for the explicitly identified run")
	flags.StringVar(&options.Summary, "summary", "", "optional bounded run metadata; never a transcript")
	flags.StringVar(&options.StateDir, "state-dir", "", "absolute owner-only durable state directory")
	flags.StringVar(&options.TokenFile, "token-file", "", "owner-only bearer token file")
	flags.StringVar(&options.AdapterURL, "adapter-url", "", "private HTTPS or loopback HTTP adapter endpoint")
	flags.DurationVar(&options.Timeout, "timeout", 5*time.Second, "HTTP timeout from 1s to 30s")
	flags.BoolVar(&options.RetryOnly, "retry", false, "deliver only previously queued observations")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			flags.SetOutput(os.Stdout)
			flags.PrintDefaults()
			return nil
		}
		return errors.New("invalid report arguments; use agentroomctl report --help")
	}
	if flags.NArg() != 0 {
		return errors.New("report accepts named metadata flags only")
	}
	result, err := adapters.Report(options)
	if encodeErr := json.NewEncoder(os.Stdout).Encode(result); encodeErr != nil {
		return errors.New("report result cannot be written")
	}
	return err
}

func splitCapabilities(value string) []string {
	var result []string
	for _, capability := range strings.Split(value, ",") {
		if capability = strings.TrimSpace(capability); capability != "" {
			result = append(result, capability)
		}
	}
	return result
}
