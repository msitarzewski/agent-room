package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseFileRejectsUnknownDuplicateAndExpansion(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"unknown":   "AGENTROOM_UNKNOWN=x\n",
		"duplicate": "AGENTROOM_LOG_LEVEL=info\nAGENTROOM_LOG_LEVEL=debug\n",
		"expansion": "AGENTROOM_LOG_LEVEL=$(whoami)\n",
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "agentroom.conf")
			if err := os.WriteFile(filename, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := parseFile(filename); err == nil {
				t.Fatalf("expected %s config to fail", name)
			}
		})
	}
}

func TestParseFileAcceptsEscapedQuotedValues(t *testing.T) {
	t.Parallel()
	filename := filepath.Join(t.TempDir(), "agentroom.conf")
	if err := os.WriteFile(filename, []byte("AGENTROOM_LOG_LEVEL=\"de\\u0062ug\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	values, err := parseFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if values["AGENTROOM_LOG_LEVEL"] != "debug" {
		t.Fatalf("got %q", values["AGENTROOM_LOG_LEVEL"])
	}
}

func TestCredentialDirectoryExpansion(t *testing.T) {
	dir := t.TempDir()
	for name, value := range map[string]string{"db": "postgres://test", "session": strings.Repeat("s", 32)} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("CREDENTIALS_DIRECTORY", dir)
	cfg, err := fromValues(map[string]string{
		"AGENTROOM_MAX_REQUEST_BYTES": "1048576", "AGENTROOM_SHUTDOWN_TIMEOUT": "20s",
		"AGENTROOM_DATABASE_URL_FILE": "%d/db", "AGENTROOM_SESSION_SECRET_FILE": "%d/session",
		"AGENTROOM_HTTPS_ADDR": "127.0.0.1:8443", "AGENTROOM_ADMIN_ADDR": "127.0.0.1:9090",
		"AGENTROOM_DEV": "true",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://test" || len(cfg.SessionSecret) != 32 {
		t.Fatalf("credentials were not loaded: %#v", cfg)
	}
}

func validDevelopmentConfig() Config {
	return Config{
		HTTPSAddr: "127.0.0.1:8080", AdminAddr: "127.0.0.1:9090", AdapterAddr: "localhost:9091",
		PublicURL: "http://127.0.0.1:8080", AllowedOrigins: []string{"http://127.0.0.1:8080"},
		DatabaseURL: "postgres://test", SessionSecret: []byte(strings.Repeat("s", 32)),
		MaxRequestBytes: 1 << 20, MaxArtifactBytes: 64 << 20, MaxConcurrentRuns: 4,
		ShutdownTimeout: 20 * time.Second, Dev: true,
	}
}

func TestConfigValidationRejectsUnsafeBoundaries(t *testing.T) {
	t.Parallel()
	valid := validDevelopmentConfig()
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid development config: %v", err)
	}
	cases := map[string]func(*Config){
		"database":           func(c *Config) { c.DatabaseURL = "" },
		"session":            func(c *Config) { c.SessionSecret = []byte("short") },
		"request-small":      func(c *Config) { c.MaxRequestBytes = 1 },
		"request-large":      func(c *Config) { c.MaxRequestBytes = 65 << 20 },
		"artifact-small":     func(c *Config) { c.MaxArtifactBytes = 1 },
		"artifact-large":     func(c *Config) { c.MaxArtifactBytes = 2 << 30 },
		"concurrency-small":  func(c *Config) { c.MaxConcurrentRuns = 0 },
		"concurrency-large":  func(c *Config) { c.MaxConcurrentRuns = 65 },
		"shutdown-zero":      func(c *Config) { c.ShutdownTimeout = 0 },
		"shutdown-large":     func(c *Config) { c.ShutdownTimeout = 6 * time.Minute },
		"public-credentials": func(c *Config) { c.PublicURL = "http://user@localhost:8080" },
		"public-path":        func(c *Config) { c.PublicURL = "http://localhost:8080/path" },
		"proxy-cidr":         func(c *Config) { c.TrustedProxyCIDRs = []string{"not-a-cidr"} },
		"origin":             func(c *Config) { c.AllowedOrigins = []string{"http://localhost:8080/path"} },
		"dev-public-bind":    func(c *Config) { c.HTTPSAddr = "0.0.0.0:8080" },
		"admin-public-bind":  func(c *Config) { c.AdminAddr = "0.0.0.0:9090" },
		"adapter-public":     func(c *Config) { c.AdapterAddr = "0.0.0.0:9091" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := valid
			mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("unsafe configuration accepted")
			}
		})
	}
}

func TestProductionValidationRequiresExternalSecurityBoundaries(t *testing.T) {
	t.Parallel()
	valid := validDevelopmentConfig()
	valid.Dev = false
	valid.PublicURL = "https://agentroom.test"
	valid.AllowedOrigins = []string{"https://agentroom.test"}
	valid.TLSCertFile, valid.TLSKeyFile, valid.TLSClientCAFile = "cert", "key", "ca"
	valid.OIDCIssuer, valid.OIDCClientID, valid.OIDCClientSecret = "https://id.test", "client", "secret"
	valid.OIDCRedirectURL = "https://agentroom.test/api/v1/auth/callback"
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid production config: %v", err)
	}
	cases := map[string]func(*Config){
		"public-url":      func(c *Config) { c.PublicURL = "" },
		"plaintext-url":   func(c *Config) { c.PublicURL = "http://agentroom.test" },
		"managed-runtime": func(c *Config) { c.CodexBin = "/usr/bin/codex" },
		"tls":             func(c *Config) { c.TLSCertFile = "" },
		"oidc":            func(c *Config) { c.OIDCIssuer = "" },
		"origins":         func(c *Config) { c.AllowedOrigins = nil },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := valid
			mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("incomplete production configuration accepted")
			}
		})
	}
}

func TestParsingAndSecretFailures(t *testing.T) {
	for _, args := range [][]string{
		{"--unknown=x"}, {"--dev=true"}, {"--config"}, {"--https-addr"},
	} {
		if _, _, _, err := parseCLI(args); err == nil {
			t.Fatalf("invalid CLI accepted: %v", args)
		}
	}
	path, values, rest, err := parseCLI([]string{
		"serve", "--config=agentroom.conf", "--dev", "--https-addr", "localhost:8080",
		"--admin-addr=localhost:9090", "--adapter-addr", "localhost:9091",
		"--public-url=http://localhost:8080",
	})
	if err != nil || path != "agentroom.conf" || !strings.EqualFold(values["AGENTROOM_DEV"], "true") ||
		len(rest) != 1 || rest[0] != "serve" {
		t.Fatalf("path=%q values=%v rest=%v err=%v", path, values, rest, err)
	}
	base := map[string]string{
		"AGENTROOM_MAX_REQUEST_BYTES": "1048576", "AGENTROOM_MAX_ARTIFACT_BYTES": "67108864",
		"AGENTROOM_MAX_CONCURRENT_RUNS": "4", "AGENTROOM_SHUTDOWN_TIMEOUT": "20s", "AGENTROOM_DEV": "true",
	}
	for key, value := range map[string]string{
		"AGENTROOM_MAX_REQUEST_BYTES": "bad", "AGENTROOM_MAX_ARTIFACT_BYTES": "bad",
		"AGENTROOM_MAX_CONCURRENT_RUNS": "bad", "AGENTROOM_SHUTDOWN_TIMEOUT": "bad",
		"AGENTROOM_DEV": "bad",
	} {
		input := make(map[string]string, len(base))
		merge(input, base)
		input[key] = value
		if _, err := fromValues(input); err == nil {
			t.Fatalf("invalid %s accepted", key)
		}
	}
	credential := make(map[string]string, len(base))
	merge(credential, base)
	credential["AGENTROOM_DATABASE_URL_FILE"] = "%d/database"
	t.Setenv("CREDENTIALS_DIRECTORY", "")
	if _, err := fromValues(credential); err == nil {
		t.Fatal("credential placeholder without directory accepted")
	}
	credential["AGENTROOM_DATABASE_URL_FILE"] = "prefix-%d/database"
	if _, err := fromValues(credential); err == nil {
		t.Fatal("credential placeholder outside prefix accepted")
	}
}

func TestRegularFileConfinementAndConfigSyntax(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if _, err := ReadRegularFile(root); err == nil {
		t.Fatal("directory accepted as a regular file")
	}
	if _, err := readRootFile(root, "../escape"); err == nil {
		t.Fatal("parent traversal accepted")
	}
	invalidUTF8 := filepath.Join(root, "invalid.conf")
	if err := os.WriteFile(invalidUTF8, []byte{0xff}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := parseFile(invalidUTF8); err == nil {
		t.Fatal("invalid UTF-8 config accepted")
	}
	for name, content := range map[string]string{
		"missing-equals": "AGENTROOM_DEV\n",
		"invalid-quote":  "AGENTROOM_LOG_LEVEL=\"unterminated\n",
	} {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := parseFile(path); err == nil {
			t.Fatalf("%s config accepted", name)
		}
	}
	if got := splitCSV(" one, ,two "); len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("split CSV=%v", got)
	}
	if splitCSV("") != nil || defaultString("", "fallback") != "fallback" || defaultString("set", "fallback") != "set" {
		t.Fatal("default parsing helpers returned unexpected values")
	}
}

func TestLoadAppliesFileAndCLIPrecedence(t *testing.T) {
	for key := range known {
		value, existed := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
		savedKey, savedValue, savedExisted := key, value, existed
		t.Cleanup(func() {
			if savedExisted {
				_ = os.Setenv(savedKey, savedValue)
			} else {
				_ = os.Unsetenv(savedKey)
			}
		})
	}
	root := t.TempDir()
	database := filepath.Join(root, "database-url")
	session := filepath.Join(root, "session-secret")
	if err := os.WriteFile(database, []byte("postgres://agentroom-test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(session, []byte(strings.Repeat("s", 32)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "agentroom.conf")
	configText := strings.Join([]string{
		"AGENTROOM_DATABASE_URL_FILE=" + database,
		"AGENTROOM_SESSION_SECRET_FILE=" + session,
		"AGENTROOM_DEV=true",
		"AGENTROOM_PUBLIC_URL=http://127.0.0.1:7000",
		"AGENTROOM_ALLOWED_ORIGINS=http://localhost:8080",
		"",
	}, "\n")
	if err := os.WriteFile(configPath, []byte(configText), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, rest, err := Load([]string{
		"serve", "--config", configPath, "--https-addr=localhost:8080",
		"--public-url", "http://localhost:8080",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConfigPath != configPath || cfg.PublicURL != "http://localhost:8080" ||
		cfg.HTTPSAddr != "localhost:8080" || cfg.DatabaseURL != "postgres://agentroom-test" ||
		len(cfg.SessionSecret) != 32 || len(rest) != 1 || rest[0] != "serve" {
		t.Fatalf("cfg=%+v rest=%v", cfg, rest)
	}
}
