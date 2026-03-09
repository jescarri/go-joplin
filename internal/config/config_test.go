package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandEnv(t *testing.T) {
	tests := []struct {
		name string
		s    string
		env  map[string]string
		want string
	}{
		{"empty", "", nil, ""},
		{"no expansion", "hello", nil, "hello"},
		{"one var", "${FOO}", map[string]string{"FOO": "bar"}, "bar"},
		{"missing var", "${MISSING}", nil, ""},
		{"mixed", "pre${A}mid${B}suf", map[string]string{"A": "1", "B": "2"}, "pre1mid2suf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				os.Setenv(k, v)
				defer os.Unsetenv(k)
			}
			got := ExpandEnv(tt.s)
			if got != tt.want {
				t.Errorf("ExpandEnv(%q) = %q, want %q", tt.s, got, tt.want)
			}
		})
	}
}

func TestIsYAMLConfig(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"config.yaml", true},
		{"config.yml", true},
		{"dir/config.YAML", true},
		{"settings.json", false},
		{"config.json", false},
		{"noext", false},
	}
	for _, tt := range tests {
		got := isYAMLConfig(tt.path)
		if got != tt.want {
			t.Errorf("isYAMLConfig(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestLoadFromJoplinJSON(t *testing.T) {
	json := `{
		"sync.target": 8,
		"sync.8.path": "mybucket",
		"sync.8.url": "https://s3.example.com",
		"sync.8.region": "us-east-1",
		"sync.8.username": "access",
		"sync.8.password": "secret",
		"sync.8.forcePathStyle": true,
		"api.token": "token123"
	}`
	cfg, err := loadFromJoplinJSON([]byte(json))
	if err != nil {
		t.Fatalf("loadFromJoplinJSON: %v", err)
	}
	if cfg.SyncTarget != 8 {
		t.Errorf("SyncTarget = %d, want 8", cfg.SyncTarget)
	}
	if cfg.S3Bucket != "mybucket" {
		t.Errorf("S3Bucket = %q, want mybucket", cfg.S3Bucket)
	}
	if cfg.S3URL != "https://s3.example.com" {
		t.Errorf("S3URL = %q", cfg.S3URL)
	}
	if cfg.S3AccessKey != "access" || cfg.S3SecretKey != "secret" {
		t.Errorf("S3 credentials: got %q / %q", cfg.S3AccessKey, cfg.S3SecretKey)
	}
	if cfg.APIToken != "token123" {
		t.Errorf("APIToken = %q", cfg.APIToken)
	}
	// Observability is not set by loadFromJoplinJSON (caller sets DefaultObservability)
	if cfg.Observability.Tracing.ServiceName != "" {
		t.Errorf("expected Observability not set by JSON loader, got ServiceName %q", cfg.Observability.Tracing.ServiceName)
	}
}

func TestLoadFromYAML_Observability(t *testing.T) {
	yaml := `
sync:
  target: 8
  s3:
    bucket: "b"
    url: "https://s3.example.com"
    region: "us-east-1"
  joplin_server:
    url: ""
api:
  token: "t"
  key: "k"
server:
  port: 41184
observability:
  tracing:
    enabled: true
    protocol: "grpc"
    endpoint: "http://otel:4317"
    service_name: "my-service"
    sample_rate: 0.5
  metrics:
    enabled: true
    prometheus_port: 9200
mcp:
  allow_folders: ""
`
	os.Setenv("AWS_ACCESS_KEY_ID", "ak")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "sk")
	defer os.Unsetenv("AWS_ACCESS_KEY_ID")
	defer os.Unsetenv("AWS_SECRET_ACCESS_KEY")

	cfg, err := loadFromYAML([]byte(yaml))
	if err != nil {
		t.Fatalf("loadFromYAML: %v", err)
	}
	tr := &cfg.Observability.Tracing
	if !tr.Enabled {
		t.Error("Tracing.Enabled want true")
	}
	if tr.Protocol != "grpc" {
		t.Errorf("Tracing.Protocol = %q, want grpc", tr.Protocol)
	}
	if tr.Endpoint != "http://otel:4317" {
		t.Errorf("Tracing.Endpoint = %q, want http://otel:4317", tr.Endpoint)
	}
	if tr.ServiceName != "my-service" {
		t.Errorf("Tracing.ServiceName = %q, want my-service", tr.ServiceName)
	}
	if tr.SampleRate != 0.5 {
		t.Errorf("Tracing.SampleRate = %v, want 0.5", tr.SampleRate)
	}
	if cfg.Observability.Metrics.PrometheusPort != 9200 {
		t.Errorf("Metrics.PrometheusPort = %d, want 9200", cfg.Observability.Metrics.PrometheusPort)
	}
}

func TestLoadFromYAML_EndpointExpandEnv(t *testing.T) {
	yaml := `
sync:
  target: 8
  s3:
    bucket: "b"
    url: "https://s3.example.com"
    region: "us-east-1"
api:
  token: "t"
  key: "k"
observability:
  tracing:
    enabled: true
    endpoint: "${OTEL_ENDPOINT}"
`
	os.Setenv("AWS_ACCESS_KEY_ID", "ak")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "sk")
	os.Setenv("OTEL_ENDPOINT", "http://custom:4318")
	defer os.Unsetenv("AWS_ACCESS_KEY_ID")
	defer os.Unsetenv("AWS_SECRET_ACCESS_KEY")
	defer os.Unsetenv("OTEL_ENDPOINT")

	cfg, err := loadFromYAML([]byte(yaml))
	if err != nil {
		t.Fatalf("loadFromYAML: %v", err)
	}
	if cfg.Observability.Tracing.Endpoint != "http://custom:4318" {
		t.Errorf("Tracing.Endpoint = %q, want http://custom:4318", cfg.Observability.Tracing.Endpoint)
	}
}

func TestValidateConfig(t *testing.T) {
	t.Run("invalid sync target", func(t *testing.T) {
		cfg := &Config{SyncTarget: 7}
		err := validateConfig(cfg)
		if err == nil {
			t.Fatal("expected error for sync target 7")
		}
	})
	t.Run("target 9 missing URL", func(t *testing.T) {
		cfg := &Config{SyncTarget: 9, ServerURL: ""}
		err := validateConfig(cfg)
		if err == nil {
			t.Fatal("expected error for missing server URL")
		}
	})
	t.Run("target 8 missing bucket", func(t *testing.T) {
		cfg := &Config{SyncTarget: 8, S3Bucket: ""}
		err := validateConfig(cfg)
		if err == nil {
			t.Fatal("expected error for missing S3 bucket")
		}
	})
	t.Run("target 8 missing credentials", func(t *testing.T) {
		cfg := &Config{SyncTarget: 8, S3Bucket: "b", S3URL: "https://s3.example.com", S3Region: "us-east-1"}
		os.Unsetenv("AWS_ACCESS_KEY_ID")
		os.Unsetenv("AWS_SECRET_ACCESS_KEY")
		err := validateConfig(cfg)
		if err == nil {
			t.Fatal("expected error for missing S3 credentials")
		}
	})
	t.Run("target 8 ok with env credentials", func(t *testing.T) {
		cfg := &Config{SyncTarget: 8, S3Bucket: "b", S3URL: "https://s3.example.com", S3Region: "us-east-1"}
		os.Setenv("AWS_ACCESS_KEY_ID", "ak")
		os.Setenv("AWS_SECRET_ACCESS_KEY", "sk")
		defer os.Unsetenv("AWS_ACCESS_KEY_ID")
		defer os.Unsetenv("AWS_SECRET_ACCESS_KEY")
		err := validateConfig(cfg)
		if err != nil {
			t.Fatalf("validateConfig: %v", err)
		}
	})
	t.Run("target 9 ok", func(t *testing.T) {
		cfg := &Config{SyncTarget: 9, ServerURL: "https://joplin.example.com"}
		err := validateConfig(cfg)
		if err != nil {
			t.Fatalf("validateConfig: %v", err)
		}
	})
}

func TestLoad_JSON_SetsObservability(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	content := `{
		"sync.target": 9,
		"sync.9.path": "https://joplin.example.com",
		"api.token": "tok"
	}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// JSON path: Load sets DefaultObservability() after loadFromJoplinJSON
	if cfg.Observability.Tracing.Exporter != "otlp" {
		t.Errorf("Tracing.Exporter = %q, want otlp", cfg.Observability.Tracing.Exporter)
	}
	if cfg.Observability.Tracing.Protocol != "http" {
		t.Errorf("Tracing.Protocol = %q, want http", cfg.Observability.Tracing.Protocol)
	}
	if cfg.Observability.Metrics.Exporter != "prometheus" {
		t.Errorf("Metrics.Exporter = %q", cfg.Observability.Metrics.Exporter)
	}
	if cfg.Observability.Metrics.PrometheusPort != 9091 {
		t.Errorf("Metrics.PrometheusPort = %d, want 9091", cfg.Observability.Metrics.PrometheusPort)
	}
}

func TestLoad_YAML_FullObservabilityPassed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
sync:
  target: 8
  s3:
    bucket: "testbucket"
    url: "https://s3.example.com"
    region: "us-east-1"
api:
  token: "t"
  key: "k"
observability:
  tracing:
    enabled: true
    protocol: "http"
    endpoint: "http://localhost:4318"
    service_name: "go-joplin"
    sample_rate: 1.0
  metrics:
    enabled: true
    prometheus_port: 9091
`
	os.Setenv("AWS_ACCESS_KEY_ID", "testak")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "testsk")
	oldOtel := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	os.Unsetenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	defer func() {
		os.Unsetenv("AWS_ACCESS_KEY_ID")
		os.Unsetenv("AWS_SECRET_ACCESS_KEY")
		if oldOtel != "" {
			os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", oldOtel)
		}
	}()

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	tr := &cfg.Observability.Tracing
	if tr.Protocol != "http" {
		t.Errorf("Tracing.Protocol = %q, want http", tr.Protocol)
	}
	if tr.Endpoint != "http://localhost:4318" {
		t.Errorf("Tracing.Endpoint = %q", tr.Endpoint)
	}
	if tr.ServiceName != "go-joplin" {
		t.Errorf("Tracing.ServiceName = %q", tr.ServiceName)
	}
	if tr.SampleRate != 1.0 {
		t.Errorf("Tracing.SampleRate = %v", tr.SampleRate)
	}
}

func TestListenAddr(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{"defaults", Config{}, "localhost:41184"},
		{"custom port", Config{Port: 9999}, "localhost:9999"},
		{"custom host", Config{ListenHost: "0.0.0.0", Port: 41184}, "0.0.0.0:41184"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.ListenAddr()
			if got != tt.want {
				t.Errorf("ListenAddr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoad_YAML_MasterPasswordFromEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
sync:
  target: 8
  s3:
    bucket: "b"
    url: "https://s3.example.com"
    region: "us-east-1"
  joplin_server:
    url: ""
api:
  token: "t"
  key: "k"
server:
  port: 41184
observability:
  tracing:
    enabled: false
  metrics:
    enabled: true
    prometheus_port: 9091
mcp: {}
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	os.Setenv("AWS_ACCESS_KEY_ID", "ak")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "sk")
	os.Setenv("GOJOPLIN_MASTER_PASSWORD", "env-master-secret")
	defer func() {
		os.Unsetenv("AWS_ACCESS_KEY_ID")
		os.Unsetenv("AWS_SECRET_ACCESS_KEY")
		os.Unsetenv("GOJOPLIN_MASTER_PASSWORD")
	}()

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.MasterPassword != "env-master-secret" {
		t.Errorf("MasterPassword = %q, want env-master-secret", cfg.MasterPassword)
	}
}

// TestLoad_YAML_MasterPasswordNoTrim ensures we do not trim GOJOPLIN_MASTER_PASSWORD
// so E2EE key derivation matches desktop (see docs/e2ee-config-delta-from-main.md).
func TestLoad_YAML_MasterPasswordNoTrim(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
sync:
  target: 8
  s3:
    bucket: "b"
    url: "https://s3.example.com"
    region: "us-east-1"
  joplin_server:
    url: ""
api:
  token: "t"
  key: "k"
server:
  port: 41184
observability:
  tracing:
    enabled: false
  metrics:
    enabled: true
    prometheus_port: 9091
mcp: {}
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	rawPassword := "trimmed-secret\n\t "
	os.Setenv("AWS_ACCESS_KEY_ID", "ak")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "sk")
	os.Setenv("GOJOPLIN_MASTER_PASSWORD", rawPassword)
	defer func() {
		os.Unsetenv("AWS_ACCESS_KEY_ID")
		os.Unsetenv("AWS_SECRET_ACCESS_KEY")
		os.Unsetenv("GOJOPLIN_MASTER_PASSWORD")
	}()

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.MasterPassword != rawPassword {
		t.Errorf("MasterPassword = %q, want %q (no trim to match desktop E2EE)", cfg.MasterPassword, rawPassword)
	}
}

func TestLoadFromYAML_MasterPasswordFromConfig(t *testing.T) {
	yaml := `
sync:
  target: 8
  s3:
    bucket: "b"
    url: "https://s3.example.com"
    region: "us-east-1"
  joplin_server:
    url: ""
api:
  token: "t"
  key: "k"
  master_password: "${E2EE_PWD}"
server:
  port: 41184
observability:
  tracing:
    enabled: false
  metrics:
    enabled: true
    prometheus_port: 9091
mcp: {}
`
	os.Setenv("AWS_ACCESS_KEY_ID", "ak")
	os.Setenv("AWS_SECRET_ACCESS_KEY", "sk")
	os.Setenv("E2EE_PWD", "from-expand")
	defer func() {
		os.Unsetenv("AWS_ACCESS_KEY_ID")
		os.Unsetenv("AWS_SECRET_ACCESS_KEY")
		os.Unsetenv("E2EE_PWD")
	}()

	cfg, err := loadFromYAML([]byte(yaml))
	if err != nil {
		t.Fatalf("loadFromYAML: %v", err)
	}
	if cfg.MasterPassword != "from-expand" {
		t.Errorf("MasterPassword = %q, want from-expand", cfg.MasterPassword)
	}
}
