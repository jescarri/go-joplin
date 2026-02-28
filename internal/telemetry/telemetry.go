package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/jescarri/go-joplin/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

const (
	serviceNameKey = "service.name"
)

// HTTPDurationHistogram is the standard histogram for API request duration.
// Use with route label for p99 via Prometheus recording rules.
var HTTPDurationHistogram = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "Duration of HTTP requests in seconds",
		Buckets: prometheus.DefBuckets, // 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10
	},
	[]string{"method", "route"},
)

// Reg is the default Prometheus registry. Used by the metrics server.
var Reg = prometheus.NewRegistry()

func init() {
	Reg.MustRegister(HTTPDurationHistogram)
}

// InitTracing initializes the global tracer provider with OTLP HTTP exporter.
// Caller must call the returned shutdown when done.
// Uses OTEL_EXPORTER_OTLP_ENDPOINT when cfg.Endpoint is set (after ExpandEnv).
func InitTracing(ctx context.Context, cfg config.TracingConfig) (func(context.Context) error, error) {
	if !cfg.Enabled {
		return func(context.Context) error { return nil }, nil
	}
	endpoint := config.ExpandEnv(cfg.Endpoint)
	if endpoint == "" {
		endpoint = "http://localhost:4318"
	}
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(endpoint),
		otlptracehttp.WithInsecure(), // use WithTLSCredentials in production
	)
	if err != nil {
		return nil, fmt.Errorf("otlp trace exporter: %w", err)
	}

	// Use a single resource to avoid Schema URL conflict with resource.Default() (1.39 vs 1.24).
	res, err := resource.New(context.Background(),
		resource.WithAttributes(semconv.ServiceName(cfg.ServiceName)),
		resource.WithSchemaURL(semconv.SchemaURL),
	)
	if err != nil {
		return nil, fmt.Errorf("resource: %w", err)
	}

	sampler := sdktrace.AlwaysSample()
	if cfg.SampleRate < 1.0 && cfg.SampleRate > 0 {
		sampler = sdktrace.TraceIDRatioBased(cfg.SampleRate)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return tp.Shutdown, nil
}

// MetricsHandler returns the Prometheus /metrics handler using Reg.
func MetricsHandler() http.Handler {
	return promhttp.HandlerFor(Reg, promhttp.HandlerOpts{})
}

// StartMetricsServer starts an HTTP server on the given address serving /metrics.
// Serves only /metrics; no tracing or request logging. Runs until context is cancelled.
// If onListening is non-nil, it is called once the port is bound (so the caller can log only on success).
func StartMetricsServer(ctx context.Context, addr string, onListening func()) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	if onListening != nil {
		onListening()
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", MetricsHandler())
	srv := &http.Server{Handler: mux}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// ObserveHTTPRequest records the duration of an HTTP request in the histogram.
// route should be the route pattern (e.g. "GET /notes", "POST /mcp").
func ObserveHTTPRequest(method, route string, duration time.Duration) {
	HTTPDurationHistogram.WithLabelValues(method, route).Observe(duration.Seconds())
}

// SkipPath returns true if the path should be excluded from tracing and request logging (e.g. /health).
func SkipPath(path string) bool {
	return path == "/health"
}

// Middleware returns an HTTP middleware that records request duration and skips /health.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if SkipPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		next.ServeHTTP(w, r)
		route := r.Method + " " + r.URL.Path
		ObserveHTTPRequest(r.Method, route, time.Since(start))
	})
}

// LoggingSkipHealth wraps next to log requests only when path is not /health.
func LoggingSkipHealth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if SkipPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		slog.Debug("request", "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
