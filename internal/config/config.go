package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

	// Observability (tracing + metrics). Filled from env in Load().
	Observability ObservabilityConfig `json:"-"`
}

// ListenAddr returns the address the clipper server should listen on.
func (c *Config) ListenAddr() string {
	port := c.Port
	if port == 0 {
		port = 41184
	}
	return fmt.Sprintf("localhost:%d", port)
}

// Overrides holds values from CLI flags that can override config file settings.
type Overrides struct {
	Username       string
	Password       string
	APIKey         string
	MasterPassword string
}

// Load reads the Joplin settings file and returns a Config.
// If cfgPath is empty, it defaults to ~/.config/joplin-desktop/settings.json.
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
			cfgPath = filepath.Join(home, ".config", "joplin-desktop", "settings.json")
		}
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read config file %s: %w", cfgPath, err)
	}

	// Joplin settings.json uses flat dotted keys
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("cannot parse config file: %w", err)
	}

	cfg := &Config{}

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

	// Apply CLI flag overrides
	if len(overrides) > 0 {
		o := overrides[0]
		if o.Username != "" {
			cfg.Username = o.Username
		}
		if o.Password != "" {
			cfg.Password = o.Password
		}
		if o.APIKey != "" {
			cfg.APIKey = o.APIKey
		}
		if o.MasterPassword != "" {
			cfg.MasterPassword = o.MasterPassword
		}
	}

	// Env vars have highest precedence
	if v := os.Getenv("GOJOPLIN_USERNAME"); v != "" {
		cfg.Username = v
	}
	if v := os.Getenv("GOJOPLIN_PASSWORD"); v != "" {
		cfg.Password = v
	}
	if v := os.Getenv("GOJOPLIN_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("GOJOPLIN_MASTER_PASSWORD"); v != "" {
		cfg.MasterPassword = v
	}

	if cfg.SyncTarget != 8 && cfg.SyncTarget != 9 {
		return nil, fmt.Errorf("unsupported sync target %d (only S3 target 8 and Joplin Server target 9 are supported)", cfg.SyncTarget)
	}

	if cfg.SyncTarget == 9 {
		if cfg.ServerURL == "" {
			return nil, fmt.Errorf("sync.9.path (server URL) is required for Joplin Server")
		}
	}
	if cfg.SyncTarget == 8 {
		if cfg.S3Bucket == "" {
			return nil, fmt.Errorf("sync.8.path (S3 bucket name) is required for S3 sync")
		}
		// S3 credentials: env overrides; else sync.8.username / sync.8.password from config (Joplin-style)
		hasEnvKey := os.Getenv("AWS_ACCESS_KEY_ID") != "" || os.Getenv("ACCESS_KEY_ID") != ""
		hasEnvSecret := os.Getenv("AWS_SECRET_ACCESS_KEY") != "" || os.Getenv("SECRET_ACCESS_KEY") != ""
		hasConfigKey := cfg.S3AccessKey != ""
		hasConfigSecret := cfg.S3SecretKey != ""
		if !(hasEnvKey || hasConfigKey) || !(hasEnvSecret || hasConfigSecret) {
			return nil, fmt.Errorf("S3 credentials required: set env vars (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY) and/or sync.8.username and sync.8.password in config")
		}
	}

	// Data directory
	cfg.DataDir = os.Getenv("GOJOPLIN_DATA_DIR")
	if cfg.DataDir == "" {
		home, _ := os.UserHomeDir()
		cfg.DataDir = filepath.Join(home, ".local", "share", "joplingo")
	}

	// Port override
	if portStr := os.Getenv("GOJOPLIN_PORT"); portStr != "" {
		fmt.Sscanf(portStr, "%d", &cfg.Port)
	}

	// Observability: defaults then env overrides
	cfg.Observability = DefaultObservability()
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

	return cfg, nil
}
