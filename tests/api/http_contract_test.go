//go:build integration

package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/msitarzewski/agent-room/internal/app"
	"github.com/msitarzewski/agent-room/internal/artifacts"
	"github.com/msitarzewski/agent-room/internal/auth"
	"github.com/msitarzewski/agent-room/internal/domain"
	"github.com/msitarzewski/agent-room/internal/httpapi"
	"github.com/msitarzewski/agent-room/internal/postgres"
	"github.com/msitarzewski/agent-room/internal/realtime"
)

const (
	projectID     = "http-contract-project"
	allowedOrigin = "https://agentroom.test"
	testSuiteLock = int64(671327099)
)

type authenticatedClient struct {
	baseURL string
	token   string
	csrf    string
}

func (c authenticatedClient) request(
	t *testing.T,
	method string,
	path string,
	body io.Reader,
	headers map[string]string,
) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), method, c.baseURL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if c.token != "" {
		request.AddCookie(&http.Cookie{Name: auth.DevCookieName, Value: c.token})
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	return response
}

func mutationHeaders(session authenticatedClient, idempotency string, version int) map[string]string {
	headers := map[string]string{
		"Content-Type":    "application/json",
		"Origin":          allowedOrigin,
		"X-CSRF-Token":    session.csrf,
		"Idempotency-Key": idempotency,
	}
	if version > 0 {
		headers["If-Match"] = strconv.Itoa(version)
	}
	return headers
}

func decodeObject(t *testing.T, response *http.Response) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func expectProblem(t *testing.T, response *http.Response, status int, code string) {
	t.Helper()
	if response.StatusCode != status {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
	if contentType := response.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/problem+json") {
		t.Fatalf("content-type=%q", contentType)
	}
	problem := decodeObject(t, response)
	if problem["code"] != code || int(problem["status"].(float64)) != status {
		t.Fatalf("problem=%v", problem)
	}
}

func TestHTTPContract(t *testing.T) {
	databaseURL := os.Getenv("AGENTROOM_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("AGENTROOM_TEST_DATABASE_URL is required for the integration HTTP contract suite")
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
	if _, err := lockConnection.Exec(ctx, "SELECT pg_advisory_lock($1)", testSuiteLock); err != nil {
		lockConnection.Release()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = lockConnection.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", testSuiteLock)
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
		t.Fatalf("refusing HTTP contract setup against non-test database %q", databaseName)
	}
	reset := `TRUNCATE oidc_states,web_sessions,service_tokens,project_memberships,user_accounts,
		run_control_outbox,event_outbox,command_results,audit_records,events,resources,projects
		RESTART IDENTITY CASCADE`
	if _, err := repository.Pool().Exec(ctx, reset); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = repository.Pool().Exec(context.Background(), reset)
	})

	exactCapabilities := []string{
		"resource:read", "resource:write", "event:read", "overview:read",
		"task:transition", "attention:manage", "claim:review",
		"run:pause", "run:resume", "run:cancel", "run:message", "run:redirect",
		"approval:request", "approval:decide",
	}
	if _, err := repository.Pool().Exec(
		ctx,
		"INSERT INTO projects(id,name) VALUES($1,$2)",
		projectID,
		"HTTP Contract",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Pool().Exec(ctx, `
		INSERT INTO user_accounts(id,username,display_name) VALUES
			('http-operator','operator','HTTP Operator'),
			('dotted-operator','dotted','Dotted Operator')`); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Pool().Exec(ctx, `
		INSERT INTO project_memberships(project_id,user_id,capabilities) VALUES
			($1,'http-operator',$2),
			($1,'dotted-operator',ARRAY['resource:read','run.pause']::text[])`,
		projectID, exactCapabilities,
	); err != nil {
		t.Fatal(err)
	}

	authManager, err := auth.New(
		ctx,
		repository.Pool(),
		[]byte("http-contract-session-secret-32-bytes"),
		false,
		allowedOrigin,
		"",
		"",
		"",
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	operatorSession, operatorToken, err := authManager.CreateSession(ctx, auth.User{
		ID: "http-operator", Username: "operator", DisplayName: "HTTP Operator",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	dottedSession, dottedToken, err := authManager.CreateSession(ctx, auth.User{
		ID: "dotted-operator", Username: "dotted", DisplayName: "Dotted Operator",
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	service := app.NewService(repository, nil)
	setupActor := app.Actor{ID: "setup", Capabilities: map[string]struct{}{"resource:write": {}}}
	seed := []struct {
		kind  domain.ResourceType
		value any
	}{
		{domain.ResourceTask, domain.Task{
			Base: domain.Base{ID: "task-one"}, Title: "Verify HTTP contract",
			Status: "ready", Priority: "high", Dependencies: []string{},
			Source: domain.Source{System: "contract-test"},
		}},
		{domain.ResourceRun, domain.Run{
			Base: domain.Base{ID: "run-one"}, AgentID: "agent-one", Status: "running",
			Capabilities: []string{"run:pause", "run:cancel"},
			Source:       domain.Source{System: "contract-test"},
		}},
		{domain.ResourceAttention, domain.Attention{
			Base: domain.Base{ID: "attention-one"}, Kind: "review", Severity: "high",
			Title: "Review release evidence", Status: "open",
		}},
		{domain.ResourceClaim, domain.Claim{
			Base: domain.Base{ID: "claim-one"}, TaskID: "task-one", AgentID: "agent-one",
			Status: "pending",
		}},
	}
	for index, resource := range seed {
		if _, err := service.Create(
			ctx,
			setupActor,
			resource.kind,
			projectID,
			"setup-"+strconv.Itoa(index),
			resource.value,
			"contract-test",
		); err != nil {
			t.Fatal(err)
		}
	}

	artifactStore, err := artifacts.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer artifactStore.Close()
	server := httptest.NewServer(httpapi.New(
		service,
		authManager,
		realtime.NewHub(),
		nil,
		artifactStore,
		1<<20,
		1<<20,
		allowedOrigin,
		[]string{allowedOrigin},
		nil,
		true,
	).Handler())
	defer server.Close()
	operator := authenticatedClient{
		baseURL: server.URL, token: operatorToken, csrf: operatorSession.CSRFToken,
	}
	dotted := authenticatedClient{
		baseURL: server.URL, token: dottedToken, csrf: dottedSession.CSRFToken,
	}
	anonymous := authenticatedClient{baseURL: server.URL}

	t.Run("registered read routes expose canonical envelopes", func(t *testing.T) {
		health := anonymous.request(t, http.MethodGet, "/healthz", nil, nil)
		if health.StatusCode != http.StatusOK {
			t.Fatalf("health status=%d", health.StatusCode)
		}
		for _, route := range []string{
			"agents", "runs", "sessions", "tasks", "attention", "evidence", "artifacts",
			"approvals", "interventions", "budgets", "claims", "audit", "organizations",
			"humans", "projects-state", "hosts", "agent-instances", "task-transitions",
			"situations", "policies", "deployments", "chat/messages",
		} {
			response := operator.request(
				t,
				http.MethodGet,
				"/api/v1/"+route+"?project_id="+projectID,
				nil,
				nil,
			)
			if response.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(response.Body)
				t.Fatalf("%s status=%d body=%s", route, response.StatusCode, body)
			}
			page := decodeObject(t, response)
			if _, ok := page["items"].([]any); !ok {
				t.Fatalf("%s page=%v", route, page)
			}
		}
		for _, route := range []string{
			"agents", "runs", "sessions", "tasks", "attention", "evidence", "artifacts",
			"approvals", "interventions", "budgets", "claims", "audit",
		} {
			response := operator.request(
				t,
				http.MethodGet,
				"/api/v1/"+route+"/missing?project_id="+projectID,
				nil,
				nil,
			)
			expectProblem(t, response, http.StatusNotFound, "not_found")
		}
		for _, route := range []string{"overview", "events", "health/components"} {
			response := operator.request(
				t,
				http.MethodGet,
				"/api/v1/"+route+"?project_id="+projectID,
				nil,
				nil,
			)
			if response.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(response.Body)
				t.Fatalf("%s status=%d body=%s", route, response.StatusCode, body)
			}
		}
	})

	t.Run("session and CSRF boundaries fail closed", func(t *testing.T) {
		unauthenticated := anonymous.request(
			t,
			http.MethodGet,
			"/api/v1/tasks?project_id="+projectID,
			nil,
			nil,
		)
		expectProblem(t, unauthenticated, http.StatusUnauthorized, "authentication_required")

		session := operator.request(t, http.MethodGet, "/api/v1/auth/session", nil, nil)
		if session.StatusCode != http.StatusOK {
			t.Fatalf("session status=%d", session.StatusCode)
		}
		sessionBody := decodeObject(t, session)
		if sessionBody["csrf_token"] != operator.csrf {
			t.Fatalf("session=%v", sessionBody)
		}

		missingOrigin := operator.request(
			t,
			http.MethodPost,
			"/api/v1/chat/messages?project_id="+projectID,
			strings.NewReader(`{"body":"boundary"}`),
			map[string]string{
				"Content-Type": "application/json", "X-CSRF-Token": operator.csrf,
				"Idempotency-Key": "missing-origin",
			},
		)
		expectProblem(t, missingOrigin, http.StatusForbidden, "origin_denied")

		missingCSRF := operator.request(
			t,
			http.MethodPost,
			"/api/v1/chat/messages?project_id="+projectID,
			strings.NewReader(`{"body":"boundary"}`),
			map[string]string{
				"Content-Type": "application/json", "Origin": allowedOrigin,
				"Idempotency-Key": "missing-csrf",
			},
		)
		expectProblem(t, missingCSRF, http.StatusForbidden, "csrf_failed")
	})

	t.Run("capabilities use exact colon syntax", func(t *testing.T) {
		dottedOnly := dotted.request(
			t,
			http.MethodPost,
			"/api/v1/runs/run-one/actions?project_id="+projectID,
			strings.NewReader(`{"action":"pause"}`),
			mutationHeaders(dotted, "dotted-pause", 1),
		)
		expectProblem(t, dottedOnly, http.StatusForbidden, "capability_denied")

		exactColon := operator.request(
			t,
			http.MethodPost,
			"/api/v1/runs/run-one/actions?project_id="+projectID,
			strings.NewReader(`{"action":"pause"}`),
			mutationHeaders(operator, "colon-pause", 1),
		)
		expectProblem(t, exactColon, http.StatusConflict, "runtime_action_unsupported")
	})

	t.Run("approval request binds action actor and target version", func(t *testing.T) {
		stale := operator.request(
			t,
			http.MethodPost,
			"/api/v1/approvals?project_id="+projectID,
			strings.NewReader(`{"run_id":"run-one","action":"cancel","expected_target_version":2,"expires_in_seconds":600}`),
			mutationHeaders(operator, "approval-stale", 0),
		)
		expectProblem(t, stale, http.StatusConflict, "version_conflict")

		unknownField := operator.request(
			t,
			http.MethodPost,
			"/api/v1/approvals?project_id="+projectID,
			strings.NewReader(`{"run_id":"run-one","action":"cancel","expected_target_version":1,"expires_in_seconds":600,"extra":true}`),
			mutationHeaders(operator, "approval-extra", 0),
		)
		expectProblem(t, unknownField, http.StatusBadRequest, "validation_failed")

		approvedRequest := operator.request(
			t,
			http.MethodPost,
			"/api/v1/approvals?project_id="+projectID,
			strings.NewReader(`{"run_id":"run-one","action":"cancel","message":"Unsafe output.","expected_target_version":1,"expires_in_seconds":600}`),
			mutationHeaders(operator, "approval-exact", 0),
		)
		if approvedRequest.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(approvedRequest.Body)
			t.Fatalf("approval status=%d body=%s", approvedRequest.StatusCode, body)
		}
		approval := decodeObject(t, approvedRequest)
		if approval["resource_id"] != "run-one" ||
			approval["expected_target_version"] != float64(1) ||
			approval["requested_by"] != "http-operator" ||
			!strings.HasPrefix(approval["command_digest"].(string), "sha256:") {
			t.Fatalf("approval=%v", approval)
		}
		contextValue := approval["context"].(map[string]any)
		if contextValue["action"] != "cancel" ||
			contextValue["expected_target_version"] != float64(1) {
			t.Fatalf("approval context=%v", contextValue)
		}
	})

	t.Run("artifact request body is immutable and integrity checked", func(t *testing.T) {
		content := []byte("verified artifact\n")
		headers := mutationHeaders(operator, "artifact-create", 0)
		headers["Content-Type"] = "text/plain; charset=utf-8"
		headers["X-Artifact-Name"] = "evidence.txt"
		uploaded := operator.request(
			t,
			http.MethodPost,
			"/api/v1/artifacts?project_id="+projectID+"&task_id=task-one&run_id=run-one",
			bytes.NewReader(content),
			headers,
		)
		if uploaded.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(uploaded.Body)
			t.Fatalf("upload status=%d body=%s", uploaded.StatusCode, body)
		}
		artifact := decodeObject(t, uploaded)
		if artifact["name"] != "evidence.txt" ||
			artifact["media_type"] != "text/plain" ||
			!strings.HasPrefix(artifact["digest"].(string), "sha256:") {
			t.Fatalf("artifact=%v", artifact)
		}
		artifactID := artifact["id"].(string)
		downloaded := operator.request(
			t,
			http.MethodGet,
			"/api/v1/artifacts/"+artifactID+"/content?project_id="+projectID,
			nil,
			nil,
		)
		if downloaded.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(downloaded.Body)
			t.Fatalf("download status=%d body=%s", downloaded.StatusCode, body)
		}
		got, err := io.ReadAll(downloaded.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, content) {
			t.Fatalf("artifact content=%q", got)
		}
		if downloaded.Header.Get("ETag") != `"`+artifact["digest"].(string)+`"` {
			t.Fatalf("etag=%q", downloaded.Header.Get("ETag"))
		}
	})
}
