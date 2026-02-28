package sync

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/jescarri/go-joplin/internal/sync"

// tracedBackend wraps a SyncBackend and adds tracing for Get, Put, Delete.
type tracedBackend struct {
	inner  SyncBackend
	name   string
	tracer trace.Tracer
}

// NewTracedBackend returns a SyncBackend that traces all storage operations.
func NewTracedBackend(inner SyncBackend, backendName string) SyncBackend {
	return &tracedBackend{
		inner:  inner,
		name:   backendName,
		tracer: otel.Tracer(tracerName),
	}
}

func (t *tracedBackend) Authenticate() error {
	return t.inner.Authenticate()
}

func (t *tracedBackend) IsAuthenticated() bool {
	return t.inner.IsAuthenticated()
}

func (t *tracedBackend) AcquireLock() (interface{}, error) {
	return t.inner.AcquireLock()
}

func (t *tracedBackend) ReleaseLock(lock interface{}) error {
	return t.inner.ReleaseLock(lock)
}

func (t *tracedBackend) SyncTarget() int {
	return t.inner.SyncTarget()
}

func (t *tracedBackend) Get(path string) ([]byte, error) {
	ctx := context.Background()
	ctx, span := t.tracer.Start(ctx, "storage.get",
		trace.WithAttributes(
			attribute.String("backend", t.name),
			attribute.String("path", path),
		),
	)
	defer span.End()
	data, err := t.inner.Get(path)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return data, err
}

func (t *tracedBackend) Put(path string, content []byte) error {
	ctx := context.Background()
	ctx, span := t.tracer.Start(ctx, "storage.put",
		trace.WithAttributes(
			attribute.String("backend", t.name),
			attribute.String("path", path),
			attribute.Int("content_len", len(content)),
		),
	)
	defer span.End()
	err := t.inner.Put(path, content)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func (t *tracedBackend) Delete(path string) error {
	ctx := context.Background()
	ctx, span := t.tracer.Start(ctx, "storage.delete",
		trace.WithAttributes(
			attribute.String("backend", t.name),
			attribute.String("path", path),
		),
	)
	defer span.End()
	err := t.inner.Delete(path)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}
