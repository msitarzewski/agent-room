//go:build integration

package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/msitarzewski/agent-room/internal/app"
	"github.com/msitarzewski/agent-room/internal/artifacts"
	"github.com/msitarzewski/agent-room/internal/auth"
	"github.com/msitarzewski/agent-room/internal/domain"
	"github.com/msitarzewski/agent-room/internal/postgres"
	"github.com/msitarzewski/agent-room/internal/realtime"
)

const httpAPITestSuiteLock = int64(671327102)

func TestServerAndMCPIntegrationBoundaries(t *testing.T) {
	databaseURL := os.Getenv("AGENTROOM_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("AGENTROOM_TEST_DATABASE_URL is required for HTTP API integration tests")
	}
	ctx := context.Background()
	repository, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(repository.Close)
	lockConnection, err := repository.Pool().Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lockConnection.Exec(ctx, "SELECT pg_advisory_lock($1)", httpAPITestSuiteLock); err != nil {
		lockConnection.Release()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = lockConnection.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", httpAPITestSuiteLock)
		lockConnection.Release()
	})
	if err := postgres.Migrate(ctx, repository.Pool()); err != nil {
		t.Fatal(err)
	}
	var databaseName string
	if err := repository.Pool().QueryRow(ctx, "SELECT current_database()").Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(databaseName, "_test") {
		t.Fatalf("refusing HTTP API setup against non-test database %q", databaseName)
	}
	reset := `TRUNCATE brief_acknowledgements,brief_cursors,oidc_states,web_sessions,service_tokens,
		project_memberships,user_accounts,run_control_outbox,event_outbox,command_results,
		audit_records,events,resources,projects RESTART IDENTITY CASCADE`
	if _, err := repository.Pool().Exec(ctx, reset); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = repository.Pool().Exec(context.Background(), reset) })

	const projectID = "httpapi-integration"
	const origin = "http://127.0.0.1"
	passwordHash, err := auth.HashPassword("httpapi integration password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Pool().Exec(ctx, "INSERT INTO projects(id,name) VALUES($1,$2)", projectID, "HTTP API Integration"); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Pool().Exec(ctx, `
		INSERT INTO user_accounts(id,username,password_hash,display_name,capabilities)
		VALUES('httpapi-operator','operator',$1,'HTTP API Operator',ARRAY[
			'project:all','resource:read','resource:write','event:read','overview:read',
			'task:transition','attention:manage','claim:review','approval:request',
			'approval:decide','run:pause','run:cancel'
		]::text[])`,
		passwordHash,
	); err != nil {
		t.Fatal(err)
	}

	authManager, err := auth.New(
		ctx, repository.Pool(), []byte("httpapi integration session secret"),
		false, origin, "", "", "", "",
	)
	if err != nil {
		t.Fatal(err)
	}
	service := app.NewService(repository, nil)
	setupActor := app.Actor{ID: "setup", Capabilities: map[string]struct{}{"resource:write": {}}}
	for index, resource := range []struct {
		kind  domain.ResourceType
		value any
	}{
		{domain.ResourceTask, domain.Task{Base: domain.Base{ID: "task-http"}, Title: "HTTP task", Status: "ready", Priority: "normal", Dependencies: []string{}}},
		{domain.ResourceTask, domain.Task{Base: domain.Base{ID: "task-mcp"}, Title: "MCP task", Status: "ready", Priority: "normal", Dependencies: []string{}}},
		{domain.ResourceRun, domain.Run{Base: domain.Base{ID: "run-http"}, AgentID: "agent-http", Status: "running", Capabilities: []string{"run:pause", "run:cancel"}}},
		{domain.ResourceAttention, domain.Attention{Base: domain.Base{ID: "attention-http"}, Kind: "review", Severity: "high", Title: "Review", Status: "open"}},
		{domain.ResourceClaim, domain.Claim{Base: domain.Base{ID: "claim-http"}, TaskID: "task-http", AgentID: "agent-http", Status: "pending"}},
	} {
		if _, err := service.Create(ctx, setupActor, resource.kind, projectID, "seed-"+strconv.Itoa(index), resource.value, "test"); err != nil {
			t.Fatal(err)
		}
	}
	store, err := artifacts.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	server := New(
		service, authManager, realtime.NewHub(), nil, store, 1<<20, 1<<20,
		origin, []string{origin}, []string{"192.0.2.0/24"}, true,
	)
	public := httptest.NewServer(server.Handler())
	defer public.Close()
	adapter := httptest.NewServer(server.AdapterHandler())
	defer adapter.Close()

	login := rawRequest(t, http.MethodPost, public.URL+"/api/v1/auth/dev-login", strings.NewReader(
		`{"username":"operator","password":"httpapi integration password"}`,
	), map[string]string{"Content-Type": "application/json"}, nil)
	if login.StatusCode != http.StatusOK {
		t.Fatalf("dev login status=%d body=%s", login.StatusCode, readBody(login))
	}
	var loggedIn auth.Session
	if err := json.NewDecoder(login.Body).Decode(&loggedIn); err != nil {
		t.Fatal(err)
	}
	var sessionCookie *http.Cookie
	for _, cookie := range login.Cookies() {
		if cookie.Name == auth.DevCookieName {
			sessionCookie = cookie
		}
	}
	if sessionCookie == nil || loggedIn.CSRFToken == "" {
		t.Fatalf("login cookie=%v session=%+v", sessionCookie, loggedIn)
	}
	mutationHeaders := func(idempotency string, version int64) map[string]string {
		headers := map[string]string{
			"Content-Type": "application/json", "Origin": origin,
			"X-CSRF-Token": loggedIn.CSRFToken, "Idempotency-Key": idempotency,
		}
		if version > 0 {
			headers["If-Match"] = strconv.FormatInt(version, 10)
		}
		return headers
	}
	api := func(method, path string, body io.Reader, headers map[string]string) *http.Response {
		return rawRequest(t, method, public.URL+path, body, headers, sessionCookie)
	}

	t.Run("web handlers traverse success and validation branches", func(t *testing.T) {
		for _, path := range []string{
			"/api/v1/projects",
			"/api/v1/brief?project_id=" + projectID,
			"/api/v1/events?project_id=" + projectID + "&cursor=bad&limit=bad",
		} {
			response := api(http.MethodGet, path, nil, nil)
			if response.StatusCode != http.StatusOK {
				t.Fatalf("%s status=%d body=%s", path, response.StatusCode, readBody(response))
			}
			_ = response.Body.Close()
		}
		invalidBrief := api(http.MethodGet, "/api/v1/brief?project_id="+projectID+"&after=bad", nil, nil)
		expectHTTPProblem(t, invalidBrief, http.StatusBadRequest, "invalid_cursor")
		sessionResponse := api(http.MethodGet, "/api/v1/auth/session", nil, nil)
		if sessionResponse.StatusCode != http.StatusOK {
			t.Fatalf("session status=%d body=%s", sessionResponse.StatusCode, readBody(sessionResponse))
		}
		_ = sessionResponse.Body.Close()
		overview := api(http.MethodGet, "/api/v1/overview?project_id="+projectID, nil, nil)
		if overview.StatusCode != http.StatusOK {
			t.Fatalf("overview status=%d body=%s", overview.StatusCode, readBody(overview))
		}
		_ = overview.Body.Close()
		missingProject := api(http.MethodGet, "/api/v1/tasks", nil, nil)
		expectHTTPProblem(t, missingProject, http.StatusBadRequest, "project_required")
		task := api(http.MethodGet, "/api/v1/tasks/task-http?project_id="+projectID, nil, nil)
		if task.StatusCode != http.StatusOK || task.Header.Get("ETag") != "1" {
			t.Fatalf("task status=%d etag=%q body=%s", task.StatusCode, task.Header.Get("ETag"), readBody(task))
		}
		_ = task.Body.Close()

		briefResponse := api(http.MethodGet, "/api/v1/brief?project_id="+projectID, nil, nil)
		var briefDocument map[string]any
		if err := json.NewDecoder(briefResponse.Body).Decode(&briefDocument); err != nil {
			t.Fatal(err)
		}
		_ = briefResponse.Body.Close()
		through := int64(briefDocument["through_cursor"].(float64))
		acknowledgeBody := fmt.Sprintf(`{"expected_cursor":0,"through_cursor":%d}`, through)
		acknowledged := api(http.MethodPost, "/api/v1/brief/acknowledge?project_id="+projectID,
			strings.NewReader(acknowledgeBody), mutationHeaders("brief-ack", 0))
		if acknowledged.StatusCode != http.StatusOK {
			t.Fatalf("brief acknowledge status=%d body=%s", acknowledged.StatusCode, readBody(acknowledged))
		}
		_ = acknowledged.Body.Close()
		replayed := api(http.MethodPost, "/api/v1/brief/acknowledge?project_id="+projectID,
			strings.NewReader(acknowledgeBody), mutationHeaders("brief-ack", 0))
		if replayed.StatusCode != http.StatusOK || replayed.Header.Get("Idempotency-Replayed") != "true" {
			t.Fatalf("brief replay status=%d header=%q body=%s", replayed.StatusCode, replayed.Header.Get("Idempotency-Replayed"), readBody(replayed))
		}
		_ = replayed.Body.Close()
		invalidAcknowledgement := api(http.MethodPost, "/api/v1/brief/acknowledge?project_id="+projectID,
			strings.NewReader(`not-json`), mutationHeaders("brief-invalid", 0))
		expectHTTPProblem(t, invalidAcknowledgement, http.StatusBadRequest, "validation_failed")

		chat := api(http.MethodPost, "/api/v1/chat/messages?project_id="+projectID,
			strings.NewReader(`{"body":"integration message"}`), mutationHeaders("chat-httpapi", 0))
		if chat.StatusCode != http.StatusCreated {
			t.Fatalf("chat status=%d body=%s", chat.StatusCode, readBody(chat))
		}
		_ = chat.Body.Close()
		chatConflict := api(http.MethodPost, "/api/v1/chat/messages?project_id="+projectID,
			strings.NewReader(`{"body":"integration message"}`), mutationHeaders("chat-httpapi", 0))
		expectHTTPProblem(t, chatConflict, http.StatusConflict, "idempotency_conflict")
		emptyChat := api(http.MethodPost, "/api/v1/chat/messages?project_id="+projectID,
			strings.NewReader(`{"body":" "}`), mutationHeaders("chat-empty", 0))
		expectHTTPProblem(t, emptyChat, http.StatusBadRequest, "validation_failed")

		transition := api(http.MethodPost, "/api/v1/tasks/task-http/transition?project_id="+projectID,
			strings.NewReader(`{"status":"working","reason":"test"}`), mutationHeaders("task-working", 1))
		if transition.StatusCode != http.StatusOK {
			t.Fatalf("transition status=%d body=%s", transition.StatusCode, readBody(transition))
		}
		_ = transition.Body.Close()
		missingVersion := api(http.MethodPost, "/api/v1/tasks/task-http/transition?project_id="+projectID,
			strings.NewReader(`{"status":"review"}`), mutationHeaders("missing-version", 0))
		expectHTTPProblem(t, missingVersion, http.StatusPreconditionRequired, "version_required")
		badTransition := api(http.MethodPost, "/api/v1/tasks/task-http/transition?project_id="+projectID,
			strings.NewReader(`not-json`), mutationHeaders("task-invalid", 2))
		expectHTTPProblem(t, badTransition, http.StatusBadRequest, "validation_failed")
		badRunAction := api(http.MethodPost, "/api/v1/runs/run-http/actions?project_id="+projectID,
			strings.NewReader(`not-json`), mutationHeaders("run-invalid", 1))
		expectHTTPProblem(t, badRunAction, http.StatusBadRequest, "validation_failed")

		attention := api(http.MethodPost, "/api/v1/attention/attention-http/acknowledge?project_id="+projectID,
			nil, mutationHeaders("attention-ack", 1))
		if attention.StatusCode != http.StatusOK {
			t.Fatalf("attention status=%d body=%s", attention.StatusCode, readBody(attention))
		}
		_ = attention.Body.Close()
		attentionReplay := api(http.MethodPost, "/api/v1/attention/attention-http/acknowledge?project_id="+projectID,
			nil, mutationHeaders("attention-ack", 1))
		if attentionReplay.StatusCode != http.StatusOK || attentionReplay.Header.Get("Idempotency-Replayed") != "true" {
			t.Fatalf("attention replay status=%d replay=%q body=%s", attentionReplay.StatusCode, attentionReplay.Header.Get("Idempotency-Replayed"), readBody(attentionReplay))
		}
		_ = attentionReplay.Body.Close()
		resolve := api(http.MethodPost, "/api/v1/attention/attention-http/resolve?project_id="+projectID,
			nil, mutationHeaders("attention-resolve", 2))
		if resolve.StatusCode != http.StatusOK {
			t.Fatalf("resolve status=%d body=%s", resolve.StatusCode, readBody(resolve))
		}
		_ = resolve.Body.Close()

		review := api(http.MethodPost, "/api/v1/claims/claim-http/review?project_id="+projectID,
			strings.NewReader(`{"decision":"rejected","note":"test"}`), mutationHeaders("claim-review", 1))
		if review.StatusCode != http.StatusOK {
			t.Fatalf("claim review status=%d body=%s", review.StatusCode, readBody(review))
		}
		_ = review.Body.Close()
		badReview := api(http.MethodPost, "/api/v1/claims/claim-http/review?project_id="+projectID,
			strings.NewReader(`not-json`), mutationHeaders("claim-invalid", 2))
		expectHTTPProblem(t, badReview, http.StatusBadRequest, "validation_failed")

		approval := api(http.MethodPost, "/api/v1/approvals?project_id="+projectID,
			strings.NewReader(`{"run_id":"run-http","action":"cancel","expected_target_version":1,"expires_in_seconds":600}`),
			mutationHeaders("approval-request", 0))
		if approval.StatusCode != http.StatusCreated {
			t.Fatalf("approval status=%d body=%s", approval.StatusCode, readBody(approval))
		}
		var approvalDocument map[string]any
		if err := json.NewDecoder(approval.Body).Decode(&approvalDocument); err != nil {
			t.Fatal(err)
		}
		_ = approval.Body.Close()
		approvalID := approvalDocument["id"].(string)
		decision := api(http.MethodPost, "/api/v1/approvals/"+approvalID+"/decision?project_id="+projectID,
			strings.NewReader(`{"decision":"rejected","note":"test"}`), mutationHeaders("approval-decision", 1))
		if decision.StatusCode != http.StatusOK {
			t.Fatalf("approval decision status=%d body=%s", decision.StatusCode, readBody(decision))
		}
		_ = decision.Body.Close()
		badDecision := api(http.MethodPost, "/api/v1/approvals/"+approvalID+"/decision?project_id="+projectID,
			strings.NewReader(`not-json`), mutationHeaders("approval-decision-invalid", 2))
		expectHTTPProblem(t, badDecision, http.StatusBadRequest, "validation_failed")

		badApproval := api(http.MethodPost, "/api/v1/approvals?project_id="+projectID,
			strings.NewReader(`not-json`), mutationHeaders("approval-invalid", 0))
		expectHTTPProblem(t, badApproval, http.StatusBadRequest, "validation_failed")
		badArtifact := api(http.MethodPost, "/api/v1/artifacts?project_id="+projectID,
			strings.NewReader("content"), mutationHeaders("artifact-bad", 0))
		expectHTTPProblem(t, badArtifact, http.StatusBadRequest, "validation_failed")
		artifactHeaders := mutationHeaders("artifact-good", 0)
		artifactHeaders["Content-Type"] = "text/plain; charset=utf-8"
		artifactHeaders["X-Artifact-Name"] = "evidence.txt"
		uploaded := api(http.MethodPost, "/api/v1/artifacts?project_id="+projectID,
			strings.NewReader("immutable evidence"), artifactHeaders)
		if uploaded.StatusCode != http.StatusCreated {
			t.Fatalf("artifact upload status=%d body=%s", uploaded.StatusCode, readBody(uploaded))
		}
		var artifactDocument map[string]any
		if err := json.NewDecoder(uploaded.Body).Decode(&artifactDocument); err != nil {
			t.Fatal(err)
		}
		_ = uploaded.Body.Close()
		artifactID := artifactDocument["id"].(string)
		downloaded := api(http.MethodGet, "/api/v1/artifacts/"+artifactID+"/content?project_id="+projectID, nil, nil)
		if downloaded.StatusCode != http.StatusOK || downloaded.Header.Get("Content-Disposition") == "" ||
			readBody(downloaded) != "immutable evidence" {
			t.Fatalf("artifact download status=%d headers=%v", downloaded.StatusCode, downloaded.Header)
		}
		missingArtifact := api(http.MethodGet, "/api/v1/artifacts/missing/content?project_id="+projectID, nil, nil)
		expectHTTPProblem(t, missingArtifact, http.StatusNotFound, "not_found")
		brokenArtifact, err := service.Create(ctx, setupActor, domain.ResourceArtifact, projectID, "broken-artifact",
			domain.Artifact{
				Base: domain.Base{ID: "artifact-broken"}, Name: "broken.txt", MediaType: "text/plain",
				Digest: "sha256:" + strings.Repeat("0", 64), URI: "artifact://missing",
			}, "test")
		if err != nil || len(brokenArtifact.Resource) == 0 {
			t.Fatal(err)
		}
		verificationFailure := api(http.MethodGet, "/api/v1/artifacts/artifact-broken/content?project_id="+projectID, nil, nil)
		expectHTTPProblem(t, verificationFailure, http.StatusConflict, "artifact_verification_failed")
		invalidNameHeaders := mutationHeaders("artifact-name", 0)
		invalidNameHeaders["Content-Type"] = "text/plain"
		invalidNameHeaders["X-Artifact-Name"] = "../escape"
		invalidName := api(http.MethodPost, "/api/v1/artifacts?project_id="+projectID,
			strings.NewReader("content"), invalidNameHeaders)
		expectHTTPProblem(t, invalidName, http.StatusBadRequest, "validation_failed")
		oversizeHeaders := mutationHeaders("artifact-oversize", 0)
		oversizeHeaders["Content-Type"] = "text/plain"
		oversizeHeaders["X-Artifact-Name"] = "large.txt"
		oversize := api(http.MethodPost, "/api/v1/artifacts?project_id="+projectID,
			strings.NewReader(strings.Repeat("x", (1<<20)+1)), oversizeHeaders)
		expectHTTPProblem(t, oversize, http.StatusBadRequest, "validation_failed")

		health := api(http.MethodGet, "/api/v1/health/components?project_id="+projectID, nil, nil)
		if health.StatusCode != http.StatusOK {
			t.Fatalf("component health status=%d body=%s", health.StatusCode, readBody(health))
		}
		_ = health.Body.Close()

		stream := api(http.MethodGet, "/api/v1/stream?project_id="+projectID+"&after=bad", nil, nil)
		expectHTTPProblem(t, stream, http.StatusBadRequest, "invalid_cursor")
		noUpgrade := api(http.MethodGet, "/api/v1/stream?project_id="+projectID, nil, nil)
		if noUpgrade.StatusCode == http.StatusInternalServerError {
			t.Fatalf("websocket negotiation leaked server error: %s", readBody(noUpgrade))
		}
		_ = noUpgrade.Body.Close()
		websocketContext, cancelWebsocket := context.WithTimeout(ctx, 5*time.Second)
		defer cancelWebsocket()
		streamURL := "ws" + strings.TrimPrefix(public.URL, "http") +
			"/api/v1/stream?project_id=" + projectID
		connection, response, err := websocket.Dial(websocketContext, streamURL, &websocket.DialOptions{
			HTTPHeader: http.Header{
				"Cookie": {sessionCookie.String()},
				"Origin": {origin},
			},
		})
		if err != nil {
			if response != nil {
				t.Fatalf("websocket dial status=%d body=%s err=%v", response.StatusCode, readBody(response), err)
			}
			t.Fatal(err)
		}
		_, eventMessage, err := connection.Read(websocketContext)
		if err != nil || len(eventMessage) == 0 {
			t.Fatalf("websocket replay=%q err=%v", eventMessage, err)
		}
		connection.CloseNow()

		loginRedirect := api(http.MethodGet, "/api/v1/auth/login", nil, nil)
		if loginRedirect.StatusCode != http.StatusInternalServerError {
			t.Fatalf("unconfigured OIDC login status=%d", loginRedirect.StatusCode)
		}
		_ = loginRedirect.Body.Close()
		callback := api(http.MethodGet, "/api/v1/auth/callback?state=x&code=y", nil, nil)
		if callback.StatusCode != http.StatusInternalServerError {
			t.Fatalf("unconfigured OIDC callback status=%d", callback.StatusCode)
		}
		_ = callback.Body.Close()
		wrongLogin := rawRequest(t, http.MethodPost, public.URL+"/api/v1/auth/dev-login",
			strings.NewReader(`{"username":"operator","password":"wrong password"}`),
			map[string]string{"Content-Type": "application/json"}, nil)
		expectHTTPProblem(t, wrongLogin, http.StatusUnauthorized, "authentication_required")
		invalidLogin := rawRequest(t, http.MethodPost, public.URL+"/api/v1/auth/dev-login",
			strings.NewReader(`not-json`), map[string]string{"Content-Type": "application/json"}, nil)
		expectHTTPProblem(t, invalidLogin, http.StatusBadRequest, "validation_failed")
	})

	t.Run("degraded and authorization boundaries remain explicit", func(t *testing.T) {
		degraded := httptest.NewServer(New(
			service, authManager, realtime.NewHub(), nil, nil, 1<<20, 1<<20,
			origin, []string{origin}, nil, true,
		).Handler())
		defer degraded.Close()
		degradedAPI := func(method, path string, body io.Reader, headers map[string]string) *http.Response {
			return rawRequest(t, method, degraded.URL+path, body, headers, sessionCookie)
		}
		health := degradedAPI(http.MethodGet, "/api/v1/health/components?project_id="+projectID, nil, nil)
		if health.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("degraded health status=%d body=%s", health.StatusCode, readBody(health))
		}
		_ = health.Body.Close()
		unavailableUpload := degradedAPI(http.MethodPost, "/api/v1/artifacts?project_id="+projectID,
			strings.NewReader("content"), mutationHeaders("unavailable-upload", 0))
		expectHTTPProblem(t, unavailableUpload, http.StatusServiceUnavailable, "artifact_store_unavailable")
		unavailableDownload := degradedAPI(http.MethodGet, "/api/v1/artifacts/missing/content?project_id="+projectID, nil, nil)
		expectHTTPProblem(t, unavailableDownload, http.StatusServiceUnavailable, "artifact_store_unavailable")

		productionMode := httptest.NewServer(New(
			service, authManager, realtime.NewHub(), nil, store, 1<<20, 1<<20,
			"https://agentroom.test", []string{origin}, nil, false,
		).Handler())
		defer productionMode.Close()
		notFound := rawRequest(t, http.MethodPost, productionMode.URL+"/api/v1/auth/dev-login",
			strings.NewReader(`{}`), mutationHeaders("prod-dev-login", 0), sessionCookie)
		if notFound.StatusCode != http.StatusNotFound {
			t.Fatalf("production dev-login status=%d body=%s", notFound.StatusCode, readBody(notFound))
		}
		_ = notFound.Body.Close()

		if _, err := repository.Pool().Exec(ctx, `
			INSERT INTO user_accounts(id,username,display_name,capabilities)
			VALUES('unscoped-user','unscoped','Unscoped',ARRAY['resource:read']::text[])`); err != nil {
			t.Fatal(err)
		}
		unscopedSession, unscopedToken, err := authManager.CreateSession(ctx, auth.User{
			ID: "unscoped-user", Username: "unscoped", DisplayName: "Unscoped",
			Capabilities: []string{"resource:read"},
		}, "")
		if err != nil || unscopedSession.CSRFToken == "" {
			t.Fatal(err)
		}
		unscopedCookie := &http.Cookie{Name: auth.DevCookieName, Value: unscopedToken}
		denied := rawRequest(t, http.MethodGet, public.URL+"/api/v1/tasks?project_id="+projectID, nil, nil, unscopedCookie)
		expectHTTPProblem(t, denied, http.StatusForbidden, "capability_denied")

		badCursor := api(http.MethodGet, "/api/v1/tasks?project_id="+projectID+"&cursor=not-a-cursor", nil, nil)
		expectHTTPProblem(t, badCursor, http.StatusInternalServerError, "internal_error")
	})

	t.Run("OIDC browser handlers complete state-bound login", func(t *testing.T) {
		signingKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		signer, err := jose.NewSigner(
			jose.SigningKey{
				Algorithm: jose.RS256,
				Key: jose.JSONWebKey{
					Key: signingKey, KeyID: "httpapi-oidc", Algorithm: string(jose.RS256), Use: "sig",
				},
			},
			(&jose.SignerOptions{}).WithType("JWT"),
		)
		if err != nil {
			t.Fatal(err)
		}
		var issuer, nonce string
		provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/openid-configuration":
				writeJSON(w, http.StatusOK, map[string]any{
					"issuer": issuer, "authorization_endpoint": issuer + "/authorize",
					"token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys",
					"id_token_signing_alg_values_supported": []string{"RS256"},
				})
			case "/keys":
				writeJSON(w, http.StatusOK, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
					Key: &signingKey.PublicKey, KeyID: "httpapi-oidc", Algorithm: string(jose.RS256), Use: "sig",
				}}})
			case "/token":
				idToken, signErr := jwt.Signed(signer).
					Claims(jwt.Claims{
						Issuer: issuer, Subject: "httpapi-oidc-user",
						Audience: jwt.Audience{"httpapi-client"},
						Expiry:   jwt.NewNumericDate(time.Now().Add(time.Hour)),
					}).
					Claims(map[string]any{
						"nonce": nonce, "email": "httpapi-oidc@agentroom.test", "name": "HTTP OIDC",
					}).
					Serialize()
				if signErr != nil {
					http.Error(w, "signing failed", http.StatusInternalServerError)
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{
					"access_token": "token", "token_type": "Bearer", "expires_in": 3600, "id_token": idToken,
				})
			default:
				http.NotFound(w, r)
			}
		}))
		defer provider.Close()
		issuer = provider.URL
		oidcManager, err := auth.New(
			ctx, repository.Pool(), []byte("httpapi OIDC handler session secret"),
			false, origin, issuer, "httpapi-client", "secret",
			origin+"/api/v1/auth/callback",
		)
		if err != nil {
			t.Fatal(err)
		}
		oidcServer := httptest.NewServer(New(
			service, oidcManager, realtime.NewHub(), nil, store, 1<<20, 1<<20,
			origin, []string{origin}, nil, true,
		).Handler())
		defer oidcServer.Close()
		noRedirect := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}}
		loginRequest, err := http.NewRequestWithContext(ctx, http.MethodGet,
			oidcServer.URL+"/api/v1/auth/login", nil)
		if err != nil {
			t.Fatal(err)
		}
		loginResponse, err := noRedirect.Do(loginRequest)
		if err != nil {
			t.Fatal(err)
		}
		defer loginResponse.Body.Close()
		if loginResponse.StatusCode != http.StatusFound {
			t.Fatalf("OIDC login status=%d body=%s", loginResponse.StatusCode, readBody(loginResponse))
		}
		authorizationURL, err := loginResponse.Location()
		if err != nil {
			t.Fatal(err)
		}
		state := authorizationURL.Query().Get("state")
		nonce = authorizationURL.Query().Get("nonce")
		var browserCookie *http.Cookie
		for _, cookie := range loginResponse.Cookies() {
			if cookie.Name == auth.DevOIDCCookieName {
				browserCookie = cookie
			}
		}
		if state == "" || nonce == "" || browserCookie == nil {
			t.Fatalf("OIDC redirect=%s cookies=%v", authorizationURL, loginResponse.Cookies())
		}
		callbackRequest, err := http.NewRequestWithContext(ctx, http.MethodGet,
			oidcServer.URL+"/api/v1/auth/callback?state="+state+"&code=valid", nil)
		if err != nil {
			t.Fatal(err)
		}
		callbackRequest.AddCookie(browserCookie)
		callbackResponse, err := noRedirect.Do(callbackRequest)
		if err != nil {
			t.Fatal(err)
		}
		defer callbackResponse.Body.Close()
		if callbackResponse.StatusCode != http.StatusSeeOther ||
			callbackResponse.Header.Get("Location") != "/" {
			t.Fatalf("OIDC callback status=%d location=%q body=%s", callbackResponse.StatusCode, callbackResponse.Header.Get("Location"), readBody(callbackResponse))
		}
		foundSession := false
		for _, cookie := range callbackResponse.Cookies() {
			if cookie.Name == auth.DevCookieName && cookie.Value != "" {
				foundSession = true
			}
		}
		if !foundSession {
			t.Fatal("OIDC callback omitted session cookie")
		}
	})

	allCapabilities := []string{
		"resource:read", "resource:write", "event:ingest", "task:read", "task:claim", "task:review",
		"attention:read", "attention:write", "run:read", "situation:read", "evidence:read",
		"evidence:write", "artifact:read", "approval:read",
	}
	serviceToken, err := auth.CreateServiceToken(
		ctx, repository.Pool(), "httpapi-service", "HTTP API service", "agent:mcp",
		[]string{projectID}, allCapabilities, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	serviceHeaders := map[string]string{"Authorization": "Bearer " + serviceToken}
	restrictedToken, err := auth.CreateServiceToken(
		ctx, repository.Pool(), "httpapi-restricted", "Restricted service", "agent:restricted",
		[]string{projectID}, []string{"event:ingest"}, nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("adapter ingest and MCP tools retain service identity", func(t *testing.T) {
		missingProject := rawRequest(t, http.MethodPost, adapter.URL+"/api/v1/ingest",
			strings.NewReader(`{"events":[]}`), map[string]string{"Authorization": "Bearer " + serviceToken, "Content-Type": "application/json"}, nil)
		expectHTTPProblem(t, missingProject, http.StatusBadRequest, "project_required")
		badToken := rawRequest(t, http.MethodPost, adapter.URL+"/api/v1/ingest?project_id="+projectID,
			strings.NewReader(`{"events":[]}`), map[string]string{"Authorization": "Bearer invalid", "Content-Type": "application/json"}, nil)
		expectHTTPProblem(t, badToken, http.StatusUnauthorized, "authentication_required")
		mismatch := rawRequest(t, http.MethodPost, adapter.URL+"/api/v1/ingest?project_id="+projectID,
			strings.NewReader(`{"events":[{"project_id":"other"}]}`),
			map[string]string{"Authorization": "Bearer " + serviceToken, "Content-Type": "application/json"}, nil)
		expectHTTPProblem(t, mismatch, http.StatusBadRequest, "project_mismatch")
		accepted := rawRequest(t, http.MethodPost, adapter.URL+"/api/v1/ingest?project_id="+projectID,
			strings.NewReader(`{"events":[]}`),
			map[string]string{"Authorization": "Bearer " + serviceToken, "Content-Type": "application/json"}, nil)
		if accepted.StatusCode != http.StatusAccepted {
			t.Fatalf("ingest status=%d body=%s", accepted.StatusCode, readBody(accepted))
		}
		_ = accepted.Body.Close()

		for index, call := range []struct {
			name string
			args string
		}{
			{"list_tasks", `{"limit":10}`},
			{"list_attention", `{"limit":10}`},
			{"list_runs", `{"limit":10}`},
			{"list_situations", `{"limit":10}`},
			{"list_evidence", `{"limit":10}`},
			{"list_artifacts", `{"limit":10}`},
			{"list_approvals", `{"limit":10}`},
			{"send_chat_message", `{"body":"MCP integration","idempotency_key":"mcp-chat"}`},
			{"transition_task", `{"task_id":"task-mcp","status":"working","expected_version":1,"idempotency_key":"mcp-transition"}`},
			{"request_attention", `{"kind":"review","severity":"high","title":"MCP attention","idempotency_key":"mcp-attention"}`},
			{"post_evidence", `{"task_id":"task-mcp","kind":"test","summary":"MCP evidence","source_system":"integration","idempotency_key":"mcp-evidence"}`},
			{"claim_task", `{"task_id":"task-mcp","expected_version":2,"idempotency_key":"mcp-claim"}`},
			{"request_review", `{"task_id":"task-mcp","expected_version":3,"reason":"ready","idempotency_key":"mcp-review"}`},
		} {
			response := mcpCall(t, adapter.URL+"/api/v1/mcp?project_id="+projectID, serviceHeaders, index+1, call.name, call.args)
			if response.StatusCode != http.StatusOK {
				t.Fatalf("MCP %s status=%d body=%s", call.name, response.StatusCode, readBody(response))
			}
			body := readBody(response)
			if !strings.Contains(body, `"result"`) {
				t.Fatalf("MCP %s response=%s", call.name, body)
			}
		}
		for index, name := range []string{
			"list_tasks", "list_attention", "list_runs", "list_situations",
			"list_evidence", "list_artifacts", "list_approvals",
			"request_attention", "post_evidence", "claim_task", "request_review",
		} {
			response := mcpCall(
				t, adapter.URL+"/api/v1/mcp?project_id="+projectID,
				map[string]string{"Authorization": "Bearer " + restrictedToken},
				100+index, name, `{}`,
			)
			if response.StatusCode != http.StatusOK {
				t.Fatalf("restricted MCP %s status=%d body=%s", name, response.StatusCode, readBody(response))
			}
			if body := readBody(response); !strings.Contains(body, `"isError":true`) {
				t.Fatalf("restricted MCP %s unexpectedly succeeded: %s", name, body)
			}
		}
	})

	t.Run("bounded listener limits fail closed", func(t *testing.T) {
		exhaustBucket := func(key string) {
			t.Helper()
			server.limits.mu.Lock()
			defer server.limits.mu.Unlock()
			current, ok := server.limits.buckets[key]
			if !ok {
				current = bucket{entry: server.limits.lru.PushBack(key)}
			}
			current.tokens = -100
			current.last = server.limits.now()
			server.limits.buckets[key] = current
		}
		server.limits.mu.Lock()
		server.limits.sockets["httpapi-operator:"+projectID] = 5
		server.limits.mu.Unlock()
		streamLimited := api(http.MethodGet, "/api/v1/stream?project_id="+projectID, nil, nil)
		expectHTTPProblem(t, streamLimited, http.StatusTooManyRequests, "websocket_limit")

		exhaustBucket("artifact-download:httpapi-operator:" + projectID)
		downloadLimited := api(http.MethodGet, "/api/v1/artifacts/missing/content?project_id="+projectID, nil, nil)
		expectHTTPProblem(t, downloadLimited, http.StatusTooManyRequests, "rate_limited")

		exhaustBucket("actor:httpapi-operator:" + projectID)
		actorLimited := api(http.MethodGet, "/api/v1/tasks?project_id="+projectID, nil, nil)
		expectHTTPProblem(t, actorLimited, http.StatusTooManyRequests, "rate_limited")
		server.limits.mu.Lock()
		delete(server.limits.buckets, "actor:httpapi-operator:"+projectID)
		server.limits.mu.Unlock()

		exhaustBucket("service:httpapi-service:" + projectID)
		adapterLimited := rawRequest(t, http.MethodPost, adapter.URL+"/api/v1/ingest?project_id="+projectID,
			strings.NewReader(`{"events":[]}`),
			map[string]string{"Authorization": "Bearer " + serviceToken, "Content-Type": "application/json"}, nil)
		expectHTTPProblem(t, adapterLimited, http.StatusTooManyRequests, "rate_limited")

		exhaustBucket("login:127.0.0.1")
		loginLimited := rawRequest(t, http.MethodGet, public.URL+"/api/v1/auth/login", nil, nil, nil)
		expectHTTPProblem(t, loginLimited, http.StatusTooManyRequests, "rate_limited")
	})

	t.Run("logout revokes the browser session", func(t *testing.T) {
		logout := api(http.MethodPost, "/api/v1/auth/logout", nil, mutationHeaders("logout", 0))
		if logout.StatusCode != http.StatusNoContent {
			t.Fatalf("logout status=%d body=%s", logout.StatusCode, readBody(logout))
		}
		_ = logout.Body.Close()
		after := api(http.MethodGet, "/api/v1/projects", nil, nil)
		expectHTTPProblem(t, after, http.StatusUnauthorized, "authentication_required")

		server.limits.mu.Lock()
		ipKey := "ip:127.0.0.1"
		current := server.limits.buckets[ipKey]
		current.tokens = -100
		current.last = server.limits.now()
		server.limits.buckets[ipKey] = current
		server.limits.mu.Unlock()
		ipLimited := rawRequest(t, http.MethodGet, public.URL+"/healthz", nil, nil, nil)
		expectHTTPProblem(t, ipLimited, http.StatusTooManyRequests, "rate_limited")
	})
}

func rawRequest(t *testing.T, method, target string, body io.Reader, headers map[string]string, cookie *http.Cookie) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), method, target, body)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func mcpCall(t *testing.T, endpoint string, headers map[string]string, id int, name, arguments string) *http.Response {
	t.Helper()
	body := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":%q,"arguments":%s}}`,
		id, name, arguments,
	)
	requestHeaders := map[string]string{
		"Authorization": headers["Authorization"],
		"Content-Type":  "application/json",
		"Accept":        "application/json, text/event-stream",
	}
	return rawRequest(t, http.MethodPost, endpoint, bytes.NewBufferString(body), requestHeaders, nil)
}

func expectHTTPProblem(t *testing.T, response *http.Response, status int, code string) {
	t.Helper()
	defer response.Body.Close()
	if response.StatusCode != status {
		t.Fatalf("status=%d body=%s", response.StatusCode, readBody(response))
	}
	var problem Problem
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatal(err)
	}
	if problem.Code != code {
		t.Fatalf("problem=%+v", problem)
	}
}

func readBody(response *http.Response) string {
	value, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	return string(value)
}
