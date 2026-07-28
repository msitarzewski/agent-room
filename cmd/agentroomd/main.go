package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/msitarzewski/agent-room/internal/app"
	"github.com/msitarzewski/agent-room/internal/artifacts"
	"github.com/msitarzewski/agent-room/internal/auth"
	"github.com/msitarzewski/agent-room/internal/config"
	"github.com/msitarzewski/agent-room/internal/httpapi"
	"github.com/msitarzewski/agent-room/internal/notify"
	"github.com/msitarzewski/agent-room/internal/postgres"
	"github.com/msitarzewski/agent-room/internal/realtime"
	"github.com/msitarzewski/agent-room/internal/runner"
	"github.com/msitarzewski/agent-room/internal/webui"
)

func main() {
	if err := run(); err != nil {
		slog.Error("agentroomd stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, rest, err := config.Load(os.Args[1:])
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return fmt.Errorf("unexpected arguments: %v", rest)
	}
	level := slog.LevelInfo
	if cfg.LogLevel == "debug" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	repo, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer repo.Close()
	if err := requireCurrentSchema(ctx, repo); err != nil {
		return err
	}
	if err := checkArtifactDir(cfg.ArtifactDir); err != nil {
		return err
	}
	authManager, err := auth.New(ctx, repo.Pool(), cfg.SessionSecret, !cfg.Dev, cfg.PublicURL, cfg.OIDCIssuer, cfg.OIDCClientID, cfg.OIDCClientSecret, cfg.OIDCRedirectURL)
	if err != nil {
		return err
	}
	spa, err := webui.Open(cfg.WebDir)
	if err != nil {
		return fmt.Errorf("open production web bundle: %w", err)
	}
	defer func() {
		if err := spa.Close(); err != nil {
			slog.Warn("web root close failed", "error", err)
		}
	}()
	artifactStore, err := artifacts.Open(cfg.ArtifactDir)
	if err != nil {
		return fmt.Errorf("open artifact store: %w", err)
	}
	defer func() {
		if err := artifactStore.Close(); err != nil {
			slog.Warn("artifact store close failed", "error", err)
		}
	}()
	hub := realtime.NewHub()
	service := app.NewService(repo, hub)
	if cfg.Dev && (cfg.CodexBin != "" || cfg.ClaudeBin != "") {
		runtimes := map[string]runner.Runtime{}
		if cfg.CodexBin != "" {
			runtimes["codex"] = runner.Runtime{Executable: cfg.CodexBin, BaseArgs: []string{"exec", "--json", "--skip-git-repo-check"}}
		}
		if cfg.ClaudeBin != "" {
			runtimes["claude"] = runner.Runtime{Executable: cfg.ClaudeBin, BaseArgs: []string{"-p", "--output-format", "stream-json", "--verbose"}}
		}
		runManager, err := runner.New(cfg.WorkspaceRoot, runtimes, cfg.MaxConcurrentRuns)
		if err != nil {
			return fmt.Errorf("initialize development-only managed runtimes: %w", err)
		}
		if err := repo.ReconcileControlOutbox(ctx); err != nil {
			return fmt.Errorf("reconcile interrupted runtime controls: %w", err)
		}
		service = app.NewServiceWithController(repo, hub, runManager)
		go repo.RunControlOutbox(ctx, runManager, func(err error) { slog.Error("runtime control dispatch failed", "error", err) })
	} else if cfg.CodexBin != "" || cfg.ClaudeBin != "" {
		slog.Warn("managed runtimes are disabled outside development mode; configure an isolated worker service before enabling production execution")
	}
	go repo.RunOutbox(ctx, hub, func(err error) { slog.Error("event outbox dispatch failed", "error", err) })
	go runArtifactGC(ctx, artifactStore, repo)
	api := httpapi.New(service, authManager, hub, spa, artifactStore, cfg.MaxRequestBytes, cfg.MaxArtifactBytes, cfg.PublicURL, cfg.AllowedOrigins, cfg.TrustedProxyCIDRs, cfg.Dev)

	publicListener, err := net.Listen("tcp", cfg.HTTPSAddr)
	if err != nil {
		return fmt.Errorf("listen public: %w", err)
	}
	if !cfg.Dev {
		publicListener, err = tlsListener(publicListener, cfg)
		if err != nil {
			closeListener(publicListener)
			return err
		}
	}
	adminListener, err := net.Listen("tcp", cfg.AdminAddr)
	if err != nil {
		closeListener(publicListener)
		return fmt.Errorf("listen admin: %w", err)
	}
	adapterListener, err := net.Listen("tcp", cfg.AdapterAddr)
	if err != nil {
		closeListener(publicListener)
		closeListener(adminListener)
		return fmt.Errorf("listen adapter: %w", err)
	}
	var ready atomic.Bool
	publicServer := &http.Server{Handler: api.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 32 << 10}
	adminMux := http.NewServeMux()
	adminMux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	adminMux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if !ready.Load() || service.Health(r.Context()) != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	adminServer := &http.Server{Handler: adminMux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	adapterServer := &http.Server{Handler: api.AdapterHandler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 32 << 10}
	errs := make(chan error, 3)
	go func() {
		if err := publicServer.Serve(publicListener); !errors.Is(err, http.ErrServerClosed) {
			errs <- fmt.Errorf("public server: %w", err)
		}
	}()
	go func() {
		if err := adminServer.Serve(adminListener); !errors.Is(err, http.ErrServerClosed) {
			errs <- fmt.Errorf("admin server: %w", err)
		}
	}()
	go func() {
		if err := adapterServer.Serve(adapterListener); !errors.Is(err, http.ErrServerClosed) {
			errs <- fmt.Errorf("adapter server: %w", err)
		}
	}()
	ready.Store(true)
	if err := notify.Send("READY=1\nSTATUS=Serving Agent Room"); err != nil {
		return fmt.Errorf("notify readiness: %w", err)
	}
	watchdogStop := make(chan struct{})
	go notify.Watchdog(watchdogStop, func(err error) { slog.Warn("systemd watchdog notification failed", "error", err) })
	slog.Info("agentroomd ready", "public_addr", cfg.HTTPSAddr, "admin_addr", cfg.AdminAddr, "adapter_addr", cfg.AdapterAddr, "dev", cfg.Dev)

	select {
	case <-ctx.Done():
	case err := <-errs:
		cancel()
		return err
	}
	ready.Store(false)
	close(watchdogStop)
	_ = notify.Send("STOPPING=1\nSTATUS=Draining requests")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()
	publicErr := publicServer.Shutdown(shutdownCtx)
	adminErr := adminServer.Shutdown(shutdownCtx)
	adapterErr := adapterServer.Shutdown(shutdownCtx)
	return errors.Join(publicErr, adminErr, adapterErr)
}

func runArtifactGC(ctx context.Context, store *artifacts.Store, repo *postgres.Repository) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			removed, err := store.GC(ctx, 24*time.Hour, repo.ArtifactDigestReferenced)
			if err != nil && ctx.Err() == nil {
				slog.Error("artifact orphan collection failed", "error", err)
			} else if removed > 0 {
				slog.Info("artifact orphan collection complete", "removed", removed)
			}
		}
	}
}

func closeListener(listener net.Listener) {
	if err := listener.Close(); err != nil {
		slog.Warn("listener close failed", "error", err)
	}
}

func tlsListener(listener net.Listener, cfg config.Config) (net.Listener, error) {
	certificate, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load TLS certificate: %w", err)
	}
	caPEM, err := config.ReadRegularFile(cfg.TLSClientCAFile)
	if err != nil {
		return nil, fmt.Errorf("read client CA: %w", err)
	}
	clientCAs := x509.NewCertPool()
	if !clientCAs.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("client CA file contains no valid certificates")
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: clientCAs, NextProtos: []string{"h2", "http/1.1"}}
	return tls.NewListener(listener, tlsConfig), nil
}

func requireCurrentSchema(ctx context.Context, repo *postgres.Repository) error {
	status, err := postgres.MigrationStatus(ctx, repo.Pool())
	if err != nil {
		return fmt.Errorf("inspect database schema: %w", err)
	}
	for _, migration := range status {
		if migration["status"] != "applied" {
			return fmt.Errorf("database migration %v is %v; run agentroomctl migrate up", migration["version"], migration["status"])
		}
	}
	return nil
}

func checkArtifactDir(directory string) error {
	info, err := os.Stat(directory)
	if err != nil {
		return fmt.Errorf("artifact directory: %w", err)
	}
	if !info.IsDir() {
		return errors.New("artifact path is not a directory")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return fmt.Errorf("open artifact root: %w", err)
	}
	defer root.Close()
	probe := fmt.Sprintf(".agentroom-write-check-%d", os.Getpid())
	file, err := root.OpenFile(probe, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("artifact directory is not writable: %w", err)
	}
	if err := file.Close(); err != nil {
		return err
	}
	return root.Remove(probe)
}
