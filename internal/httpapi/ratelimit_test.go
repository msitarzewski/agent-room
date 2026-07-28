package httpapi

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/msitarzewski/agent-room/internal/app"
	"github.com/msitarzewski/agent-room/internal/auth"
	"github.com/msitarzewski/agent-room/internal/domain"
)

func TestLimiterBurstAndSocketCap(t *testing.T) {
	l := newLimiter()
	first := l.allow("actor", 2, time.Hour)
	second := l.allow("actor", 2, time.Hour)
	if !first || !second {
		t.Fatal("initial burst rejected")
	}
	if l.allow("actor", 2, time.Hour) {
		t.Fatal("request beyond burst accepted")
	}
	if !l.acquireSocket("actor:project", 1) {
		t.Fatal("first socket rejected")
	}
	if l.acquireSocket("actor:project", 1) {
		t.Fatal("socket beyond cap accepted")
	}
	l.releaseSocket("actor:project")
	if !l.acquireSocket("actor:project", 1) {
		t.Fatal("released socket capacity was not restored")
	}
}

func TestLimiterBoundedLRUAndExpiry(t *testing.T) {
	now := time.Unix(0, 0)
	l := newLimiterWithClock(func() time.Time { return now }, 2)
	if !l.allow("oldest", 1, time.Hour) || !l.allow("newer", 1, time.Hour) {
		t.Fatal("initial keys rejected")
	}
	if !l.allow("third", 1, time.Hour) {
		t.Fatal("third key rejected instead of evicting oldest")
	}
	if len(l.buckets) != 2 {
		t.Fatalf("bucket count=%d", len(l.buckets))
	}
	if _, exists := l.buckets["oldest"]; exists {
		t.Fatal("deterministic LRU eviction did not remove oldest")
	}
	now = now.Add(11 * time.Minute)
	if !l.allow("fresh", 1, time.Hour) {
		t.Fatal("fresh request rejected")
	}
	if len(l.buckets) != 1 {
		t.Fatalf("expired buckets retained: %d", len(l.buckets))
	}
}

func TestLimiterRejectsDisabledCapacityAndBoundsSockets(t *testing.T) {
	t.Parallel()
	now := time.Unix(0, 0)
	disabled := newLimiterWithClock(func() time.Time { return now }, 0)
	if disabled.allow("request", 1, time.Minute) {
		t.Fatal("disabled limiter accepted a new request key")
	}
	if disabled.acquireSocket("socket", 1) {
		t.Fatal("disabled limiter accepted a new socket key")
	}

	limited := newLimiterWithClock(func() time.Time { return now }, 1)
	if !limited.acquireSocket("first", 2) {
		t.Fatal("socket capacity rejected first connection")
	}
	if !limited.acquireSocket("first", 2) {
		t.Fatal("socket capacity rejected second connection")
	}
	if limited.acquireSocket("first", 2) {
		t.Fatal("per-key socket maximum was bypassed")
	}
	if limited.acquireSocket("second", 1) {
		t.Fatal("global socket-key bound was bypassed")
	}
	limited.releaseSocket("first")
	limited.releaseSocket("first")
	limited.releaseSocket("missing")
	if len(limited.sockets) != 0 {
		t.Fatalf("socket counters leaked: %v", limited.sockets)
	}
}

func TestRemoteIPOnlyTrustsForwardingOverVerifiedMTLS(t *testing.T) {
	t.Parallel()
	_, trusted, err := net.ParseCIDR("192.0.2.0/24")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "192.0.2.10:443"
	request.Header.Set("X-Forwarded-For", "203.0.113.9, 198.51.100.2")
	if got := remoteIP(request, []*net.IPNet{trusted}); got != "192.0.2.10" {
		t.Fatalf("unverified forwarding trusted: %q", got)
	}
	request.TLS = &tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{{}}}}
	if got := remoteIP(request, []*net.IPNet{trusted}); got != "203.0.113.9" {
		t.Fatalf("verified forwarding ignored: %q", got)
	}
	request.Header.Set("X-Forwarded-For", "not-an-ip")
	if got := remoteIP(request, []*net.IPNet{trusted}); got != "192.0.2.10" {
		t.Fatalf("invalid forwarding trusted: %q", got)
	}
	request.RemoteAddr = "not-a-host-port"
	if got := remoteIP(request, []*net.IPNet{trusted}); got != "not-a-host-port" {
		t.Fatalf("unparseable peer changed: %q", got)
	}
	if !trustedIP(net.ParseIP("192.0.2.42"), []*net.IPNet{trusted}) ||
		trustedIP(net.ParseIP("198.51.100.42"), []*net.IPNet{trusted}) {
		t.Fatal("trusted-network matching is incorrect")
	}
}

func TestArtifactFilenameRejectsControlCharacters(t *testing.T) {
	for _, name := range []string{"tab\t.txt", "delete\x7f.txt", "next\u0085line.txt", "nested/file.txt", `nested\file.txt`} {
		if validArtifactName(name) {
			t.Fatalf("unsafe artifact filename accepted: %q", name)
		}
	}
	for _, name := range []string{"evidence.json", "résumé.pdf", "build output.txt"} {
		if !validArtifactName(name) {
			t.Fatalf("safe artifact filename rejected: %q", name)
		}
	}
}

func TestServerPublicBoundaryAndSecurityHeaders(t *testing.T) {
	t.Parallel()
	spa := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("login shell"))
	})
	server := New(nil, nil, nil, spa, nil, 64, 64,
		"https://agentroom.test",
		[]string{"https://agentroom.test/", "not a URL"},
		[]string{"192.0.2.0/24", "invalid"},
		false,
	)
	handler := server.Handler()

	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health status=%d body=%s", health.Code, health.Body.String())
	}
	for name, expected := range map[string]string{
		"Cache-Control":           "no-store",
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "no-referrer",
		"Permissions-Policy":      "camera=(), microphone=(), geolocation=()",
		"Content-Security-Policy": "default-src 'self'; base-uri 'none'; frame-ancestors 'none'; object-src 'none'; form-action 'self'; script-src 'self'; style-src 'self'; connect-src 'self' wss://agentroom.test; upgrade-insecure-requests",
	} {
		if got := health.Header().Get(name); got != expected {
			t.Fatalf("%s=%q", name, got)
		}
	}

	hidden := httptest.NewRecorder()
	handler.ServeHTTP(hidden, httptest.NewRequest(http.MethodPost, "/api/v1/ingest", nil))
	if hidden.Code != http.StatusNotFound {
		t.Fatalf("adapter route exposed on public listener: %d", hidden.Code)
	}

	bearer := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/tasks?project_id=one", nil)
	request.Header.Set("Authorization", "Bearer service-secret")
	handler.ServeHTTP(bearer, request)
	expectRecordedProblem(t, bearer, http.StatusForbidden, "service_token_scope")

	if server.isPublic("/api/v1/auth/dev-login") {
		t.Fatal("dev login exposed outside development mode")
	}
	shell := httptest.NewRecorder()
	handler.ServeHTTP(shell, httptest.NewRequest(http.MethodGet, "/", nil))
	if shell.Code != http.StatusOK || shell.Body.String() != "login shell" {
		t.Fatalf("unauthenticated SPA shell unavailable: status=%d body=%q", shell.Code, shell.Body.String())
	}
	if server.isPublic("/api") || server.isPublic("/api/v1/tasks") {
		t.Fatal("protected API path classified as public")
	}
	server.dev = true
	if !server.isPublic("/api/v1/auth/dev-login") {
		t.Fatal("dev login unavailable in development mode")
	}
}

func TestAdapterListenerRequiresProjectBeforeAuthentication(t *testing.T) {
	t.Parallel()
	server := New(nil, &auth.Manager{}, nil, nil, nil, 64, 64, "", nil, nil, true)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/ingest",
		strings.NewReader(`{"events":[]}`),
	)

	server.AdapterHandler().ServeHTTP(recorder, request)

	expectRecordedProblem(t, recorder, http.StatusBadRequest, "project_required")
}

func TestAuthenticationEntryPointsFailClosedWithoutConfiguration(t *testing.T) {
	t.Parallel()
	server := &Server{auth: &auth.Manager{}}

	loginRecorder := httptest.NewRecorder()
	server.login(loginRecorder, httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil))
	expectRecordedProblem(t, loginRecorder, http.StatusInternalServerError, "internal_error")

	callbackRecorder := httptest.NewRecorder()
	server.callback(callbackRecorder, httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback", nil))
	expectRecordedProblem(t, callbackRecorder, http.StatusInternalServerError, "internal_error")
	if cookie := callbackRecorder.Header().Get("Set-Cookie"); !strings.Contains(cookie, auth.DevOIDCCookieName+"=") ||
		!strings.Contains(cookie, "Max-Age=0") {
		t.Fatalf("callback did not clear the OIDC transaction cookie: %q", cookie)
	}

	devLoginRecorder := httptest.NewRecorder()
	server.devLogin(devLoginRecorder, httptest.NewRequest(http.MethodPost, "/api/v1/auth/dev-login", nil))
	if devLoginRecorder.Code != http.StatusNotFound {
		t.Fatalf("disabled development login status=%d", devLoginRecorder.Code)
	}

	server.dev = true
	invalidJSONRecorder := httptest.NewRecorder()
	server.devLogin(
		invalidJSONRecorder,
		httptest.NewRequest(http.MethodPost, "/api/v1/auth/dev-login", strings.NewReader(`{"username":`)),
	)
	expectRecordedProblem(t, invalidJSONRecorder, http.StatusBadRequest, "validation_failed")
}

func TestResourceHandlersRequireProjectBeforeAccessingDependencies(t *testing.T) {
	t.Parallel()
	server := &Server{}
	tests := []struct {
		name    string
		handler http.Handler
		body    string
	}{
		{"list", server.list(domain.ResourceTask), ""},
		{"get", server.get(domain.ResourceTask), ""},
		{"overview", http.HandlerFunc(server.overview), ""},
		{"brief", http.HandlerFunc(server.brief), ""},
		{"acknowledge brief", http.HandlerFunc(server.acknowledgeBrief), ""},
		{"events", http.HandlerFunc(server.events), ""},
		{"create chat", http.HandlerFunc(server.createChat), ""},
		{"transition task", http.HandlerFunc(server.transitionTask), ""},
		{"attention", server.attention("acknowledged"), ""},
		{"run action", http.HandlerFunc(server.runAction), ""},
		{"review claim", http.HandlerFunc(server.reviewClaim), ""},
		{"decide approval", http.HandlerFunc(server.decideApproval), ""},
		{"request approval", http.HandlerFunc(server.requestApproval), ""},
		{"upload artifact", http.HandlerFunc(server.uploadArtifact), ""},
		{"download artifact", http.HandlerFunc(server.downloadArtifact), ""},
		{"ingest", http.HandlerFunc(server.ingest), `{"events":[]}`},
		{"stream", http.HandlerFunc(server.stream), ""},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/v1/test", strings.NewReader(test.body))
			test.handler.ServeHTTP(recorder, request)
			expectRecordedProblem(t, recorder, http.StatusBadRequest, "project_required")
		})
	}
}

func TestDirectProtocolHandlersFailClosedWithoutAuthContext(t *testing.T) {
	t.Parallel()
	server := &Server{auth: &auth.Manager{}}

	sessionRecorder := httptest.NewRecorder()
	sessionRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil).
		WithContext(context.WithValue(context.Background(), sessionKey, auth.Session{
			User: auth.User{ID: "context-only"},
		}))
	server.session(sessionRecorder, sessionRequest)
	expectRecordedProblem(t, sessionRecorder, http.StatusForbidden, "capability_denied")

	mcpRecorder := httptest.NewRecorder()
	mcpRequest := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
	))
	mcpRequest.Header.Set("Content-Type", "application/json")
	mcpRequest.Header.Set("Accept", "application/json, text/event-stream")
	server.mcpHandler().ServeHTTP(mcpRecorder, mcpRequest)
	if mcpRecorder.Code == http.StatusOK {
		t.Fatalf("MCP handler accepted missing project/actor context: %s", mcpRecorder.Body.String())
	}
}

func TestOriginMatchingIsExactAndDevelopmentFallbackIsHostBound(t *testing.T) {
	t.Parallel()
	server := New(nil, nil, nil, nil, nil, 1, 1,
		"https://agentroom.test",
		[]string{"https://agentroom.test/"},
		nil,
		false,
	)
	for origin, allowed := range map[string]bool{
		"https://agentroom.test":      true,
		"https://agentroom.test/":     true,
		"https://agentroom.test.evil": false,
		"":                            false,
	} {
		request := httptest.NewRequest(http.MethodPost, "https://agentroom.test/api", nil)
		request.Header.Set("Origin", origin)
		if got := server.validOrigin(request); got != allowed {
			t.Fatalf("origin=%q allowed=%v", origin, got)
		}
	}
	server.dev = true
	request := httptest.NewRequest(http.MethodPost, "http://localhost:8080/api", nil)
	request.Host = "localhost:8080"
	request.Header.Set("Origin", "http://localhost:8080")
	if !server.validOrigin(request) {
		t.Fatal("host-bound development origin rejected")
	}
	request.Header.Set("Origin", "http://localhost:8081")
	if server.validOrigin(request) {
		t.Fatal("cross-origin development request accepted")
	}
}

func TestWebSocketCSPOriginIsExactAndHeaderSafe(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		publicURL string
		dev       bool
		expected  string
		ok        bool
	}{
		{"https://agentroom.test", false, "wss://agentroom.test", true},
		{"https://agentroom.test:8443/", false, "wss://agentroom.test:8443", true},
		{"http://127.0.0.1:58443", true, "ws://127.0.0.1:58443", true},
		{"http://agentroom.test", false, "", false},
		{"https://user@agentroom.test", false, "", false},
		{"https://agentroom.test/path", false, "", false},
		{"https://agentroom.test?source=wss:", false, "", false},
		{"https://agentroom.test/#fragment", false, "", false},
		{"https://agentroom.test\r\nX-Injected: yes", false, "", false},
		{"", false, "", false},
	} {
		got, ok := exactWebSocketOrigin(test.publicURL, test.dev)
		if got != test.expected || ok != test.ok {
			t.Fatalf("publicURL=%q origin=%q ok=%v", test.publicURL, got, ok)
		}
		if strings.ContainsAny(got, "\r\n") || got == "ws:" || got == "wss:" {
			t.Fatalf("unsafe CSP source generated: %q", got)
		}
	}

	server := New(nil, nil, nil, nil, nil, 1, 1,
		"https://agentroom.test\r\nX-Injected: yes",
		[]string{"https://agentroom.test"},
		nil,
		false,
	)
	if server.cspConnect != "'self'" {
		t.Fatalf("invalid public URL entered CSP: %q", server.cspConnect)
	}
}

func TestRequestHelpersAndErrorMapping(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(http.MethodPost, "/resource", strings.NewReader(`{"name":"one"}`))
	var payload struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(request, &payload); err != nil || payload.Name != "one" {
		t.Fatalf("payload=%+v err=%v", payload, err)
	}
	for _, body := range []string{`{"unknown":true}`, `{"name":"one"} {"name":"two"}`, ``} {
		request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		if err := decodeJSON(request, &payload); err == nil {
			t.Fatalf("invalid JSON accepted: %q", body)
		}
	}

	for _, test := range []struct {
		err    error
		status int
		code   string
	}{
		{auth.ErrUnauthenticated, http.StatusUnauthorized, "authentication_required"},
		{app.ErrDenied, http.StatusForbidden, "capability_denied"},
		{app.ErrNotFound, http.StatusNotFound, "not_found"},
		{app.ErrVersionConflict, http.StatusConflict, "version_conflict"},
		{app.ErrIdempotency, http.StatusConflict, "idempotency_conflict"},
		{app.ErrUnsupported, http.StatusConflict, "runtime_action_unsupported"},
		{app.ErrBudgetExceeded, http.StatusConflict, "budget_exhausted"},
		{app.Invalid("bad input"), http.StatusBadRequest, "validation_failed"},
		{errors.New("database secret"), http.StatusInternalServerError, "internal_error"},
	} {
		recorder := httptest.NewRecorder()
		writeProblem(recorder, test.err)
		expectRecordedProblem(t, recorder, test.status, test.code)
		if test.status == http.StatusInternalServerError {
			if recorder.Header().Get("X-Correlation-ID") == "" ||
				strings.Contains(recorder.Body.String(), "database secret") {
				t.Fatal("internal error leaked details or omitted correlation ID")
			}
		}
	}
}

func TestMutationAndResponseHelpers(t *testing.T) {
	t.Parallel()
	for method, expected := range map[string]bool{
		http.MethodGet: false, http.MethodHead: false, http.MethodPost: true,
		http.MethodPut: true, http.MethodPatch: true, http.MethodDelete: true,
	} {
		if got := isMutation(method); got != expected {
			t.Fatalf("method=%s mutation=%v", method, got)
		}
	}
	if statusFor(true) != "ok" || statusFor(false) != "degraded" {
		t.Fatal("health status mapping changed")
	}
	if version := resourceVersion(json.RawMessage(`{"version":7}`)); version != 7 {
		t.Fatalf("resource version=%d", version)
	}
	if version := resourceVersion(json.RawMessage(`not-json`)); version != 0 {
		t.Fatalf("invalid resource version=%d", version)
	}

	recorder := httptest.NewRecorder()
	writeRawJSON(recorder, http.StatusOK, []byte(`not-json`))
	expectRecordedProblem(t, recorder, http.StatusInternalServerError, "invalid_server_json")

	recorder = httptest.NewRecorder()
	writeRawJSON(recorder, http.StatusCreated, []byte(`{"safe":"<script>"}`))
	if recorder.Code != http.StatusCreated || recorder.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("raw JSON response=%d headers=%v", recorder.Code, recorder.Header())
	}

	request := httptest.NewRequest(http.MethodGet, "/?project_id=project-one", nil)
	recorder = httptest.NewRecorder()
	if project, ok := requireProject(recorder, request); !ok || project != "project-one" {
		t.Fatalf("project=%q ok=%v", project, ok)
	}
	request = httptest.NewRequest(http.MethodGet, "/", nil)
	recorder = httptest.NewRecorder()
	if _, ok := requireProject(recorder, request); ok {
		t.Fatal("missing project accepted")
	}
	expectRecordedProblem(t, recorder, http.StatusBadRequest, "project_required")

	request.Header.Set("Idempotency-Key", " key-one ")
	recorder = httptest.NewRecorder()
	if key, ok := requireIdempotency(recorder, request); !ok || key != "key-one" {
		t.Fatalf("idempotency=%q ok=%v", key, ok)
	}
	request.Header.Set("Idempotency-Key", strings.Repeat("x", 201))
	recorder = httptest.NewRecorder()
	if _, ok := requireIdempotency(recorder, request); ok {
		t.Fatal("oversize idempotency key accepted")
	}
	expectRecordedProblem(t, recorder, http.StatusBadRequest, "idempotency_key_required")

	user := auth.User{ID: "user", Capabilities: []string{"resource:read"}}
	session := auth.Session{User: user}
	request = httptest.NewRequest(http.MethodGet, "/", nil).WithContext(context.WithValue(context.Background(), sessionKey, session))
	if currentSession(request).User.ID != "user" || !currentActor(request).Can("resource:read") {
		t.Fatal("session context did not produce actor")
	}
	adapterActor := app.Actor{ID: "adapter", Capabilities: map[string]struct{}{"event:write": {}}}
	request = request.WithContext(context.WithValue(request.Context(), actorKey, adapterActor))
	if actor := currentActor(request); actor.ID != "adapter" || !actor.Can("event:write") {
		t.Fatalf("explicit actor=%+v", actor)
	}

	pageOutput, err := mcpListPage(app.Page{
		Items: []json.RawMessage{json.RawMessage(`{"id":"one"}`)}, NextCursor: "next",
	}, nil)
	if err != nil || len(pageOutput.Items) != 1 || pageOutput.NextCursor != "next" {
		t.Fatalf("MCP page=%+v err=%v", pageOutput, err)
	}
	if _, err := mcpListPage(app.Page{}, errors.New("list failed")); err == nil {
		t.Fatal("MCP list error discarded")
	}
	if _, err := mcpListPage(app.Page{Items: []json.RawMessage{json.RawMessage(`bad`)}}, nil); err == nil {
		t.Fatal("invalid MCP list JSON accepted")
	}
	mutationOutput, err := mcpMutationResult(app.CommandResult{
		Resource: json.RawMessage(`{"id":"one"}`),
		Event:    domain.Event{Cursor: 7},
	}, nil)
	if err != nil || mutationOutput.Cursor != 7 {
		t.Fatalf("MCP mutation=%+v err=%v", mutationOutput, err)
	}
	if _, err := mcpMutationResult(app.CommandResult{}, errors.New("command failed")); err == nil {
		t.Fatal("MCP command error discarded")
	}
	if _, err := mcpMutationResult(app.CommandResult{Resource: json.RawMessage(`bad`)}, nil); err == nil {
		t.Fatal("invalid MCP mutation JSON accepted")
	}

	recorder = httptest.NewRecorder()
	writeResult(recorder, app.CommandResult{
		Resource: json.RawMessage(`{"id":"one","version":4}`), Replayed: true,
	}, nil, http.StatusCreated)
	if recorder.Code != http.StatusCreated || recorder.Header().Get("ETag") != "4" ||
		recorder.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatalf("command result status=%d headers=%v", recorder.Code, recorder.Header())
	}
	recorder = httptest.NewRecorder()
	writeResult(recorder, app.CommandResult{}, app.ErrDenied, http.StatusCreated)
	expectRecordedProblem(t, recorder, http.StatusForbidden, "capability_denied")
}

func expectRecordedProblem(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var problem Problem
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if problem.Code != code || problem.Status != status {
		t.Fatalf("problem=%+v", problem)
	}
}
