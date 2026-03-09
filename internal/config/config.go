package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// TracingConfig holds OpenTelemetry tracing configuration.
type TracingConfig struct {
	Enabled     bool    `json:"enabled"`
	Exporter    string  `json:"exporter"` // "otlp"
	Protocol    string  `json:"protocol"` // "http" or "grpc"
	Endpoint    string  `json:"endpoint"` // e.g. from OTEL_EXPORTER_OTLP_ENDPOINT
	ServiceName string  `json:"service_name"`
	SampleRate  float64 `json:"sample_rate"`
}

// MetricsConfig holds metrics configuration (Prometheus).
type MetricsConfig struct {
	Enabled        bool   `json:"enabled"`
	Exporter       string `json:"exporter"`        // "prometheus"
	PrometheusPort int    `json:"prometheus_port"` // port for /metrics
}

// ObservabilityConfig holds tracing and metrics configuration.
// Loaded from environment variables; endpoint supports ${VAR} expansion.
func DefaultObservability() ObservabilityConfig {
	return ObservabilityConfig{
		Tracing: TracingConfig{
			Enabled:     true,
			Exporter:    "otlp",
			Protocol:    "http",
			Endpoint:    os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
			ServiceName: "go-joplin",
			SampleRate:  1.0,
		},
		Metrics: MetricsConfig{
			Enabled:        true,
			Exporter:       "prometheus",
			PrometheusPort: 9091,
		},
	}
}

// ObservabilityConfig holds tracing and metrics config.
type ObservabilityConfig struct {
	Tracing TracingConfig `json:"tracing"`
	Metrics MetricsConfig `json:"metrics"`
}

// ExpandEnv replaces ${VAR} in s with os.Getenv("VAR").
func ExpandEnv(s string) string {
	return os.Expand(s, func(key string) string { return os.Getenv(key) })
}

// Config holds the resolved configuration for go-joplin.
type Config struct {
	SyncTarget int    `json:"sync.target"`
	ServerURL  string `json:"sync.9.path"`
	Username   string `json:"sync.9.username"`
	Password   string `json:"sync.9.password"`
	// S3 (sync target 8)
	S3Bucket         string `json:"sync.8.path"`
	S3URL            string `json:"sync.8.url"`
	S3Region         string `json:"sync.8.region"`
	S3AccessKey      string `json:"sync.8.username"`
	S3SecretKey      string `json:"sync.8.password"`
	S3ForcePathStyle bool   `json:"sync.8.forcePathStyle"`
	//
	APIToken       string `json:"api.token"`
	APIKey         string `json:"-"`
	MasterPassword string `json:"-"`
	DataDir        string `json:"-"`
	Port           int    `json:"-"`
	ListenHost     string `json:"-"` // bind address host (default: localhost)

	// Observability (tracing + metrics). Filled from env in Load().
	Observability ObservabilityConfig `json:"-"`

	// MCP / Clipper mutation allow-list. By default all mutations are read-only.
	// Use "*" to allow all mutations for that category.
	MCPAllowFolders      string `json:"-"` // comma-separated folder IDs or titles; "*" = all
	MCPAllowTags         string `json:"-"` // comma-separated tag IDs or titles; "*" = all
	MCPAllowCreateTag    bool   `json:"-"` // allow creating new tags
	MCPAllowCreateFolder bool   `json:"-"` // allow creating new folders
}

// ListenAddr returns the address the clipper server should listen on.
func (c *Config) ListenAddr() string {
	host := c.ListenHost
	if host == "" {
		host = "localhost"
	}
	port := c.Port
	if port == 0 {
		port = 41184
	}
	return fmt.Sprintf("%s:%d", host, port)
}

// Overrides holds values from CLI flags that can override config file settings.
type Overrides struct {
	Username       string
	Password       string
	APIKey         string
	MasterPassword string
}

// Load reads the config file and returns a Config.
// If cfgPath is empty, it defaults to ~/.config/joplin-desktop/settings.json.
// If the path has a .yaml or .yml extension, the native YAML config format is used; otherwise Joplin settings.json (JSON) is expected.
// Precedence: env vars > CLI flags (via overrides) > config file.
func Load(cfgPath string, overrides ...Overrides) (*Config, error) {
	if cfgPath == "" {
		if v := os.Getenv("GOJOPLIN_CONFIG_PATH"); v != "" {
			cfgPath = v
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("cannot determine home directory: %w", err)
			}
			// Prefer go-joplin YAML if present, else Joplin desktop settings
			yamlPath := filepath.Join(home, ".config", "go-joplin", "config.yaml")
			if _, err := os.Stat(yamlPath); err == nil {
				cfgPath = yamlPath
			} else {
				cfgPath = filepath.Join(home, ".config", "joplin-desktop", "settings.json")
			}
		}
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read config file %s: %w", cfgPath, err)
	}

	var cfg *Config
	if isYAMLConfig(cfgPath) {
		cfg, err = loadFromYAML(data)
		if err != nil {
			return nil, err
		}
	} else {
		cfg, err = loadFromJoplinJSON(data)
		if err != nil {
			return nil, err
		}
		cfg.Observability = DefaultObservability()
	}

	o := Overrides{}
	if len(overrides) > 0 {
		o = overrides[0]
	}
	applyOverridesAndEnv(cfg, o)
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	applyRuntimeDefaults(cfg)
	return cfg, nil
}

// loadFromJoplinJSON parses data as Joplin settings.json (flat dotted keys) and returns a Config.
func loadFromJoplinJSON(data []byte) (*Config, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("cannot parse config file: %w", err)
	}

	getString := func(key string) string {
		v, ok := raw[key]
		if !ok {
			return ""
		}
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return ""
		}
		return s
	}
	getInt := func(key string) int {
		v, ok := raw[key]
		if !ok {
			return 0
		}
		var n int
		if err := json.Unmarshal(v, &n); err != nil {
			return 0
		}
		return n
	}
	getBool := func(key string) bool {
		v, ok := raw[key]
		if !ok {
			return false
		}
		var b bool
		if err := json.Unmarshal(v, &b); err != nil {
			return false
		}
		return b
	}

	cfg := &Config{}
	cfg.SyncTarget = getInt("sync.target")
	cfg.ServerURL = getString("sync.9.path")
	cfg.Username = getString("sync.9.username")
	cfg.Password = getString("sync.9.password")
	cfg.S3Bucket = getString("sync.8.path")
	cfg.S3URL = getString("sync.8.url")
	cfg.S3Region = getString("sync.8.region")
	cfg.S3AccessKey = getString("sync.8.username")
	cfg.S3SecretKey = getString("sync.8.password")
	cfg.S3ForcePathStyle = getBool("sync.8.forcePathStyle")
	cfg.APIToken = getString("api.token")
	return cfg, nil
}
