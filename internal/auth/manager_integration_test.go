//go:build integration

package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/jackc/pgx/v5"
	"github.com/msitarzewski/agent-room/internal/app"
	"github.com/msitarzewski/agent-room/internal/postgres"
)

const authTestSuiteLock = int64(671327101)

func TestManagerDatabaseSecurityBoundaries(t *testing.T) {
	databaseURL := os.Getenv("AGENTROOM_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("AGENTROOM_TEST_DATABASE_URL is required for auth integration tests")
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
	if _, err := lockConnection.Exec(ctx, "SELECT pg_advisory_lock($1)", authTestSuiteLock); err != nil {
		lockConnection.Release()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = lockConnection.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", authTestSuiteLock)
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
		t.Fatalf("refusing auth test setup against non-test database %q", databaseName)
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

	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Pool().Exec(ctx, `
		INSERT INTO projects(id,name) VALUES
			('auth-project','Auth Project'),
			('other-project','Other Project')`); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Pool().Exec(ctx, `
		INSERT INTO user_accounts(id,username,password_hash,display_name,capabilities) VALUES
			('operator','operator',$1,'Operator',ARRAY['global:read']::text[]),
			('disabled','disabled',$1,'Disabled',ARRAY[]::text[]),
			('no-password','no-password',NULL,'No Password',ARRAY[]::text[])`,
		hash,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Pool().Exec(ctx, "UPDATE user_accounts SET disabled=true WHERE id='disabled'"); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Pool().Exec(ctx, `
		INSERT INTO project_memberships(project_id,user_id,capabilities)
			VALUES('auth-project','operator',ARRAY['resource:read','task:write']::text[])`); err != nil {
		t.Fatal(err)
	}

	manager, err := New(ctx, repository.Pool(), []byte("auth integration session secret"), true, "https://agentroom.test", "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if manager.OIDCConfigured() {
		t.Fatal("manager unexpectedly configured OIDC")
	}

	t.Run("password authentication fails closed", func(t *testing.T) {
		user, err := manager.AuthenticatePassword(ctx, "operator", "correct horse battery staple")
		if err != nil || user.ID != "operator" || user.DisplayName != "Operator" {
			t.Fatalf("user=%+v err=%v", user, err)
		}
		for _, attempt := range []struct{ username, password string }{
			{"operator", "wrong password"},
			{"missing", "correct horse battery staple"},
			{"disabled", "correct horse battery staple"},
			{"no-password", "correct horse battery staple"},
		} {
			if _, err := manager.AuthenticatePassword(ctx, attempt.username, attempt.password); !errors.Is(err, ErrUnauthenticated) {
				t.Fatalf("attempt=%+v err=%v", attempt, err)
			}
		}
		closedRepository, err := postgres.Open(ctx, databaseURL)
		if err != nil {
			t.Fatal(err)
		}
		closedManager, err := New(ctx, closedRepository.Pool(), []byte("closed database manager secret"), true, "https://agentroom.test", "", "", "", "")
		if err != nil {
			t.Fatal(err)
		}
		closedRepository.Close()
		if _, err := closedManager.AuthenticatePassword(ctx, "operator", "correct horse battery staple"); err == nil ||
			errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("database failure collapsed into credential rejection: %v", err)
		}
		if _, _, err := closedManager.CreateSession(ctx, User{ID: "operator"}, ""); err == nil {
			t.Fatal("session creation succeeded against a closed database")
		}
	})

	user := User{ID: "operator", Username: "operator", DisplayName: "Operator", Capabilities: []string{"global:read"}}
	t.Run("session lifecycle enforces expiry idle timeout and CSRF", func(t *testing.T) {
		session, token, err := manager.CreateSession(ctx, user, "")
		if err != nil || token == "" || session.CSRFToken == "" {
			t.Fatalf("session=%+v token=%q err=%v", session, token, err)
		}
		request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
		request.AddCookie(&http.Cookie{Name: CookieName, Value: token})
		resolved, err := manager.Resolve(ctx, request)
		if err != nil || resolved.User.ID != user.ID {
			t.Fatalf("resolved=%+v err=%v", resolved, err)
		}
		csrf, err := manager.CSRFToken(ctx, request)
		if err != nil || csrf != session.CSRFToken {
			t.Fatalf("csrf=%q err=%v", csrf, err)
		}
		request.Header.Set("X-CSRF-Token", csrf)
		if err := manager.CheckCSRF(ctx, request); err != nil {
			t.Fatal(err)
		}
		request.Header.Set("X-CSRF-Token", csrf+"x")
		if err := manager.CheckCSRF(ctx, request); !errors.Is(err, app.ErrDenied) {
			t.Fatalf("invalid CSRF err=%v", err)
		}

		manager.Logout(ctx, request)
		if _, err := manager.Resolve(ctx, request); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("logged-out session resolved: %v", err)
		}
		manager.Logout(ctx, httptest.NewRequest(http.MethodGet, "/", nil))

		_, expiredToken, err := manager.CreateSession(ctx, user, "")
		if err != nil {
			t.Fatal(err)
		}
		expiredHash := sha256.Sum256([]byte(expiredToken))
		if _, err := repository.Pool().Exec(ctx, "UPDATE web_sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1", expiredHash[:]); err != nil {
			t.Fatal(err)
		}
		expired := httptest.NewRequest(http.MethodGet, "/", nil)
		expired.AddCookie(&http.Cookie{Name: CookieName, Value: expiredToken})
		if _, err := manager.Resolve(ctx, expired); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("expired session resolved: %v", err)
		}
		if _, err := manager.CSRFToken(ctx, expired); !errors.Is(err, app.ErrDenied) {
			t.Fatalf("expired session returned CSRF: %v", err)
		}

		_, idleToken, err := manager.CreateSession(ctx, user, "")
		if err != nil {
			t.Fatal(err)
		}
		idleHash := sha256.Sum256([]byte(idleToken))
		if _, err := repository.Pool().Exec(ctx, "UPDATE web_sessions SET last_seen_at=now()-interval '31 minutes' WHERE token_hash=$1", idleHash[:]); err != nil {
			t.Fatal(err)
		}
		idle := httptest.NewRequest(http.MethodGet, "/", nil)
		idle.AddCookie(&http.Cookie{Name: CookieName, Value: idleToken})
		if _, err := manager.Resolve(ctx, idle); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("idle session resolved: %v", err)
		}
		if err := manager.CheckCSRF(ctx, idle); !errors.Is(err, app.ErrDenied) {
			t.Fatalf("idle session passed CSRF: %v", err)
		}

		missing := httptest.NewRequest(http.MethodGet, "/", nil)
		if _, err := manager.Resolve(ctx, missing); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("missing session resolved: %v", err)
		}
		if _, err := manager.CSRFToken(ctx, missing); !errors.Is(err, app.ErrDenied) {
			t.Fatalf("missing session returned CSRF: %v", err)
		}
		if err := manager.CheckCSRF(ctx, missing); !errors.Is(err, app.ErrDenied) {
			t.Fatalf("missing session passed CSRF: %v", err)
		}
	})

	t.Run("project authorization is explicit", func(t *testing.T) {
		actor, err := manager.ActorForProject(ctx, user, "auth-project")
		if err != nil || actor.ID != user.ID || !actor.Can("resource:read") || actor.Can("resource:write") {
			t.Fatalf("actor=%+v err=%v", actor, err)
		}
		if _, err := manager.ActorForProject(ctx, user, "other-project"); !errors.Is(err, app.ErrDenied) {
			t.Fatalf("unscoped project err=%v", err)
		}
		global := user
		global.Capabilities = append(global.Capabilities, "project:all")
		actor, err = manager.ActorForProject(ctx, global, "other-project")
		if err != nil || !actor.Can("project:all") {
			t.Fatalf("global actor=%+v err=%v", actor, err)
		}
		memberProjects, err := manager.Projects(ctx, user)
		if err != nil || len(memberProjects) != 1 || memberProjects[0]["id"] != "auth-project" {
			t.Fatalf("member projects=%v err=%v", memberProjects, err)
		}
		allProjects, err := manager.Projects(ctx, global)
		if err != nil || len(allProjects) != 2 {
			t.Fatalf("global projects=%v err=%v", allProjects, err)
		}
		emptyProjects, err := manager.Projects(ctx, User{ID: "no-password"})
		if err != nil || emptyProjects == nil || len(emptyProjects) != 0 {
			t.Fatalf("empty projects=%v err=%v", emptyProjects, err)
		}
	})

	t.Run("service tokens are scoped expiring and revocable", func(t *testing.T) {
		for _, invalid := range []struct {
			id, name, actor string
			projects, caps  []string
			expiry          *time.Time
		}{
			{"", "name", "actor", []string{"auth-project"}, []string{"resource:read"}, nil},
			{"wildcard", "name", "actor", []string{"*"}, []string{"resource:read"}, nil},
			{"missing-project", "name", "actor", []string{"absent"}, []string{"resource:read"}, nil},
		} {
			if _, err := CreateServiceTokenWithMetadata(ctx, repository.Pool(), invalid.id, invalid.name, invalid.actor, invalid.projects, invalid.caps, invalid.expiry); err == nil {
				t.Fatalf("invalid service token accepted: %+v", invalid)
			}
		}
		past := time.Now().Add(-time.Minute)
		farFuture := time.Now().Add(91 * 24 * time.Hour)
		for index, expiry := range []*time.Time{&past, &farFuture} {
			if _, err := CreateServiceTokenWithMetadata(ctx, repository.Pool(), "bad-expiry-"+string(rune('0'+index)), "name", "actor", []string{"auth-project"}, []string{"resource:read"}, expiry); err == nil {
				t.Fatalf("invalid expiry accepted: %v", expiry)
			}
		}

		expires := time.Now().Add(time.Hour)
		created, err := CreateServiceTokenWithMetadata(ctx, repository.Pool(), "service-one", "Pip", "agent:pip", []string{"auth-project"}, []string{"resource:read", "task:write"}, &expires)
		if err != nil || created.Token == "" || created.ID != "service-one" {
			t.Fatalf("created=%+v err=%v", created, err)
		}
		items, err := ListServiceTokens(ctx, repository.Pool())
		if err != nil || len(items) != 1 || items[0].ID != created.ID || items[0].RevokedAt != nil {
			t.Fatalf("tokens=%+v err=%v", items, err)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/v1/ingest?project_id=auth-project", nil)
		request.Header.Set("Authorization", "Bearer "+created.Token)
		identity, err := manager.ResolveServiceToken(ctx, request, "auth-project")
		if err != nil || identity.ID != created.ID || identity.Actor.ID != "agent:pip" ||
			!identity.Actor.Can("task:write") {
			t.Fatalf("identity=%+v err=%v", identity, err)
		}
		if _, err := manager.ResolveServiceToken(ctx, request, "other-project"); !errors.Is(err, app.ErrDenied) {
			t.Fatalf("cross-project token err=%v", err)
		}
		for _, authorization := range []string{"", "Basic abc", "Bearer", "Bearer "} {
			request.Header.Set("Authorization", authorization)
			if _, err := manager.ResolveServiceToken(ctx, request, "auth-project"); !errors.Is(err, ErrUnauthenticated) {
				t.Fatalf("authorization=%q err=%v", authorization, err)
			}
		}
		request.Header.Set("Authorization", "Bearer "+created.Token)
		alreadyRevoked, err := RevokeServiceToken(ctx, repository.Pool(), created.ID)
		if err != nil || alreadyRevoked {
			t.Fatalf("first revoke already=%v err=%v", alreadyRevoked, err)
		}
		if _, err := manager.ResolveServiceToken(ctx, request, "auth-project"); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("revoked token resolved: %v", err)
		}
		alreadyRevoked, err = RevokeServiceToken(ctx, repository.Pool(), created.ID)
		if err != nil || !alreadyRevoked {
			t.Fatalf("second revoke already=%v err=%v", alreadyRevoked, err)
		}
		items, err = ListServiceTokens(ctx, repository.Pool())
		if err != nil || items[0].RevokedAt == nil {
			t.Fatalf("revoked metadata=%+v err=%v", items, err)
		}
		legacyToken, err := CreateServiceToken(
			ctx, repository.Pool(), "service-wrapper", "Wrapper", "agent:wrapper",
			[]string{"auth-project"}, []string{"resource:read"}, nil,
		)
		if err != nil || legacyToken == "" {
			t.Fatalf("service-token wrapper token=%q err=%v", legacyToken, err)
		}
		if _, err := RevokeServiceToken(ctx, repository.Pool(), "missing"); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("missing revoke err=%v", err)
		}
	})

	t.Run("OIDC state PKCE browser and nonce binding", func(t *testing.T) {
		brokenProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		}))
		if _, err := New(
			ctx, repository.Pool(), []byte("OIDC integration session secret"),
			true, "https://agentroom.test", brokenProvider.URL, "auth-test-client", "client-secret",
			"https://agentroom.test/api/v1/auth/callback",
		); err == nil || !strings.Contains(err.Error(), "discover OIDC provider") {
			t.Fatalf("broken OIDC discovery error=%v", err)
		}
		brokenProvider.Close()

		signingKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		signer, err := jose.NewSigner(
			jose.SigningKey{
				Algorithm: jose.RS256,
				Key:       jose.JSONWebKey{Key: signingKey, KeyID: "auth-test", Algorithm: string(jose.RS256), Use: "sig"},
			},
			(&jose.SignerOptions{}).WithType("JWT"),
		)
		if err != nil {
			t.Fatal(err)
		}
		var issuer string
		var tokenNonce string
		provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/openid-configuration":
				writeTestJSON(t, w, map[string]any{
					"issuer":                                issuer,
					"authorization_endpoint":                issuer + "/authorize",
					"token_endpoint":                        issuer + "/token",
					"jwks_uri":                              issuer + "/keys",
					"id_token_signing_alg_values_supported": []string{"RS256"},
				})
			case "/keys":
				writeTestJSON(t, w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
					Key: &signingKey.PublicKey, KeyID: "auth-test", Algorithm: string(jose.RS256), Use: "sig",
				}}})
			case "/token":
				if err := r.ParseForm(); err != nil {
					t.Error(err)
					http.Error(w, "invalid form", http.StatusBadRequest)
					return
				}
				code := r.Form.Get("code")
				if r.Form.Get("code_verifier") == "" || code == "invalid-code" {
					http.Error(w, "invalid grant", http.StatusBadRequest)
					return
				}
				if code == "missing-id-token" {
					writeTestJSON(t, w, map[string]any{
						"access_token": "access-token", "token_type": "Bearer", "expires_in": 3600,
					})
					return
				}
				if code == "invalid-id-token" {
					writeTestJSON(t, w, map[string]any{
						"access_token": "access-token", "token_type": "Bearer",
						"expires_in": 3600, "id_token": "not-a-jwt",
					})
					return
				}
				nonce := tokenNonce
				if code == "wrong-nonce" {
					nonce = "a-different-nonce"
				}
				customClaims := map[string]any{"nonce": nonce}
				if code != "default-profile" {
					customClaims["email"] = "oidc-security@agentroom.test"
					customClaims["name"] = "OIDC Security"
				}
				idToken, err := jwt.Signed(signer).
					Claims(jwt.Claims{
						Issuer: issuer, Subject: "oidc-security-user",
						Audience: jwt.Audience{"auth-test-client"},
						Expiry:   jwt.NewNumericDate(time.Now().Add(time.Hour)),
						IssuedAt: jwt.NewNumericDate(time.Now()),
					}).
					Claims(customClaims).
					Serialize()
				if err != nil {
					t.Error(err)
					http.Error(w, "signing failed", http.StatusInternalServerError)
					return
				}
				writeTestJSON(t, w, map[string]any{
					"access_token": "access-token", "token_type": "Bearer",
					"expires_in": 3600, "id_token": idToken,
				})
			default:
				http.NotFound(w, r)
			}
		}))
		defer provider.Close()
		issuer = provider.URL

		oidcManager, err := New(
			ctx, repository.Pool(), []byte("OIDC integration session secret"),
			true, "https://agentroom.test", issuer, "auth-test-client", "client-secret",
			"https://agentroom.test/api/v1/auth/callback",
		)
		if err != nil || !oidcManager.OIDCConfigured() {
			t.Fatalf("OIDC manager configured=%v err=%v", oidcManager != nil && oidcManager.OIDCConfigured(), err)
		}
		if _, _, err := oidcManager.BeginOIDC(ctx, "https://attacker.test"); err == nil {
			t.Fatal("unsafe OIDC return path accepted")
		}
		begin := func(t *testing.T, returnTo string) (string, string) {
			t.Helper()
			location, browser, err := oidcManager.BeginOIDC(ctx, returnTo)
			if err != nil {
				t.Fatal(err)
			}
			authorizationURL, err := url.Parse(location)
			if err != nil {
				t.Fatal(err)
			}
			state := authorizationURL.Query().Get("state")
			tokenNonce = authorizationURL.Query().Get("nonce")
			if state == "" || tokenNonce == "" ||
				authorizationURL.Query().Get("code_challenge") == "" ||
				authorizationURL.Query().Get("code_challenge_method") != "S256" {
				t.Fatalf("authorization URL lacks state/nonce/PKCE: %s", location)
			}
			return state, browser
		}
		state, browser := begin(t, "/projects/auth-project")
		primaryNonce := tokenNonce
		if _, _, _, err := oidcManager.FinishOIDC(ctx, state, "invalid-code", browser); err == nil ||
			!strings.Contains(err.Error(), "exchange OIDC code") {
			t.Fatalf("invalid code error=%v", err)
		}
		if _, _, _, err := oidcManager.FinishOIDC(ctx, state, "missing-id-token", browser); err == nil ||
			!strings.Contains(err.Error(), "did not include id_token") {
			t.Fatalf("missing ID token error=%v", err)
		}
		if _, _, _, err := oidcManager.FinishOIDC(ctx, state, "invalid-id-token", browser); err == nil ||
			!strings.Contains(err.Error(), "verify ID token") {
			t.Fatalf("invalid ID token error=%v", err)
		}
		if _, _, _, err := oidcManager.FinishOIDC(ctx, state, "wrong-nonce", browser); err == nil ||
			!strings.Contains(err.Error(), "nonce mismatch") {
			t.Fatalf("nonce mismatch error=%v", err)
		}
		cipherState, cipherBrowser := begin(t, "/")
		cipherStateHash := sha256.Sum256([]byte(cipherState))
		if _, err := repository.Pool().Exec(ctx,
			"UPDATE oidc_states SET verifier_ciphertext=$1 WHERE state_hash=$2",
			[]byte("short"), cipherStateHash[:],
		); err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := oidcManager.FinishOIDC(ctx, cipherState, "valid-code", cipherBrowser); err == nil ||
			!strings.Contains(err.Error(), "invalid ciphertext") {
			t.Fatalf("invalid verifier ciphertext error=%v", err)
		}
		integrityState, integrityBrowser := begin(t, "/")
		integrityStateHash := sha256.Sum256([]byte(integrityState))
		if _, err := repository.Pool().Exec(ctx,
			"UPDATE oidc_states SET verifier_hash=$1 WHERE state_hash=$2",
			make([]byte, sha256.Size), integrityStateHash[:],
		); err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := oidcManager.FinishOIDC(ctx, integrityState, "valid-code", integrityBrowser); err == nil ||
			!strings.Contains(err.Error(), "integrity check failed") {
			t.Fatalf("verifier integrity error=%v", err)
		}
		if _, _, _, err := oidcManager.FinishOIDC(ctx, "missing", "valid-code", browser); err == nil ||
			!strings.Contains(err.Error(), "invalid or expired") {
			t.Fatalf("invalid state error=%v", err)
		}
		if _, _, _, err := oidcManager.FinishOIDC(ctx, state, "valid-code", "other-browser"); err == nil ||
			!strings.Contains(err.Error(), "browser mismatch") {
			t.Fatalf("browser mismatch error=%v", err)
		}
		tokenNonce = primaryNonce
		session, sessionToken, returnTo, err := oidcManager.FinishOIDC(ctx, state, "valid-code", browser)
		if err != nil {
			t.Fatal(err)
		}
		if session.User.ID != "oidc:oidc-security-user" ||
			session.User.Username != "oidc-security@agentroom.test" ||
			session.User.DisplayName != "OIDC Security" ||
			len(session.User.Capabilities) != 1 || session.User.Capabilities[0] != "resource:read" ||
			sessionToken == "" || returnTo != "/projects/auth-project" {
			t.Fatalf("OIDC session=%+v token=%q returnTo=%q", session, sessionToken, returnTo)
		}
		var storedSubject string
		if err := repository.Pool().QueryRow(ctx,
			"SELECT oidc_subject FROM web_sessions WHERE user_id=$1",
			session.User.ID,
		).Scan(&storedSubject); err != nil || storedSubject != "oidc-security-user" {
			t.Fatalf("stored OIDC subject=%q err=%v", storedSubject, err)
		}

		replay, _, _, err := oidcManager.FinishOIDC(ctx, state, "valid-code", browser)
		if err == nil || replay.User.ID != "" || !strings.Contains(err.Error(), "invalid or expired") {
			t.Fatalf("OIDC state replay accepted: session=%+v err=%v", replay, err)
		}

		defaultState, defaultBrowser := begin(t, "")
		defaultSession, _, defaultReturnTo, err := oidcManager.FinishOIDC(
			ctx, defaultState, "default-profile", defaultBrowser,
		)
		if err != nil {
			t.Fatal(err)
		}
		if defaultSession.User.Username != defaultSession.User.ID ||
			defaultSession.User.DisplayName != defaultSession.User.ID || defaultReturnTo != "" {
			t.Fatalf("default OIDC profile=%+v returnTo=%q", defaultSession.User, defaultReturnTo)
		}
	})
}

func writeTestJSON(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		t.Error(err)
	}
}
