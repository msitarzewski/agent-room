package config

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type Config struct {
	HTTPSAddr         string
	AdminAddr         string
	AdapterAddr       string
	PublicURL         string
	AllowedOrigins    []string
	TrustedProxyCIDRs []string
	OIDCIssuer        string
	OIDCClientID      string
	OIDCRedirectURL   string
	ArtifactDir       string
	WebDir            string
	WorkspaceRoot     string
	CodexBin          string
	ClaudeBin         string
	MaxConcurrentRuns int
	LogLevel          string
	OTLPEndpoint      string
	MaxRequestBytes   int64
	MaxArtifactBytes  int64
	ShutdownTimeout   time.Duration
	DatabaseURL       string
	SessionSecret     []byte
	OIDCClientSecret  string
	TLSCertFile       string
	TLSKeyFile        string
	TLSClientCAFile   string
	Dev               bool
	ConfigPath        string
}

var known = map[string]struct{}{
	"AGENTROOM_HTTPS_ADDR": {}, "AGENTROOM_ADMIN_ADDR": {}, "AGENTROOM_PUBLIC_URL": {},
	"AGENTROOM_ADAPTER_ADDR":        {},
	"AGENTROOM_TRUSTED_PROXY_CIDRS": {},
	"AGENTROOM_ALLOWED_ORIGINS":     {}, "AGENTROOM_OIDC_ISSUER": {}, "AGENTROOM_OIDC_CLIENT_ID": {},
	"AGENTROOM_OIDC_REDIRECT_URL": {}, "AGENTROOM_ARTIFACT_DIR": {}, "AGENTROOM_LOG_LEVEL": {},
	"AGENTROOM_WEB_DIR":        {},
	"AGENTROOM_WORKSPACE_ROOT": {}, "AGENTROOM_CODEX_BIN": {}, "AGENTROOM_CLAUDE_BIN": {},
	"AGENTROOM_MAX_CONCURRENT_RUNS":         {},
	"AGENTROOM_OTEL_EXPORTER_OTLP_ENDPOINT": {}, "AGENTROOM_MAX_REQUEST_BYTES": {},
	"AGENTROOM_MAX_ARTIFACT_BYTES": {},
	"AGENTROOM_SHUTDOWN_TIMEOUT":   {}, "AGENTROOM_DATABASE_URL_FILE": {},
	"AGENTROOM_SESSION_SECRET_FILE": {}, "AGENTROOM_OIDC_CLIENT_SECRET_FILE": {},
	"AGENTROOM_TLS_CERT_FILE": {}, "AGENTROOM_TLS_KEY_FILE": {}, "AGENTROOM_TLS_CLIENT_CA_FILE": {},
	"AGENTROOM_DEV": {},
}

func Load(args []string) (Config, []string, error) {
	values := map[string]string{
		"AGENTROOM_HTTPS_ADDR": ":8443", "AGENTROOM_ADMIN_ADDR": "127.0.0.1:9090",
		"AGENTROOM_ADAPTER_ADDR": "127.0.0.1:9091",
		"AGENTROOM_ARTIFACT_DIR": "/var/lib/agentroom/artifacts", "AGENTROOM_LOG_LEVEL": "info",
		"AGENTROOM_WEB_DIR":             "/opt/agentroom/current/web",
		"AGENTROOM_WORKSPACE_ROOT":      "/var/lib/agentroom/workspaces",
		"AGENTROOM_MAX_CONCURRENT_RUNS": "4",
		"AGENTROOM_MAX_REQUEST_BYTES":   "1048576", "AGENTROOM_SHUTDOWN_TIMEOUT": "20s",
		"AGENTROOM_MAX_ARTIFACT_BYTES": "67108864",
	}
	configPath, cli, rest, err := parseCLI(args)
	if err != nil {
		return Config{}, nil, err
	}
	if configPath != "" {
		fileValues, err := parseFile(configPath)
		if err != nil {
			return Config{}, nil, err
		}
		merge(values, fileValues)
	}
	for key := range known {
		if value, ok := os.LookupEnv(key); ok {
			values[key] = value
		}
	}
	merge(values, cli)
	cfg, err := fromValues(values)
	if err != nil {
		return Config{}, nil, err
	}
	cfg.ConfigPath = configPath
	return cfg, rest, cfg.Validate()
}

func parseCLI(args []string) (string, map[string]string, []string, error) {
	var configPath string
	values := map[string]string{}
	var rest []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			rest = append(rest, arg)
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		if name == "dev" {
			if hasValue {
				return "", nil, nil, errors.New("--dev does not take a value")
			}
			values["AGENTROOM_DEV"] = "true"
			continue
		}
		if !hasValue {
			i++
			if i >= len(args) {
				return "", nil, nil, fmt.Errorf("--%s requires a value", name)
			}
			value = args[i]
		}
		switch name {
		case "config":
			configPath = value
		case "https-addr":
			values["AGENTROOM_HTTPS_ADDR"] = value
		case "admin-addr":
			values["AGENTROOM_ADMIN_ADDR"] = value
		case "adapter-addr":
			values["AGENTROOM_ADAPTER_ADDR"] = value
		case "public-url":
			values["AGENTROOM_PUBLIC_URL"] = value
		default:
			return "", nil, nil, fmt.Errorf("unknown flag --%s", name)
		}
	}
	return configPath, values, rest, nil
}

func parseFile(filename string) (map[string]string, error) {
	content, err := ReadRegularFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", filename, err)
	}
	if !utf8.Valid(content) {
		return nil, errors.New("config must be valid UTF-8")
	}
	values := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("config line %d: expected AGENTROOM_KEY=value", lineNo)
		}
		if _, ok := known[key]; !ok {
			return nil, fmt.Errorf("config line %d: unknown key %s", lineNo, key)
		}
		if _, duplicate := values[key]; duplicate {
			return nil, fmt.Errorf("config line %d: duplicate key %s", lineNo, key)
		}
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, `"`) {
			unquoted, err := strconv.Unquote(value)
			if err != nil {
				return nil, fmt.Errorf("config line %d: invalid quoted value: %w", lineNo, err)
			}
			value = unquoted
		} else if strings.ContainsAny(value, "`$") {
			return nil, fmt.Errorf("config line %d: shell expansion is not supported", lineNo)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func merge(dst, src map[string]string) {
	for key, value := range src {
		dst[key] = value
	}
}

func fromValues(v map[string]string) (Config, error) {
	maxBytes, err := strconv.ParseInt(v["AGENTROOM_MAX_REQUEST_BYTES"], 10, 64)
	if err != nil {
		return Config{}, fmt.Errorf("AGENTROOM_MAX_REQUEST_BYTES: %w", err)
	}
	maxArtifactBytes, err := strconv.ParseInt(defaultString(v["AGENTROOM_MAX_ARTIFACT_BYTES"], "67108864"), 10, 64)
	if err != nil {
		return Config{}, fmt.Errorf("AGENTROOM_MAX_ARTIFACT_BYTES: %w", err)
	}
	maxConcurrentRuns, err := strconv.Atoi(defaultString(v["AGENTROOM_MAX_CONCURRENT_RUNS"], "4"))
	if err != nil {
		return Config{}, fmt.Errorf("AGENTROOM_MAX_CONCURRENT_RUNS: %w", err)
	}
	shutdown, err := time.ParseDuration(v["AGENTROOM_SHUTDOWN_TIMEOUT"])
	if err != nil {
		return Config{}, fmt.Errorf("AGENTROOM_SHUTDOWN_TIMEOUT: %w", err)
	}
	dev, err := strconv.ParseBool(defaultString(v["AGENTROOM_DEV"], "false"))
	if err != nil {
		return Config{}, fmt.Errorf("AGENTROOM_DEV: %w", err)
	}
	readSecret := func(key string) ([]byte, error) {
		filename := v[key]
		if filename == "" {
			return nil, nil
		}
		if strings.Contains(filename, "%d") {
			dir := os.Getenv("CREDENTIALS_DIRECTORY")
			if dir == "" {
				return nil, fmt.Errorf("%s uses %%d but CREDENTIALS_DIRECTORY is empty", key)
			}
			if filename != "%d" && !strings.HasPrefix(filename, "%d/") {
				return nil, fmt.Errorf("%s must use %%d only as the credential-directory prefix", key)
			}
			relative := strings.TrimPrefix(strings.TrimPrefix(filename, "%d"), "/")
			value, err := readRootFile(dir, relative)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", key, err)
			}
			return bytes.TrimSuffix(value, []byte("\n")), nil
		}
		value, err := ReadRegularFile(filename)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", key, err)
		}
		return bytes.TrimSuffix(value, []byte("\n")), nil
	}
	databaseURL, err := readSecret("AGENTROOM_DATABASE_URL_FILE")
	if err != nil {
		return Config{}, err
	}
	sessionSecret, err := readSecret("AGENTROOM_SESSION_SECRET_FILE")
	if err != nil {
		return Config{}, err
	}
	oidcSecret, err := readSecret("AGENTROOM_OIDC_CLIENT_SECRET_FILE")
	if err != nil {
		return Config{}, err
	}
	return Config{
		HTTPSAddr: v["AGENTROOM_HTTPS_ADDR"], AdminAddr: v["AGENTROOM_ADMIN_ADDR"], AdapterAddr: v["AGENTROOM_ADAPTER_ADDR"], PublicURL: v["AGENTROOM_PUBLIC_URL"],
		AllowedOrigins: splitCSV(v["AGENTROOM_ALLOWED_ORIGINS"]), TrustedProxyCIDRs: splitCSV(v["AGENTROOM_TRUSTED_PROXY_CIDRS"]), OIDCIssuer: v["AGENTROOM_OIDC_ISSUER"],
		OIDCClientID: v["AGENTROOM_OIDC_CLIENT_ID"], OIDCRedirectURL: v["AGENTROOM_OIDC_REDIRECT_URL"],
		ArtifactDir: v["AGENTROOM_ARTIFACT_DIR"], WebDir: v["AGENTROOM_WEB_DIR"], WorkspaceRoot: v["AGENTROOM_WORKSPACE_ROOT"],
		CodexBin: v["AGENTROOM_CODEX_BIN"], ClaudeBin: v["AGENTROOM_CLAUDE_BIN"], MaxConcurrentRuns: maxConcurrentRuns,
		LogLevel: v["AGENTROOM_LOG_LEVEL"], OTLPEndpoint: v["AGENTROOM_OTEL_EXPORTER_OTLP_ENDPOINT"],
		MaxRequestBytes: maxBytes, MaxArtifactBytes: maxArtifactBytes, ShutdownTimeout: shutdown, DatabaseURL: string(databaseURL), SessionSecret: sessionSecret,
		OIDCClientSecret: string(oidcSecret), TLSCertFile: v["AGENTROOM_TLS_CERT_FILE"], TLSKeyFile: v["AGENTROOM_TLS_KEY_FILE"],
		TLSClientCAFile: v["AGENTROOM_TLS_CLIENT_CA_FILE"], Dev: dev,
	}, nil
}

func ReadRegularFile(filename string) ([]byte, error) {
	clean := filepath.Clean(filename)
	return readRootFile(filepath.Dir(clean), filepath.Base(clean))
}

func readRootFile(directory, name string) ([]byte, error) {
	if name == "" || name == "." || filepath.IsAbs(name) {
		return nil, errors.New("file name must be relative to its confined root")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("path is not a regular file")
	}
	return io.ReadAll(io.LimitReader(file, 16<<20))
}

func (c Config) Validate() error {
	if c.DatabaseURL == "" {
		return errors.New("AGENTROOM_DATABASE_URL_FILE is required")
	}
	if len(c.SessionSecret) < 32 {
		return errors.New("session secret must contain at least 32 bytes")
	}
	if c.MaxRequestBytes < 1024 || c.MaxRequestBytes > 64<<20 {
		return errors.New("max request bytes must be between 1 KiB and 64 MiB")
	}
	if c.MaxArtifactBytes < 1024 || c.MaxArtifactBytes > 1<<30 {
		return errors.New("max artifact bytes must be between 1 KiB and 1 GiB")
	}
	if c.MaxConcurrentRuns < 1 || c.MaxConcurrentRuns > 64 {
		return errors.New("maximum concurrent runs must be between 1 and 64")
	}
	if c.ShutdownTimeout <= 0 || c.ShutdownTimeout > 5*time.Minute {
		return errors.New("shutdown timeout must be positive and at most five minutes")
	}
	if c.PublicURL != "" {
		u, err := url.Parse(c.PublicURL)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
			(u.Path != "" && u.Path != "/") || (u.Scheme != "https" && !(c.Dev && u.Scheme == "http")) {
			return errors.New("public URL must be an absolute http(s) origin without credentials, path, query, or fragment")
		}
	} else if !c.Dev {
		return errors.New("production requires a public URL")
	}
	for _, cidr := range c.TrustedProxyCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("invalid trusted proxy CIDR %q", cidr)
		}
	}
	for _, origin := range c.AllowedOrigins {
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
			(parsed.Path != "" && parsed.Path != "/") || (parsed.Scheme != "https" && !(c.Dev && parsed.Scheme == "http")) {
			return fmt.Errorf("invalid allowed origin %q", origin)
		}
	}
	if c.Dev {
		host, _, err := net.SplitHostPort(c.HTTPSAddr)
		if err != nil || (host != "127.0.0.1" && host != "localhost" && host != "[::1]" && host != "::1") {
			return errors.New("dev plaintext listener must bind to loopback")
		}
	} else {
		if c.CodexBin != "" || c.ClaudeBin != "" {
			return errors.New("production refuses in-process managed runtimes; use an isolated worker service")
		}
		if c.TLSCertFile == "" || c.TLSKeyFile == "" || c.TLSClientCAFile == "" {
			return errors.New("production requires TLS certificate, key, and client CA files")
		}
		if c.OIDCIssuer == "" || c.OIDCClientID == "" || c.OIDCClientSecret == "" || c.OIDCRedirectURL == "" {
			return errors.New("production requires complete OIDC configuration")
		}
		if len(c.AllowedOrigins) == 0 {
			return errors.New("production requires an explicit allowed origin list")
		}
	}
	adminHost, _, err := net.SplitHostPort(c.AdminAddr)
	if err != nil || (adminHost != "127.0.0.1" && adminHost != "localhost" && adminHost != "[::1]" && adminHost != "::1") {
		return errors.New("admin listener must bind to loopback")
	}
	adapterHost, _, err := net.SplitHostPort(c.AdapterAddr)
	if err != nil || (adapterHost != "127.0.0.1" && adapterHost != "localhost" && adapterHost != "[::1]" && adapterHost != "::1") {
		return errors.New("adapter listener must bind to loopback; use a private encrypted tunnel for remote adapters")
	}
	return nil
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	var values []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			values = append(values, item)
		}
	}
	return values
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
