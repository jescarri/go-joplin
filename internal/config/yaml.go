package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

// yamlConfig is the on-disk shape of the native YAML config file.
// Secrets are not stored in the file; use env vars (see README).
type yamlConfig struct {
	Sync          yamlSync          `yaml:"sync"`
	API           yamlAPI           `yaml:"api"`
	Server        yamlServer        `yaml:"server"`
	Observability yamlObservability `yaml:"observability"`
	MCP           yamlMCP           `yaml:"mcp"`
	RAG           yamlRAG           `yaml:"rag"`
}

type yamlSync struct {
	Target        int             `yaml:"target"`
	S3            yamlS3          `yaml:"s3"`
	JoplinServer  yamlJoplinServer `yaml:"joplin_server"`
}

type yamlS3 struct {
	Bucket         string `yaml:"bucket"`
	URL            string `yaml:"url"`
	Region         string `yaml:"region"`
	ForcePathStyle bool   `yaml:"force_path_style"`
}

type yamlJoplinServer struct {
	URL string `yaml:"url"`
}

type yamlAPI struct {
	Token          string `yaml:"token"`
	Key            string `yaml:"key"`
	MasterPassword string `yaml:"master_password"`
}

type yamlServer struct {
	DataDir    string `yaml:"data_dir"`
	Port       int    `yaml:"port"`
	ListenHost string `yaml:"listen_host"`
}

type yamlObservability struct {
	Tracing yamlTracing `yaml:"tracing"`
	Metrics yamlMetrics `yaml:"metrics"`
}

type yamlTracing struct {
	Enabled     bool    `yaml:"enabled"`
	Protocol    string  `yaml:"protocol"` // "http" or "grpc"
	Endpoint    string  `yaml:"endpoint"` // e.g. "${OTEL_EXPORTER_OTLP_ENDPOINT}" or URL
	ServiceName string  `yaml:"service_name"`
	SampleRate  float64 `yaml:"sample_rate"`
}

type yamlMetrics struct {
	Enabled        bool `yaml:"enabled"`
	PrometheusPort int  `yaml:"prometheus_port"`
}

type yamlMCP struct {
	AllowFolders      string `yaml:"allow_folders"`
	AllowTags         string `yaml:"allow_tags"`
	AllowCreateTag    bool   `yaml:"allow_create_tag"`
	AllowCreateFolder bool   `yaml:"allow_create_folder"`
	EnabledTools      string `yaml:"enabled_tools"`
}

type yamlRAG struct {
	Enabled      bool   `yaml:"enabled"`
	Endpoint     string `yaml:"endpoint"`
	APIKey       string `yaml:"api_key"`
	Model        string `yaml:"model"`
	Dimensions   int    `yaml:"dimensions"`
	ChunkSize    int    `yaml:"chunk_size"`
	ChunkOverlap int    `yaml:"chunk_overlap"`
	Workers      int    `yaml:"workers"`
	QueueSize    int    `yaml:"queue_size"`
}

// loadFromYAML parses data as the native YAML config and returns a Config.
// API token and key are expanded via ExpandEnv (e.g. ${GOJOPLIN_API_TOKEN}).
// S3 credentials are read from env (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY);
// Joplin Server credentials from GOJOPLIN_USERNAME, GOJOPLIN_PASSWORD.
func loadFromYAML(data []byte) (*Config, error) {
	var y yamlConfig
	if err := yaml.Unmarshal(data, &y); err != nil {
		return nil, fmt.Errorf("parse YAML config: %w", err)
	}

	cfg := &Config{}

	cfg.SyncTarget = y.Sync.Target
	if cfg.SyncTarget == 0 {
		cfg.SyncTarget = 8
	}

	// S3 (target 8)
	cfg.S3Bucket = y.Sync.S3.Bucket
	cfg.S3URL = y.Sync.S3.URL
	cfg.S3Region = y.Sync.S3.Region
	cfg.S3ForcePathStyle = y.Sync.S3.ForcePathStyle
	cfg.S3AccessKey = os.Getenv("AWS_ACCESS_KEY_ID")
	if cfg.S3AccessKey == "" {
		cfg.S3AccessKey = os.Getenv("ACCESS_KEY_ID")
	}
	cfg.S3SecretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	if cfg.S3SecretKey == "" {
		cfg.S3SecretKey = os.Getenv("SECRET_ACCESS_KEY")
	}

	// Joplin Server (target 9)
	cfg.ServerURL = y.Sync.JoplinServer.URL
	cfg.Username = os.Getenv("GOJOPLIN_USERNAME")
	cfg.Password = os.Getenv("GOJOPLIN_PASSWORD")

	// API: expand ${VAR} so secrets come from env
	cfg.APIToken = ExpandEnv(y.API.Token)
	cfg.APIKey = ExpandEnv(y.API.Key)
	// Do not trim master password so it matches desktop E2EE (see docs/e2ee-config-delta-from-main.md).
	cfg.MasterPassword = ExpandEnv(y.API.MasterPassword)

	// Server
	cfg.DataDir = y.Server.DataDir
	cfg.Port = y.Server.Port
	if cfg.Port == 0 {
		cfg.Port = 41184
	}
	cfg.ListenHost = y.Server.ListenHost

	// Observability
	tracingEndpoint := ExpandEnv(y.Observability.Tracing.Endpoint)
	if tracingEndpoint == "" {
		tracingEndpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	}
	tracingProtocol := y.Observability.Tracing.Protocol
	if tracingProtocol == "" {
		tracingProtocol = "http"
	}
	cfg.Observability = ObservabilityConfig{
		Tracing: TracingConfig{
			Enabled:     y.Observability.Tracing.Enabled,
			Exporter:    "otlp",
			Protocol:    tracingProtocol,
			Endpoint:    tracingEndpoint,
			ServiceName: y.Observability.Tracing.ServiceName,
			SampleRate:  y.Observability.Tracing.SampleRate,
		},
		Metrics: MetricsConfig{
			Enabled:        y.Observability.Metrics.Enabled,
			Exporter:       "prometheus",
			PrometheusPort: y.Observability.Metrics.PrometheusPort,
		},
	}
	if cfg.Observability.Tracing.ServiceName == "" {
		cfg.Observability.Tracing.ServiceName = "go-joplin"
	}
	if cfg.Observability.Tracing.SampleRate == 0 {
		cfg.Observability.Tracing.SampleRate = 1.0
	}
	if cfg.Observability.Metrics.PrometheusPort == 0 {
		cfg.Observability.Metrics.PrometheusPort = 9091
	}

	// MCP
	cfg.MCPAllowFolders = strings.TrimSpace(y.MCP.AllowFolders)
	cfg.MCPAllowTags = strings.TrimSpace(y.MCP.AllowTags)
	cfg.MCPAllowCreateTag = y.MCP.AllowCreateTag
	cfg.MCPAllowCreateFolder = y.MCP.AllowCreateFolder
	cfg.MCPEnabledTools = strings.TrimSpace(y.MCP.EnabledTools)

	// RAG
	cfg.RAG = RAGConfig{
		Enabled:      y.RAG.Enabled,
		Endpoint:     ExpandEnv(y.RAG.Endpoint),
		APIKey:       ExpandEnv(y.RAG.APIKey),
		Model:        y.RAG.Model,
		Dimensions:   y.RAG.Dimensions,
		ChunkSize:    y.RAG.ChunkSize,
		ChunkOverlap: y.RAG.ChunkOverlap,
		Workers:      y.RAG.Workers,
		QueueSize:    y.RAG.QueueSize,
	}

	return cfg, nil
}

// isYAMLConfig returns true if path has a .yaml or .yml extension.
func isYAMLConfig(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

// applyOverridesAndEnv applies CLI overrides and env vars on top of cfg (precedence: env > overrides > cfg).
// Sensitive values are trimmed (TrimSpace) to avoid newlines when set from files.
func applyOverridesAndEnv(cfg *Config, overrides Overrides) {
	if s := strings.TrimSpace(overrides.Username); s != "" {
		cfg.Username = s
	}
	if s := strings.TrimSpace(overrides.Password); s != "" {
		cfg.Password = s
	}
	if s := strings.TrimSpace(overrides.APIKey); s != "" {
		cfg.APIKey = s
	}
	// Master password: do not trim, so it matches main and desktop (E2EE key must be identical).
	if overrides.MasterPassword != "" {
		cfg.MasterPassword = overrides.MasterPassword
	}
	if v := strings.TrimSpace(os.Getenv("GOJOPLIN_USERNAME")); v != "" {
		cfg.Username = v
	}
	if v := strings.TrimSpace(os.Getenv("GOJOPLIN_PASSWORD")); v != "" {
		cfg.Password = v
	}
	if v := strings.TrimSpace(os.Getenv("GOJOPLIN_API_KEY")); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("GOJOPLIN_MASTER_PASSWORD"); v != "" {
		cfg.MasterPassword = v
	}
}

// validateConfig returns an error if cfg is invalid.
func validateConfig(cfg *Config) error {
	if cfg.SyncTarget != 8 && cfg.SyncTarget != 9 {
		return fmt.Errorf("unsupported sync target %d (only S3 target 8 and Joplin Server target 9 are supported)", cfg.SyncTarget)
	}
	if cfg.SyncTarget == 9 {
		if cfg.ServerURL == "" {
			return fmt.Errorf("sync server URL is required for Joplin Server (sync.target 9)")
		}
	}
	if cfg.SyncTarget == 8 {
		if cfg.S3Bucket == "" {
			return fmt.Errorf("sync.8.path (S3 bucket name) is required for S3 sync")
		}
		hasEnvKey := os.Getenv("AWS_ACCESS_KEY_ID") != "" || os.Getenv("ACCESS_KEY_ID") != ""
		hasEnvSecret := os.Getenv("AWS_SECRET_ACCESS_KEY") != "" || os.Getenv("SECRET_ACCESS_KEY") != ""
		hasConfigKey := cfg.S3AccessKey != ""
		hasConfigSecret := cfg.S3SecretKey != ""
		if !(hasEnvKey || hasConfigKey) || !(hasEnvSecret || hasConfigSecret) {
			return fmt.Errorf("S3 credentials required: set env vars (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY) and/or sync.8.username and sync.8.password in config")
		}
	}
	return nil
}

// applyRuntimeDefaults sets data dir, port, listen host, observability, and MCP from env when not set.
func applyRuntimeDefaults(cfg *Config) {
	if cfg.DataDir == "" {
		cfg.DataDir = os.Getenv("GOJOPLIN_DATA_DIR")
		if cfg.DataDir == "" {
			home, _ := os.UserHomeDir()
			cfg.DataDir = filepath.Join(home, ".local", "share", "gojoplin")
		}
	}
	if portStr := os.Getenv("GOJOPLIN_PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			cfg.Port = p
		}
	}
	if v := os.Getenv("GOJOPLIN_LISTEN_HOST"); v != "" {
		cfg.ListenHost = v
	}

	// Observability env overrides
	if v := os.Getenv("GOJOPLIN_TRACING_ENABLED"); v != "" {
		cfg.Observability.Tracing.Enabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("GOJOPLIN_TRACING_PROTOCOL"); v != "" {
		cfg.Observability.Tracing.Protocol = v
	}
	if v := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); v != "" {
		cfg.Observability.Tracing.Endpoint = ExpandEnv(v)
	}
	if v := os.Getenv("GOJOPLIN_TRACING_SERVICE_NAME"); v != "" {
		cfg.Observability.Tracing.ServiceName = v
	}
	if v := os.Getenv("GOJOPLIN_TRACING_SAMPLE_RATE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.Observability.Tracing.SampleRate = f
		}
	}
	if v := os.Getenv("GOJOPLIN_METRICS_ENABLED"); v != "" {
		cfg.Observability.Metrics.Enabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("GOJOPLIN_METRICS_PROMETHEUS_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Observability.Metrics.PrometheusPort = p
		}
	}

	if v := os.Getenv("GOJOPLIN_MCP_ALLOW_FOLDERS"); v != "" {
		cfg.MCPAllowFolders = strings.TrimSpace(v)
	}
	if v := os.Getenv("GOJOPLIN_MCP_ALLOW_TAGS"); v != "" {
		cfg.MCPAllowTags = strings.TrimSpace(v)
	}
	if v := os.Getenv("GOJOPLIN_MCP_ALLOW_CREATE_TAG"); v != "" {
		cfg.MCPAllowCreateTag = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("GOJOPLIN_MCP_ALLOW_CREATE_FOLDER"); v != "" {
		cfg.MCPAllowCreateFolder = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("GOJOPLIN_MCP_ENABLED_TOOLS"); v != "" {
		cfg.MCPEnabledTools = strings.TrimSpace(v)
	}

	// RAG env overrides
	if v := os.Getenv("GOJOPLIN_RAG_ENABLED"); v != "" {
		cfg.RAG.Enabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("GOJOPLIN_RAG_ENDPOINT"); v != "" {
		cfg.RAG.Endpoint = strings.TrimSpace(v)
	}
	if v := os.Getenv("GOJOPLIN_RAG_API_KEY"); v != "" {
		cfg.RAG.APIKey = v
	}
	if v := os.Getenv("GOJOPLIN_RAG_MODEL"); v != "" {
		cfg.RAG.Model = strings.TrimSpace(v)
	}
	if v := os.Getenv("GOJOPLIN_RAG_DIMENSIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.RAG.Dimensions = n
		}
	}
	if v := os.Getenv("GOJOPLIN_RAG_CHUNK_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.RAG.ChunkSize = n
		}
	}
	if v := os.Getenv("GOJOPLIN_RAG_CHUNK_OVERLAP"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.RAG.ChunkOverlap = n
		}
	}
	if v := os.Getenv("GOJOPLIN_RAG_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.RAG.Workers = n
		}
	}
	if v := os.Getenv("GOJOPLIN_RAG_QUEUE_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.RAG.QueueSize = n
		}
	}

	// RAG defaults
	if cfg.RAG.Dimensions == 0 {
		cfg.RAG.Dimensions = 1536
	}
	if cfg.RAG.ChunkSize == 0 {
		cfg.RAG.ChunkSize = 512
	}
	if cfg.RAG.ChunkOverlap == 0 {
		cfg.RAG.ChunkOverlap = 50
	}
	if cfg.RAG.Workers == 0 {
		cfg.RAG.Workers = 2
	}
	if cfg.RAG.QueueSize == 0 {
		cfg.RAG.QueueSize = 1000
	}
}
