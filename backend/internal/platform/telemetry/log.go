package telemetry

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// logHandler adds trace_id and span_id to every record logged with a context
// that carries an active span, so logs can be matched to traces.
type logHandler struct{ inner slog.Handler }

// NewLogHandler wraps inner with trace correlation.
func NewLogHandler(inner slog.Handler) slog.Handler { return logHandler{inner: inner} }

func (h logHandler) Enabled(ctx context.Context, l slog.Level) bool { return h.inner.Enabled(ctx, l) }

func (h logHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	return h.inner.Handle(ctx, r)
}

func (h logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return logHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h logHandler) WithGroup(name string) slog.Handler {
	return logHandler{inner: h.inner.WithGroup(name)}
}
