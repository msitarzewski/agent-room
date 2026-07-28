package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/coder/websocket"
	"github.com/msitarzewski/agent-room/internal/app"
	"github.com/msitarzewski/agent-room/internal/artifacts"
	"github.com/msitarzewski/agent-room/internal/auth"
	"github.com/msitarzewski/agent-room/internal/domain"
	"github.com/msitarzewski/agent-room/internal/realtime"
)

type Server struct {
	service        *app.Service
	auth           *auth.Manager
	hub            *realtime.Hub
	maxBody        int64
	allowedOrigins map[string]struct{}
	wsOrigins      []string
	cspConnect     string
	dev            bool
	limits         *limiter
	trustedProxies []*net.IPNet
	spa            http.Handler
	artifacts      *artifacts.Store
	maxArtifact    int64
}

type Problem struct {
	Type        string            `json:"type"`
	Title       string            `json:"title"`
	Status      int               `json:"status"`
	Detail      string            `json:"detail"`
	Code        string            `json:"code"`
	FieldErrors map[string]string `json:"field_errors,omitempty"`
}

type contextKey string

const sessionKey contextKey = "session"
const actorKey contextKey = "actor"

func New(service *app.Service, authManager *auth.Manager, hub *realtime.Hub, spa http.Handler, artifactStore *artifacts.Store, maxBody, maxArtifact int64, publicURL string, allowedOrigins, trustedProxyCIDRs []string, dev bool) *Server {
	s := &Server{service: service, auth: authManager, hub: hub, spa: spa, artifacts: artifactStore, maxBody: maxBody, maxArtifact: maxArtifact, allowedOrigins: make(map[string]struct{}), dev: dev, limits: newLimiter(), cspConnect: "'self'"}
	for _, origin := range allowedOrigins {
		origin = strings.TrimSuffix(origin, "/")
		s.allowedOrigins[origin] = struct{}{}
		if parsed, err := url.Parse(origin); err == nil && parsed.Host != "" {
			s.wsOrigins = append(s.wsOrigins, parsed.Host)
		}
	}
	if websocketOrigin, ok := exactWebSocketOrigin(publicURL, dev); ok {
		s.cspConnect += " " + websocketOrigin
	}
	for _, cidr := range trustedProxyCIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil {
			s.trustedProxies = append(s.trustedProxies, network)
		}
	}
	return s
}

func exactWebSocketOrigin(publicURL string, dev bool) (string, bool) {
	parsed, err := url.Parse(publicURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" ||
		parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", false
	}
	switch parsed.Scheme {
	case "https":
		return "wss://" + parsed.Host, true
	case "http":
		if dev {
			return "ws://" + parsed.Host, true
		}
	}
	return "", false
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/v1/auth/login", s.login)
	mux.HandleFunc("GET /api/v1/auth/callback", s.callback)
	mux.HandleFunc("POST /api/v1/auth/dev-login", s.devLogin)
	mux.HandleFunc("GET /api/v1/auth/session", s.session)
	mux.HandleFunc("POST /api/v1/auth/logout", s.logout)
	mux.HandleFunc("GET /api/v1/projects", s.projects)
	mux.HandleFunc("GET /api/v1/overview", s.overview)
	mux.HandleFunc("GET /api/v1/brief", s.brief)
	mux.HandleFunc("POST /api/v1/brief/acknowledge", s.acknowledgeBrief)
	mux.HandleFunc("GET /api/v1/health/components", s.componentHealth)
	mux.HandleFunc("GET /api/v1/stream", s.stream)
	for path, kind := range map[string]domain.ResourceType{
		"agents": domain.ResourceAgent, "runs": domain.ResourceRun, "sessions": domain.ResourceSession,
		"tasks": domain.ResourceTask, "attention": domain.ResourceAttention, "evidence": domain.ResourceEvidence,
		"artifacts": domain.ResourceArtifact, "approvals": domain.ResourceApproval, "interventions": domain.ResourceIntervention,
		"budgets": domain.ResourceBudget, "claims": domain.ResourceClaim, "audit": domain.ResourceAudit,
		"organizations": domain.ResourceOrganization, "humans": domain.ResourceHuman, "projects-state": domain.ResourceProject,
		"hosts": domain.ResourceHost, "agent-instances": domain.ResourceAgentInstance,
		"task-transitions": domain.ResourceTaskTransition, "situations": domain.ResourceSituation,
		"policies": domain.ResourcePolicy, "deployments": domain.ResourceDeployment,
	} {
		mux.Handle("GET /api/v1/"+path, s.list(kind))
		mux.Handle("GET /api/v1/"+path+"/{id}", s.get(kind))
	}
	mux.HandleFunc("GET /api/v1/events", s.events)
	mux.Handle("GET /api/v1/chat/messages", s.list(domain.ResourceChatMessage))
	mux.HandleFunc("POST /api/v1/chat/messages", s.createChat)
	mux.HandleFunc("POST /api/v1/tasks/{id}/transition", s.transitionTask)
	mux.HandleFunc("POST /api/v1/attention/{id}/acknowledge", s.attention("acknowledged"))
	mux.HandleFunc("POST /api/v1/attention/{id}/resolve", s.attention("resolved"))
	mux.HandleFunc("POST /api/v1/runs/{id}/actions", s.runAction)
	mux.HandleFunc("POST /api/v1/claims/{id}/review", s.reviewClaim)
	mux.HandleFunc("POST /api/v1/approvals/{id}/decision", s.decideApproval)
	mux.HandleFunc("POST /api/v1/approvals", s.requestApproval)
	mux.HandleFunc("POST /api/v1/artifacts", s.uploadArtifact)
	mux.HandleFunc("GET /api/v1/artifacts/{id}/content", s.downloadArtifact)
	if s.spa != nil {
		mux.Handle("/", s.spa)
	}
	return s.middleware(mux)
}

func (s *Server) AdapterHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/ingest", s.ingest)
	mux.Handle("/api/v1/mcp", s.mcpHandler())
	return s.adapterMiddleware(mux)
}

func (s *Server) adapterMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		projectID := r.URL.Query().Get("project_id")
		if projectID == "" {
			writeProblemStatus(w, http.StatusBadRequest, "project_required", "project_id is required")
			return
		}
		identity, err := s.auth.ResolveServiceToken(r.Context(), r, projectID)
		if err != nil {
			writeProblem(w, err)
			return
		}
		if !s.limits.allow("service:"+identity.ID+":"+projectID, 600, time.Minute) {
			writeProblemStatus(w, http.StatusTooManyRequests, "rate_limited", "Adapter request rate exceeded")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey, identity.Actor)))
	})
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/ingest" || r.URL.Path == "/api/v1/mcp" || strings.HasPrefix(r.URL.Path, "/api/v1/adapters/") {
			http.NotFound(w, r)
			return
		}
		clientIP := remoteIP(r, s.trustedProxies)
		if !s.limits.allow("ip:"+clientIP, 120, time.Minute) {
			writeProblemStatus(w, http.StatusTooManyRequests, "rate_limited", "Request rate exceeded")
			return
		}
		if r.URL.Path == "/api/v1/auth/login" && !s.limits.allow("login:"+clientIP, 10, time.Minute) {
			writeProblemStatus(w, http.StatusTooManyRequests, "rate_limited", "Login rate exceeded")
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; frame-ancestors 'none'; object-src 'none'; form-action 'self'; script-src 'self'; style-src 'self'; connect-src "+s.cspConnect+"; upgrade-insecure-requests")
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, s.maxBody)
		}
		if s.isPublic(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				writeProblemStatus(w, http.StatusMethodNotAllowed, "method_not_allowed", "SPA routes only accept GET or HEAD")
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			writeProblemStatus(w, http.StatusForbidden, "service_token_scope", "Service tokens are not accepted by the public listener")
			return
		}
		session, err := s.auth.Resolve(r.Context(), r)
		if err != nil {
			writeProblem(w, err)
			return
		}
		if projectID := r.URL.Query().Get("project_id"); projectID != "" {
			actor, err := s.auth.ActorForProject(r.Context(), session.User, projectID)
			if err != nil {
				writeProblem(w, err)
				return
			}
			session.User.Capabilities = session.User.Capabilities[:0]
			for capability := range actor.Capabilities {
				session.User.Capabilities = append(session.User.Capabilities, capability)
			}
			if !s.limits.allow("actor:"+session.User.ID+":"+projectID, 300, time.Minute) {
				writeProblemStatus(w, http.StatusTooManyRequests, "rate_limited", "Actor request rate exceeded")
				return
			}
		}
		if isMutation(r.Method) {
			if !s.validOrigin(r) {
				writeProblemStatus(w, http.StatusForbidden, "origin_denied", "Request origin is not allowed")
				return
			}
			if err := s.auth.CheckCSRF(r.Context(), r); err != nil {
				writeProblemStatus(w, http.StatusForbidden, "csrf_failed", "CSRF token is missing or invalid")
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey, session)))
	})
}

func (s *Server) isPublic(path string) bool {
	if path == "/healthz" || path == "/api/v1/auth/login" || path == "/api/v1/auth/callback" ||
		(s.dev && path == "/api/v1/auth/dev-login") {
		return true
	}
	// The SPA shell and its immutable assets must remain reachable so an
	// unauthenticated browser can initiate OIDC. Every API route remains
	// authenticated unless explicitly allowlisted above.
	return path != "/api" && !strings.HasPrefix(path, "/api/")
}

func (s *Server) validOrigin(r *http.Request) bool {
	origin := strings.TrimSuffix(r.Header.Get("Origin"), "/")
	if origin == "" {
		return false
	}
	if _, ok := s.allowedOrigins[origin]; ok {
		return true
	}
	if s.dev {
		return origin == "http://"+r.Host
	}
	return false
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	location, browser, err := s.auth.BeginOIDC(r.Context(), r.URL.Query().Get("return_to"))
	if err != nil {
		writeProblem(w, err)
		return
	}
	s.auth.SetOIDCCookie(w, browser)
	http.Redirect(w, r, location, http.StatusFound)
}

func (s *Server) callback(w http.ResponseWriter, r *http.Request) {
	browser := s.auth.OIDCCookie(r)
	s.auth.ClearOIDCCookie(w)
	session, token, returnTo, err := s.auth.FinishOIDC(r.Context(), r.URL.Query().Get("state"), r.URL.Query().Get("code"), browser)
	if err != nil {
		writeProblem(w, err)
		return
	}
	s.auth.SetCookie(w, token, session.ExpiresAt)
	if returnTo == "" {
		returnTo = "/"
	}
	http.Redirect(w, r, returnTo, http.StatusSeeOther)
}

func (s *Server) devLogin(w http.ResponseWriter, r *http.Request) {
	if !s.dev {
		http.NotFound(w, r)
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, err)
		return
	}
	user, err := s.auth.AuthenticatePassword(r.Context(), input.Username, input.Password)
	if err != nil {
		writeProblem(w, err)
		return
	}
	session, token, err := s.auth.CreateSession(r.Context(), user, "")
	if err != nil {
		writeProblem(w, err)
		return
	}
	s.auth.SetCookie(w, token, session.ExpiresAt)
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	session := currentSession(r)
	csrf, err := s.auth.CSRFToken(r.Context(), r)
	if err != nil {
		writeProblem(w, err)
		return
	}
	session.CSRFToken = csrf
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	s.auth.Logout(r.Context(), r)
	s.auth.ClearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) projects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.auth.Projects(r.Context(), currentSession(r).User)
	if err != nil {
		writeProblem(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": projects})
}

func (s *Server) list(kind domain.ResourceType) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		projectID, ok := requireProject(w, r)
		if !ok {
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		page, err := s.service.List(r.Context(), currentActor(r), projectID, kind, r.URL.Query().Get("cursor"), limit)
		if err != nil {
			writeProblem(w, err)
			return
		}
		writeJSON(w, http.StatusOK, page)
	})
}

func (s *Server) get(kind domain.ResourceType) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		projectID, ok := requireProject(w, r)
		if !ok {
			return
		}
		value, err := s.service.Get(r.Context(), currentActor(r), projectID, kind, r.PathValue("id"))
		if err != nil {
			writeProblem(w, err)
			return
		}
		var base struct {
			Version int64 `json:"version"`
		}
		_ = json.Unmarshal(value, &base)
		w.Header().Set("ETag", strconv.FormatInt(base.Version, 10))
		writeRawJSON(w, http.StatusOK, value)
	})
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}
	value, err := s.service.Overview(r.Context(), currentActor(r), projectID)
	if err != nil {
		writeProblem(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) brief(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}
	after := int64(-1)
	var err error
	if r.URL.Query().Get("after") != "" {
		after, err = strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	}
	if err != nil && r.URL.Query().Get("after") != "" {
		writeProblemStatus(w, http.StatusBadRequest, "invalid_cursor", "after must be an integer cursor")
		return
	}
	value, err := s.service.Brief(r.Context(), currentActor(r), projectID, after)
	if err != nil {
		writeProblem(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) acknowledgeBrief(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}
	idem, ok := requireIdempotency(w, r)
	if !ok {
		return
	}
	var input struct {
		ExpectedCursor int64 `json:"expected_cursor"`
		ThroughCursor  int64 `json:"through_cursor"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, app.Invalid("invalid brief acknowledgement"))
		return
	}
	replayed, err := s.service.AcknowledgeBrief(r.Context(), currentActor(r), projectID, idem, input.ExpectedCursor, input.ThroughCursor)
	if err != nil {
		writeProblem(w, err)
		return
	}
	if replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	writeJSON(w, http.StatusOK, map[string]any{"reviewed_cursor": input.ThroughCursor, "replayed": replayed})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("cursor"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	page, err := s.service.Events(r.Context(), currentActor(r), projectID, after, limit)
	if err != nil {
		writeProblem(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) createChat(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}
	idem, ok := requireIdempotency(w, r)
	if !ok {
		return
	}
	var input struct {
		SessionID string          `json:"session_id"`
		RunID     string          `json:"run_id"`
		Body      string          `json:"body"`
		Metadata  json.RawMessage `json:"metadata"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, err)
		return
	}
	if strings.TrimSpace(input.Body) == "" {
		writeProblemStatus(w, http.StatusBadRequest, "validation_failed", "body is required")
		return
	}
	message := domain.ChatMessage{Base: domain.Base{ID: s.service.NewID()}, SessionID: input.SessionID, RunID: input.RunID, AuthorID: currentSession(r).User.ID, Role: "user", Body: input.Body, Metadata: input.Metadata}
	result, err := s.service.Create(r.Context(), currentSession(r).User.Actor(), domain.ResourceChatMessage, projectID, idem, message, r.RemoteAddr)
	writeResult(w, result, err, http.StatusCreated)
}

func (s *Server) transitionTask(w http.ResponseWriter, r *http.Request) {
	projectID, version, idem, ok := mutationHeaders(w, r)
	if !ok {
		return
	}
	var input struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, err)
		return
	}
	result, err := s.service.TransitionTask(r.Context(), currentSession(r).User.Actor(), projectID, r.PathValue("id"), input.Status, input.Reason, idem, version, r.RemoteAddr)
	writeResult(w, result, err, http.StatusOK)
}

func (s *Server) attention(status string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID, version, idem, ok := mutationHeaders(w, r)
		if !ok {
			return
		}
		result, err := s.service.SetAttentionStatus(r.Context(), currentSession(r).User.Actor(), projectID, r.PathValue("id"), status, idem, version, r.RemoteAddr)
		writeResult(w, result, err, http.StatusOK)
	}
}

func (s *Server) runAction(w http.ResponseWriter, r *http.Request) {
	projectID, version, idem, ok := mutationHeaders(w, r)
	if !ok {
		return
	}
	var input struct {
		Action     string `json:"action"`
		Message    string `json:"message"`
		ApprovalID string `json:"approval_id"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, err)
		return
	}
	result, err := s.service.RunAction(r.Context(), currentSession(r).User.Actor(), projectID, r.PathValue("id"), input.Action, input.Message, input.ApprovalID, idem, version, r.RemoteAddr)
	writeResult(w, result, err, http.StatusOK)
}

func (s *Server) reviewClaim(w http.ResponseWriter, r *http.Request) {
	projectID, version, idem, ok := mutationHeaders(w, r)
	if !ok {
		return
	}
	var input struct {
		Decision string `json:"decision"`
		Note     string `json:"note"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, err)
		return
	}
	result, err := s.service.ReviewClaim(r.Context(), currentSession(r).User.Actor(), projectID, r.PathValue("id"), input.Decision, input.Note, idem, version, r.RemoteAddr)
	writeResult(w, result, err, http.StatusOK)
}

func (s *Server) decideApproval(w http.ResponseWriter, r *http.Request) {
	projectID, version, idem, ok := mutationHeaders(w, r)
	if !ok {
		return
	}
	var input struct {
		Decision string `json:"decision"`
		Note     string `json:"note"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, err)
		return
	}
	result, err := s.service.DecideApproval(r.Context(), currentSession(r).User.Actor(), projectID, r.PathValue("id"), input.Decision, input.Note, idem, version, r.RemoteAddr)
	writeResult(w, result, err, http.StatusOK)
}

func (s *Server) requestApproval(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}
	idem, ok := requireIdempotency(w, r)
	if !ok {
		return
	}
	var input struct {
		RunID                 string `json:"run_id"`
		Action                string `json:"action"`
		Message               string `json:"message"`
		ExpectedTargetVersion int64  `json:"expected_target_version"`
		ExpiresInSeconds      int64  `json:"expires_in_seconds"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, app.Invalid("invalid approval request"))
		return
	}
	result, err := s.service.RequestRunApproval(r.Context(), currentActor(r), projectID, input.RunID, input.Action, input.Message, idem, r.RemoteAddr, input.ExpectedTargetVersion, time.Duration(input.ExpiresInSeconds)*time.Second)
	writeResult(w, result, err, http.StatusCreated)
}

func (s *Server) uploadArtifact(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}
	idem, ok := requireIdempotency(w, r)
	if !ok {
		return
	}
	if s.artifacts == nil {
		writeProblemStatus(w, http.StatusServiceUnavailable, "artifact_store_unavailable", "Artifact storage is unavailable")
		return
	}
	name := strings.TrimSpace(r.Header.Get("X-Artifact-Name"))
	if !validArtifactName(name) {
		writeProblem(w, app.Invalid("X-Artifact-Name must be a plain UTF-8 filename of at most 255 bytes"))
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		writeProblem(w, app.Invalid("Content-Type must be a valid media type"))
		return
	}
	stored, err := s.artifacts.Put(r.Body, s.maxArtifact)
	if err != nil {
		writeProblem(w, app.Invalid("artifact content was rejected"))
		return
	}
	artifact := domain.Artifact{
		Base: domain.Base{ID: s.service.NewID()}, TaskID: r.URL.Query().Get("task_id"), RunID: r.URL.Query().Get("run_id"),
		Name: name, MediaType: mediaType, URI: "artifact://" + stored.Path, Digest: stored.Digest,
		Source: domain.Source{System: "upload"},
	}
	result, err := s.service.Create(r.Context(), currentActor(r), domain.ResourceArtifact, projectID, idem, artifact, r.RemoteAddr)
	writeResult(w, result, err, http.StatusCreated)
}

func (s *Server) downloadArtifact(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}
	if s.artifacts == nil {
		writeProblemStatus(w, http.StatusServiceUnavailable, "artifact_store_unavailable", "Artifact storage is unavailable")
		return
	}
	if !s.limits.allow("artifact-download:"+currentActor(r).ID+":"+projectID, 30, time.Minute) {
		writeProblemStatus(w, http.StatusTooManyRequests, "rate_limited", "Artifact download rate exceeded")
		return
	}
	raw, err := s.service.Get(r.Context(), currentActor(r), projectID, domain.ResourceArtifact, r.PathValue("id"))
	if err != nil {
		writeProblem(w, err)
		return
	}
	var artifact domain.Artifact
	if err := json.Unmarshal(raw, &artifact); err != nil {
		writeProblem(w, err)
		return
	}
	file, _, err := s.artifacts.OpenVerified(artifact.Digest)
	if err != nil {
		writeProblemStatus(w, http.StatusConflict, "artifact_verification_failed", "Artifact content failed integrity verification")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		writeProblem(w, err)
		return
	}
	w.Header().Set("Content-Type", artifact.MediaType)
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": artifact.Name})
	w.Header().Set("Content-Disposition", disposition)
	w.Header().Set("ETag", `"`+artifact.Digest+`"`)
	http.ServeContent(w, r, artifact.Name, info.ModTime(), file)
}

func validArtifactName(name string) bool {
	if name == "" || len(name) > 255 || !utf8.ValidString(name) || name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, `/\`) {
		return false
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func (s *Server) ingest(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Events []domain.Event `json:"events"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, err)
		return
	}
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}
	for _, event := range input.Events {
		if event.ProjectID != projectID {
			writeProblemStatus(w, http.StatusBadRequest, "project_mismatch", "Every event must match project_id")
			return
		}
	}
	if err := s.service.Ingest(r.Context(), currentActor(r), input.Events); err != nil {
		writeProblem(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return
	}
	after, err := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	if err != nil && r.URL.Query().Get("after") != "" {
		writeProblemStatus(w, http.StatusBadRequest, "invalid_cursor", "after must be an integer cursor")
		return
	}
	socketKey := currentSession(r).User.ID + ":" + projectID
	if !s.limits.acquireSocket(socketKey, 5) {
		writeProblemStatus(w, http.StatusTooManyRequests, "websocket_limit", "Too many concurrent project streams")
		return
	}
	defer s.limits.releaseSocket(socketKey)
	messages, cancel := s.hub.Subscribe(projectID)
	defer cancel()
	replay, err := s.service.Events(r.Context(), currentActor(r), projectID, after, 1000)
	if err != nil {
		writeProblem(w, err)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: s.wsOrigins, CompressionMode: websocket.CompressionContextTakeover})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	validate := func(ctx context.Context) error {
		session, err := s.auth.Resolve(ctx, r)
		if err != nil {
			return auth.ErrUnauthenticated
		}
		_, err = s.auth.ActorForProject(ctx, session.User, projectID)
		return err
	}
	if err := realtime.WriteLoop(r.Context(), conn, messages, replay.Items, validate); err != nil {
		code := websocket.StatusCode(4403)
		reason := "project access revoked"
		if errors.Is(err, auth.ErrUnauthenticated) {
			code, reason = websocket.StatusCode(4401), "session expired or revoked"
		}
		_ = conn.Close(code, reason)
	}
}

func (s *Server) componentHealth(w http.ResponseWriter, r *http.Request) {
	components, err := s.service.ComponentHealth(r.Context())
	code := http.StatusOK
	if err != nil {
		components, code = map[string]any{"database": map[string]string{"status": "degraded"}}, http.StatusServiceUnavailable
	}
	artifactStatus := "ok"
	if s.artifacts == nil || s.artifacts.Health() != nil {
		artifactStatus, code = "degraded", http.StatusServiceUnavailable
	}
	components["artifacts"] = map[string]string{"status": artifactStatus}
	components["oidc"] = map[string]any{"status": statusFor(s.dev || s.auth.OIDCConfigured()), "configured": s.auth.OIDCConfigured()}
	components["realtime"] = map[string]string{"status": "ok"}
	components["checked_at"] = time.Now().UTC()
	writeJSON(w, code, components)
}

func statusFor(ok bool) string {
	if ok {
		return "ok"
	}
	return "degraded"
}

func currentSession(r *http.Request) auth.Session {
	session, _ := r.Context().Value(sessionKey).(auth.Session)
	return session
}

func currentActor(r *http.Request) app.Actor {
	if actor, ok := r.Context().Value(actorKey).(app.Actor); ok {
		return actor
	}
	return currentSession(r).User.Actor()
}

func requireProject(w http.ResponseWriter, r *http.Request) (string, bool) {
	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		writeProblemStatus(w, http.StatusBadRequest, "project_required", "project_id is required")
		return "", false
	}
	return projectID, true
}

func requireIdempotency(w http.ResponseWriter, r *http.Request) (string, bool) {
	value := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if value == "" || len(value) > 200 {
		writeProblemStatus(w, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key is required and must be at most 200 characters")
		return "", false
	}
	return value, true
}

func mutationHeaders(w http.ResponseWriter, r *http.Request) (string, int64, string, bool) {
	projectID, ok := requireProject(w, r)
	if !ok {
		return "", 0, "", false
	}
	idem, ok := requireIdempotency(w, r)
	if !ok {
		return "", 0, "", false
	}
	version, err := app.ParseExpectedVersion(r.Header.Get("If-Match"))
	if err != nil {
		writeProblemStatus(w, http.StatusPreconditionRequired, "version_required", err.Error())
		return "", 0, "", false
	}
	return projectID, version, idem, true
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return app.Invalid("request body must be valid JSON and match the expected schema")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return app.Invalid("request body must contain exactly one JSON value")
	}
	return nil
}

func writeResult(w http.ResponseWriter, result app.CommandResult, err error, success int) {
	if err != nil {
		writeProblem(w, err)
		return
	}
	w.Header().Set("ETag", strconv.FormatInt(resourceVersion(result.Resource), 10))
	if result.Replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	}
	writeRawJSON(w, success, result.Resource)
}

func resourceVersion(raw json.RawMessage) int64 {
	var value struct {
		Version int64 `json:"version"`
	}
	_ = json.Unmarshal(raw, &value)
	return value.Version
}

func writeProblem(w http.ResponseWriter, err error) {
	var validation app.ValidationError
	switch {
	case errors.Is(err, auth.ErrUnauthenticated):
		writeProblemStatus(w, http.StatusUnauthorized, "authentication_required", "Authentication is required or credentials are invalid")
	case errors.Is(err, app.ErrDenied):
		writeProblemStatus(w, http.StatusForbidden, "capability_denied", "The actor lacks a required project capability")
	case errors.Is(err, app.ErrNotFound):
		writeProblemStatus(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, app.ErrVersionConflict):
		writeProblemStatus(w, http.StatusConflict, "version_conflict", err.Error())
	case errors.Is(err, app.ErrIdempotency):
		writeProblemStatus(w, http.StatusConflict, "idempotency_conflict", err.Error())
	case errors.Is(err, app.ErrUnsupported):
		writeProblemStatus(w, http.StatusConflict, "runtime_action_unsupported", "The live runtime does not support this action")
	case errors.Is(err, app.ErrBudgetExceeded):
		writeProblemStatus(w, http.StatusConflict, "budget_exhausted", "An enforced budget blocks this command")
	case errors.As(err, &validation):
		writeProblemStatus(w, http.StatusBadRequest, "validation_failed", validation.Message)
	default:
		correlationID := newCorrelationID()
		slog.Error("request failed", "correlation_id", correlationID, "error", err)
		w.Header().Set("X-Correlation-ID", correlationID)
		writeProblemStatus(w, http.StatusInternalServerError, "internal_error", "The request could not be completed")
	}
}

func newCorrelationID() string {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(value[:])
}

func writeProblemStatus(w http.ResponseWriter, status int, code, detail string) {
	title := http.StatusText(status)
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Problem{Type: "https://agentroom.dev/problems/" + code, Title: title, Status: status, Detail: detail, Code: code})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeRawJSON(w http.ResponseWriter, status int, value []byte) {
	if !json.Valid(value) {
		writeProblemStatus(w, http.StatusInternalServerError, "invalid_server_json", "The response could not be encoded")
		return
	}
	var document any
	if err := json.Unmarshal(value, &document); err != nil {
		writeProblemStatus(w, http.StatusInternalServerError, "invalid_server_json", "The response could not be encoded")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(document)
}

func isMutation(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
}
