package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/msitarzewski/agent-room/internal/app"
	"github.com/msitarzewski/agent-room/internal/postgres/sqlcgen"
	"golang.org/x/crypto/argon2"
	"golang.org/x/oauth2"
)

const CookieName = "__Host-agentroom"
const OIDCCookieName = "__Host-agentroom-oidc"
const DevCookieName = "agentroom-dev"
const DevOIDCCookieName = "agentroom-oidc-dev"

var ErrUnauthenticated = errors.New("authentication required")

type User struct {
	ID           string   `json:"id"`
	Username     string   `json:"username"`
	DisplayName  string   `json:"display_name"`
	Capabilities []string `json:"capabilities"`
}

func (u User) Actor() app.Actor {
	caps := make(map[string]struct{}, len(u.Capabilities))
	for _, capability := range u.Capabilities {
		caps[capability] = struct{}{}
	}
	return app.Actor{ID: u.ID, Capabilities: caps}
}

type Session struct {
	User      User      `json:"user"`
	CSRFToken string    `json:"csrf_token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type ServiceIdentity struct {
	ID           string
	Actor        app.Actor
	ProjectIDs   []string
	Capabilities []string
}

type Manager struct {
	pool          *pgxpool.Pool
	sessionSecret []byte
	secureCookies bool
	publicURL     string
	oidcProvider  *oidc.Provider
	oidcVerifier  *oidc.IDTokenVerifier
	oauth         *oauth2.Config
	queries       *sqlcgen.Queries
}

func (m *Manager) OIDCConfigured() bool {
	return m.oidcProvider != nil && m.oidcVerifier != nil && m.oauth != nil
}

func New(ctx context.Context, pool *pgxpool.Pool, sessionSecret []byte, secureCookies bool, publicURL, issuer, clientID, clientSecret, redirectURL string) (*Manager, error) {
	m := &Manager{pool: pool, sessionSecret: append([]byte(nil), sessionSecret...), secureCookies: secureCookies, publicURL: publicURL, queries: sqlcgen.New(pool)}
	if issuer == "" {
		return m, nil
	}
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider: %w", err)
	}
	m.oidcProvider = provider
	m.oidcVerifier = provider.Verifier(&oidc.Config{ClientID: clientID})
	m.oauth = &oauth2.Config{ClientID: clientID, ClientSecret: clientSecret, Endpoint: provider.Endpoint(), RedirectURL: redirectURL, Scopes: []string{oidc.ScopeOpenID, "profile", "email"}}
	return m, nil
}

func (m *Manager) AuthenticatePassword(ctx context.Context, username, password string) (User, error) {
	var user User
	var hash *string
	var disabled bool
	err := m.pool.QueryRow(ctx, `SELECT id,username,display_name,password_hash,disabled,capabilities
		FROM user_accounts WHERE username=$1`, username).
		Scan(&user.ID, &user.Username, &user.DisplayName, &hash, &disabled, &user.Capabilities)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUnauthenticated
	}
	if err != nil {
		return User{}, err
	}
	if hash == nil || disabled || !verifyPassword(*hash, password) {
		return User{}, ErrUnauthenticated
	}
	return user, nil
}

func (m *Manager) CreateSession(ctx context.Context, user User, oidcSubject string) (Session, string, error) {
	token, err := randomToken(32)
	if err != nil {
		return Session{}, "", err
	}
	csrf := m.csrfForToken(token)
	tokenHash, csrfHash := sha256.Sum256([]byte(token)), sha256.Sum256([]byte(csrf))
	expires := time.Now().UTC().Add(12 * time.Hour)
	_, err = m.pool.Exec(ctx, `INSERT INTO web_sessions(token_hash,user_id,csrf_hash,expires_at,oidc_subject)
		VALUES($1,$2,$3,$4,NULLIF($5,''))`, tokenHash[:], user.ID, csrfHash[:], expires, oidcSubject)
	if err != nil {
		return Session{}, "", err
	}
	return Session{User: user, CSRFToken: csrf, ExpiresAt: expires}, token, nil
}

func (m *Manager) Resolve(ctx context.Context, request *http.Request) (Session, error) {
	cookie, err := request.Cookie(m.sessionCookieName())
	if err != nil {
		return Session{}, ErrUnauthenticated
	}
	tokenHash := sha256.Sum256([]byte(cookie.Value))
	var session Session
	var disabled bool
	err = m.pool.QueryRow(ctx, `SELECT u.id,u.username,u.display_name,u.capabilities,s.csrf_hash,s.expires_at,u.disabled
		FROM web_sessions s JOIN user_accounts u ON u.id=s.user_id
		WHERE s.token_hash=$1 AND s.expires_at>now() AND s.last_seen_at>now()-interval '30 minutes'`, tokenHash[:]).
		Scan(&session.User.ID, &session.User.Username, &session.User.DisplayName, &session.User.Capabilities, new([]byte), &session.ExpiresAt, &disabled)
	if errors.Is(err, pgx.ErrNoRows) || disabled {
		return Session{}, ErrUnauthenticated
	}
	if err != nil {
		return Session{}, err
	}
	_, _ = m.pool.Exec(ctx, "UPDATE web_sessions SET last_seen_at=now() WHERE token_hash=$1", tokenHash[:])
	return session, nil
}

func (m *Manager) ActorForProject(ctx context.Context, user User, projectID string) (app.Actor, error) {
	for _, capability := range user.Capabilities {
		if capability == "project:all" {
			return user.Actor(), nil
		}
	}
	capabilities, err := m.queries.GetProjectCapabilities(ctx, sqlcgen.GetProjectCapabilitiesParams{ProjectID: projectID, UserID: user.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.Actor{}, app.ErrDenied
	}
	if err != nil {
		return app.Actor{}, err
	}
	actor := app.Actor{ID: user.ID, Capabilities: make(map[string]struct{}, len(capabilities))}
	for _, capability := range capabilities {
		actor.Capabilities[capability] = struct{}{}
	}
	return actor, nil
}

func (m *Manager) Projects(ctx context.Context, user User) ([]map[string]any, error) {
	query := `SELECT p.id,p.name,p.version,p.updated_at,m.capabilities
		FROM projects p JOIN project_memberships m ON m.project_id=p.id
		WHERE m.user_id=$1 ORDER BY p.name,p.id`
	args := []any{user.ID}
	for _, capability := range user.Capabilities {
		if capability == "project:all" {
			query = `SELECT id,name,version,updated_at,$1::text[] FROM projects ORDER BY name,id`
			args = []any{user.Capabilities}
			break
		}
	}
	rows, err := m.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	projects := make([]map[string]any, 0)
	for rows.Next() {
		var id, name string
		var version int64
		var updatedAt time.Time
		var capabilities []string
		if err := rows.Scan(&id, &name, &version, &updatedAt, &capabilities); err != nil {
			return nil, err
		}
		projects = append(projects, map[string]any{"id": id, "name": name, "version": version, "updated_at": updatedAt, "capabilities": capabilities})
	}
	return projects, rows.Err()
}

func (m *Manager) ResolveServiceToken(ctx context.Context, request *http.Request, projectID string) (ServiceIdentity, error) {
	scheme, token, ok := strings.Cut(request.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return ServiceIdentity{}, ErrUnauthenticated
	}
	tokenHash := sha256.Sum256([]byte(token))
	var identity ServiceIdentity
	row, err := m.queries.GetServiceToken(ctx, tokenHash[:])
	if errors.Is(err, pgx.ErrNoRows) {
		return ServiceIdentity{}, ErrUnauthenticated
	}
	if err != nil {
		return ServiceIdentity{}, err
	}
	identity.ID, identity.Actor.ID, identity.ProjectIDs, identity.Capabilities = row.ID, row.ActorID, row.ProjectIds, row.Capabilities
	allowed := false
	for _, candidate := range identity.ProjectIDs {
		if candidate == projectID || candidate == "*" {
			allowed = true
			break
		}
	}
	if !allowed {
		return ServiceIdentity{}, app.ErrDenied
	}
	identity.Actor.Capabilities = make(map[string]struct{}, len(identity.Capabilities))
	for _, capability := range identity.Capabilities {
		identity.Actor.Capabilities[capability] = struct{}{}
	}
	return identity, nil
}

func CreateServiceToken(ctx context.Context, pool *pgxpool.Pool, id, name, actorID string, projectIDs, capabilities []string, expiresAt *time.Time) (string, error) {
	created, err := CreateServiceTokenWithMetadata(ctx, pool, id, name, actorID, projectIDs, capabilities, expiresAt)
	return created.Token, err
}

type CreatedServiceToken struct {
	ID        string    `json:"id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type ServiceTokenMetadata struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	ActorID      string     `json:"actor_id"`
	ProjectIDs   []string   `json:"project_ids"`
	Capabilities []string   `json:"capabilities"`
	ExpiresAt    *time.Time `json:"expires_at"`
	RevokedAt    *time.Time `json:"revoked_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

func CreateServiceTokenWithMetadata(ctx context.Context, pool *pgxpool.Pool, id, name, actorID string, projectIDs, capabilities []string, expiresAt *time.Time) (CreatedServiceToken, error) {
	if id == "" || name == "" || actorID == "" || len(projectIDs) == 0 || len(capabilities) == 0 {
		return CreatedServiceToken{}, errors.New("service token id, name, actor, projects, and capabilities are required")
	}
	now := time.Now().UTC()
	expiry := now.Add(30 * 24 * time.Hour)
	if expiresAt != nil {
		expiry = expiresAt.UTC()
	}
	if !expiry.After(now) || expiry.After(now.Add(90*24*time.Hour)) {
		return CreatedServiceToken{}, errors.New("service token expiry must be in the future and at most 90 days")
	}
	for _, projectID := range projectIDs {
		if projectID == "*" {
			return CreatedServiceToken{}, errors.New("wildcard project scope is not allowed")
		}
	}
	var projectCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM projects WHERE id=ANY($1::text[])`, projectIDs).Scan(&projectCount); err != nil {
		return CreatedServiceToken{}, err
	}
	if projectCount != len(projectIDs) {
		return CreatedServiceToken{}, errors.New("every service token project must exist")
	}
	token, err := randomToken(32)
	if err != nil {
		return CreatedServiceToken{}, err
	}
	hash := sha256.Sum256([]byte(token))
	tx, err := pool.Begin(ctx)
	if err != nil {
		return CreatedServiceToken{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO service_tokens(token_hash,id,name,actor_id,project_ids,capabilities,expires_at)
		VALUES($1,$2,$3,$4,$5,$6,$7)`, hash[:], id, name, actorID, projectIDs, capabilities, expiry)
	if err != nil {
		return CreatedServiceToken{}, err
	}
	for _, projectID := range projectIDs {
		details, _ := json.Marshal(map[string]any{"name": name, "actor_id": actorID, "expires_at": expiry, "capabilities": capabilities})
		auditID, err := randomToken(16)
		if err != nil {
			return CreatedServiceToken{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO audit_records(
			id,project_id,actor_id,action,resource_type,resource_id,outcome,details,occurred_at
		) VALUES($1,$2,'local-operator','service_token.create','service_token',$3,'accepted',$4,$5)`,
			auditID, projectID, id, details, now); err != nil {
			return CreatedServiceToken{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return CreatedServiceToken{}, err
	}
	return CreatedServiceToken{ID: id, Token: token, ExpiresAt: expiry}, nil
}

func ListServiceTokens(ctx context.Context, pool *pgxpool.Pool) ([]ServiceTokenMetadata, error) {
	rows, err := pool.Query(ctx, `SELECT id,name,actor_id,project_ids,capabilities,expires_at,revoked_at,created_at
		FROM service_tokens ORDER BY created_at DESC,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ServiceTokenMetadata, 0)
	for rows.Next() {
		var item ServiceTokenMetadata
		if err := rows.Scan(&item.ID, &item.Name, &item.ActorID, &item.ProjectIDs, &item.Capabilities,
			&item.ExpiresAt, &item.RevokedAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func RevokeServiceToken(ctx context.Context, pool *pgxpool.Pool, id string) (bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var projectIDs []string
	var revokedAt *time.Time
	if err := tx.QueryRow(ctx, `SELECT project_ids,revoked_at FROM service_tokens WHERE id=$1 FOR UPDATE`, id).Scan(&projectIDs, &revokedAt); err != nil {
		return false, err
	}
	if revokedAt != nil {
		return true, tx.Commit(ctx)
	}
	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `UPDATE service_tokens SET revoked_at=$1 WHERE id=$2`, now, id); err != nil {
		return false, err
	}
	for _, projectID := range projectIDs {
		details, _ := json.Marshal(map[string]any{"revoked_at": now})
		auditID, err := randomToken(16)
		if err != nil {
			return false, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO audit_records(
			id,project_id,actor_id,action,resource_type,resource_id,outcome,details,occurred_at
		) VALUES($1,$2,'local-operator','service_token.revoke','service_token',$3,'accepted',$4,$5)`,
			auditID, projectID, id, details, now); err != nil {
			return false, err
		}
	}
	return false, tx.Commit(ctx)
}

func (m *Manager) CSRFToken(ctx context.Context, request *http.Request) (string, error) {
	cookie, err := request.Cookie(m.sessionCookieName())
	if err != nil {
		return "", app.ErrDenied
	}
	tokenHash := sha256.Sum256([]byte(cookie.Value))
	var exists bool
	err = m.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM web_sessions
		WHERE token_hash=$1 AND expires_at>now() AND last_seen_at>now()-interval '30 minutes')`, tokenHash[:]).Scan(&exists)
	if err != nil || !exists {
		return "", app.ErrDenied
	}
	return m.csrfForToken(cookie.Value), nil
}

func (m *Manager) CheckCSRF(ctx context.Context, request *http.Request) error {
	cookie, err := request.Cookie(m.sessionCookieName())
	if err != nil {
		return app.ErrDenied
	}
	tokenHash := sha256.Sum256([]byte(cookie.Value))
	var storedHash []byte
	err = m.pool.QueryRow(ctx, "SELECT csrf_hash FROM web_sessions WHERE token_hash=$1 AND expires_at>now() AND last_seen_at>now()-interval '30 minutes'", tokenHash[:]).Scan(&storedHash)
	if err != nil {
		return app.ErrDenied
	}
	expected := m.csrfForToken(cookie.Value)
	actual := request.Header.Get("X-CSRF-Token")
	actualHash := sha256.Sum256([]byte(actual))
	if subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) != 1 || subtle.ConstantTimeCompare(storedHash, actualHash[:]) != 1 {
		return app.ErrDenied
	}
	return nil
}

func (m *Manager) csrfForToken(token string) string {
	mac := hmac.New(sha256.New, m.sessionSecret)
	_, _ = mac.Write([]byte("agentroom-csrf-v1\x00"))
	_, _ = mac.Write([]byte(token))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (m *Manager) Logout(ctx context.Context, request *http.Request) {
	if cookie, err := request.Cookie(m.sessionCookieName()); err == nil {
		tokenHash := sha256.Sum256([]byte(cookie.Value))
		_, _ = m.pool.Exec(ctx, "DELETE FROM web_sessions WHERE token_hash=$1", tokenHash[:])
	}
}

func (m *Manager) SetCookie(writer http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(writer, &http.Cookie{Name: m.sessionCookieName(), Value: token, Path: "/", Secure: m.secureCookies, HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(time.Until(expires).Seconds())})
}

func (m *Manager) ClearCookie(writer http.ResponseWriter) {
	http.SetCookie(writer, &http.Cookie{Name: m.sessionCookieName(), Value: "", Path: "/", Secure: m.secureCookies, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

func (m *Manager) BeginOIDC(ctx context.Context, returnTo string) (string, string, error) {
	if m.oauth == nil {
		return "", "", errors.New("OIDC is not configured")
	}
	if !safeReturnTo(returnTo) {
		return "", "", errors.New("unsafe return_to")
	}
	state, err := randomToken(32)
	if err != nil {
		return "", "", err
	}
	browser, _ := randomToken(32)
	nonce, _ := randomToken(32)
	verifier := oauth2.GenerateVerifier()
	stateHash, browserHash := sha256.Sum256([]byte(state)), sha256.Sum256([]byte(browser))
	nonceHash, verifierHash := sha256.Sum256([]byte(nonce)), sha256.Sum256([]byte(verifier))
	encrypted, err := m.encrypt([]byte(verifier))
	if err != nil {
		return "", "", err
	}
	_, err = m.pool.Exec(ctx, `INSERT INTO oidc_states(state_hash,browser_hash,nonce_hash,verifier_hash,verifier_ciphertext,return_to,expires_at)
		VALUES($1,$2,$3,$4,$5,$6,$7)`, stateHash[:], browserHash[:], nonceHash[:], verifierHash[:], encrypted, returnTo, time.Now().UTC().Add(10*time.Minute))
	if err != nil {
		return "", "", err
	}
	return m.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), browser, nil
}

func (m *Manager) FinishOIDC(ctx context.Context, state, code, browser string) (Session, string, string, error) {
	if m.oauth == nil {
		return Session{}, "", "", errors.New("OIDC is not configured")
	}
	stateHash := sha256.Sum256([]byte(state))
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return Session{}, "", "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var expectedBrowserHash, nonceHash, verifierHash, encrypted []byte
	var returnTo string
	err = tx.QueryRow(ctx, `DELETE FROM oidc_states WHERE state_hash=$1 AND expires_at>now()
		RETURNING browser_hash,nonce_hash,verifier_hash,verifier_ciphertext,return_to`, stateHash[:]).
		Scan(&expectedBrowserHash, &nonceHash, &verifierHash, &encrypted, &returnTo)
	if err != nil {
		return Session{}, "", "", errors.New("invalid or expired OIDC state")
	}
	actualBrowserHash := sha256.Sum256([]byte(browser))
	if browser == "" || subtle.ConstantTimeCompare(actualBrowserHash[:], expectedBrowserHash) != 1 {
		return Session{}, "", "", errors.New("OIDC transaction browser mismatch")
	}
	verifierBytes, err := m.decrypt(encrypted)
	if err != nil {
		return Session{}, "", "", err
	}
	check := sha256.Sum256(verifierBytes)
	if subtle.ConstantTimeCompare(check[:], verifierHash) != 1 {
		return Session{}, "", "", errors.New("OIDC verifier integrity check failed")
	}
	token, err := m.oauth.Exchange(ctx, code, oauth2.VerifierOption(string(verifierBytes)))
	if err != nil {
		return Session{}, "", "", fmt.Errorf("exchange OIDC code: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return Session{}, "", "", errors.New("OIDC response did not include id_token")
	}
	idToken, err := m.oidcVerifier.Verify(ctx, rawIDToken)
	if err != nil {
		return Session{}, "", "", fmt.Errorf("verify ID token: %w", err)
	}
	var claims struct {
		Subject string `json:"sub"`
		Nonce   string `json:"nonce"`
		Email   string `json:"email"`
		Name    string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return Session{}, "", "", err
	}
	claimedNonceHash := sha256.Sum256([]byte(claims.Nonce))
	if subtle.ConstantTimeCompare(claimedNonceHash[:], nonceHash) != 1 {
		return Session{}, "", "", errors.New("OIDC nonce mismatch")
	}
	userID := "oidc:" + claims.Subject
	username := claims.Email
	if username == "" {
		username = userID
	}
	displayName := claims.Name
	if displayName == "" {
		displayName = username
	}
	// New OIDC users deliberately receive read-only access. An administrator
	// must explicitly grant mutation capabilities in user_accounts.
	_, err = tx.Exec(ctx, `INSERT INTO user_accounts(id,username,display_name,capabilities)
		VALUES($1,$2,$3,$4) ON CONFLICT(id) DO UPDATE SET username=excluded.username,
		display_name=excluded.display_name,updated_at=now()`, userID, username, displayName, []string{"resource:read"})
	if err != nil {
		return Session{}, "", "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return Session{}, "", "", err
	}
	user := User{ID: userID, Username: username, DisplayName: displayName, Capabilities: []string{"resource:read"}}
	session, sessionToken, err := m.CreateSession(ctx, user, claims.Subject)
	return session, sessionToken, returnTo, err
}

func (m *Manager) SetOIDCCookie(writer http.ResponseWriter, token string) {
	http.SetCookie(writer, &http.Cookie{Name: m.oidcCookieName(), Value: token, Path: "/", Secure: m.secureCookies, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 600})
}

func (m *Manager) OIDCCookie(request *http.Request) string {
	cookie, err := request.Cookie(m.oidcCookieName())
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (m *Manager) ClearOIDCCookie(writer http.ResponseWriter) {
	http.SetCookie(writer, &http.Cookie{Name: m.oidcCookieName(), Value: "", Path: "/", Secure: m.secureCookies, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

func (m *Manager) sessionCookieName() string {
	if m.secureCookies {
		return CookieName
	}
	return DevCookieName
}

func (m *Manager) oidcCookieName() string {
	if m.secureCookies {
		return OIDCCookieName
	}
	return DevOIDCCookieName
}

func HashPassword(password string) (string, error) {
	if len(password) < 12 {
		return "", errors.New("password must contain at least 12 characters")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=2$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[4])
	expected, err2 := base64.RawStdEncoding.DecodeString(parts[5])
	if err1 != nil || err2 != nil || len(expected) != 32 {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func randomToken(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (m *Manager) encrypt(plaintext []byte) ([]byte, error) {
	key := sha256.Sum256(m.sessionSecret)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, plaintext, nil), nil
}

func (m *Manager) decrypt(ciphertext []byte) ([]byte, error) {
	key := sha256.Sum256(m.sessionSecret)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < aead.NonceSize() {
		return nil, errors.New("invalid ciphertext")
	}
	return aead.Open(nil, ciphertext[:aead.NonceSize()], ciphertext[aead.NonceSize():], nil)
}

func safeReturnTo(value string) bool {
	return value == "" || (strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") && !strings.Contains(value, "\\"))
}

func EncodeSessionJSON(session Session) []byte {
	value, _ := json.Marshal(session)
	return value
}
