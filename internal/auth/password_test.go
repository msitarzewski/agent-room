package auth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	t.Parallel()
	encoded, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !verifyPassword(encoded, "correct horse battery staple") {
		t.Fatal("correct password rejected")
	}
	if verifyPassword(encoded, "incorrect horse battery staple") {
		t.Fatal("incorrect password accepted")
	}
}

func TestPasswordMinimumLength(t *testing.T) {
	t.Parallel()
	if _, err := HashPassword("too-short"); err == nil {
		t.Fatal("expected short password rejection")
	}
}

func TestPasswordVerifierRejectsMalformedHashes(t *testing.T) {
	t.Parallel()
	for _, encoded := range []string{
		"",
		"$scrypt$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=65536,t=3,p=2$%%%$aGFzaA",
		"$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$%%%",
		"$argon2id$v=19$m=65536,t=3,p=2$c2FsdA$c2hvcnQ",
	} {
		if verifyPassword(encoded, "correct horse battery staple") {
			t.Fatalf("malformed password hash accepted: %q", encoded)
		}
	}
}

func TestSessionCryptographyAndReturnPathValidation(t *testing.T) {
	t.Parallel()
	manager := &Manager{sessionSecret: []byte("a stable test secret with enough entropy")}

	first := manager.csrfForToken("session-token")
	second := manager.csrfForToken("session-token")
	if first == "" || first != second || first == manager.csrfForToken("other-token") {
		t.Fatal("CSRF derivation is not stable and token-bound")
	}

	plaintext := []byte("pkce-verifier")
	ciphertext, err := manager.encrypt(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := manager.decrypt(ciphertext)
	if err != nil || !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("decrypt=%q err=%v", decrypted, err)
	}
	ciphertext[len(ciphertext)-1] ^= 1
	if _, err := manager.decrypt(ciphertext); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
	if _, err := manager.decrypt([]byte("short")); err == nil {
		t.Fatal("truncated ciphertext accepted")
	}

	for _, value := range []string{"", "/", "/projects/one?tab=tasks"} {
		if !safeReturnTo(value) {
			t.Fatalf("safe return path rejected: %q", value)
		}
	}
	for _, value := range []string{"https://attacker.test", "//attacker.test", `/\attacker.test`, `\attacker.test`} {
		if safeReturnTo(value) {
			t.Fatalf("unsafe return path accepted: %q", value)
		}
	}
}

func TestCookieSecurityAttributes(t *testing.T) {
	t.Parallel()
	expires := time.Now().Add(time.Hour)

	manager := &Manager{secureCookies: true}
	recorder := httptest.NewRecorder()
	manager.SetCookie(recorder, "secret", expires)
	cookie := recorder.Result().Cookies()[0]
	if cookie.Name != CookieName || cookie.Value != "secret" || cookie.Path != "/" ||
		!cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode ||
		cookie.MaxAge <= 0 {
		t.Fatalf("insecure session cookie: %#v", cookie)
	}

	recorder = httptest.NewRecorder()
	manager.ClearCookie(recorder)
	cookie = recorder.Result().Cookies()[0]
	if cookie.Name != CookieName || cookie.MaxAge != -1 || !cookie.Secure || !cookie.HttpOnly {
		t.Fatalf("insecure session-cookie removal: %#v", cookie)
	}

	recorder = httptest.NewRecorder()
	manager.SetOIDCCookie(recorder, "browser")
	cookie = recorder.Result().Cookies()[0]
	if cookie.Name != OIDCCookieName || cookie.Path != "/" ||
		cookie.MaxAge != 600 || !cookie.Secure || !cookie.HttpOnly {
		t.Fatalf("insecure OIDC cookie: %#v", cookie)
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(cookie)
	if got := manager.OIDCCookie(request); got != "browser" {
		t.Fatalf("OIDC cookie=%q", got)
	}
	if got := manager.OIDCCookie(httptest.NewRequest(http.MethodGet, "/", nil)); got != "" {
		t.Fatalf("missing OIDC cookie=%q", got)
	}

	recorder = httptest.NewRecorder()
	manager.ClearOIDCCookie(recorder)
	if got := recorder.Result().Cookies()[0]; got.MaxAge != -1 || !got.Secure || !got.HttpOnly {
		t.Fatalf("insecure OIDC-cookie removal: %#v", got)
	}

	development := &Manager{}
	recorder = httptest.NewRecorder()
	development.SetCookie(recorder, "secret", expires)
	cookie = recorder.Result().Cookies()[0]
	if cookie.Name != DevCookieName || cookie.Secure || !cookie.HttpOnly {
		t.Fatalf("invalid development session cookie: %#v", cookie)
	}
	recorder = httptest.NewRecorder()
	development.SetOIDCCookie(recorder, "browser")
	cookie = recorder.Result().Cookies()[0]
	if cookie.Name != DevOIDCCookieName || cookie.Secure || !cookie.HttpOnly || cookie.Path != "/" {
		t.Fatalf("invalid development OIDC cookie: %#v", cookie)
	}
}

func TestUnconfiguredOIDCAndSessionJSON(t *testing.T) {
	t.Parallel()
	manager := &Manager{}
	if manager.OIDCConfigured() {
		t.Fatal("empty manager reported configured OIDC")
	}
	if _, _, err := manager.BeginOIDC(t.Context(), "/"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("BeginOIDC error=%v", err)
	}
	if _, _, _, err := manager.FinishOIDC(t.Context(), "", "", ""); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("FinishOIDC error=%v", err)
	}
	session := Session{User: User{ID: "user-one"}, CSRFToken: "csrf"}
	if encoded := EncodeSessionJSON(session); !bytes.Contains(encoded, []byte(`"id":"user-one"`)) {
		t.Fatalf("session JSON=%s", encoded)
	}
}
