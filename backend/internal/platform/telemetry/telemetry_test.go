package telemetry

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestSetup_IsANoOpWithoutAnEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	shutdown, err := Setup(context.Background(), "test")
	if err != nil || shutdown == nil {
		t.Fatalf("err=%v shutdown nil=%v", err, shutdown == nil)
	}
	if Enabled() {
		t.Fatal("tracing must be off without an endpoint")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLogHandler_AddsTraceIDsOnlyInsideASpan(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(NewLogHandler(slog.NewJSONHandler(&buf, nil)))
	tp := sdktrace.NewTracerProvider()

	log.InfoContext(context.Background(), "outside")
	ctx, span := tp.Tracer("t").Start(context.Background(), "op")
	log.InfoContext(ctx, "inside")
	span.End()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %v", lines)
	}
	if strings.Contains(lines[0], "trace_id") {
		t.Fatalf("no trace id outside a span: %s", lines[0])
	}
	if !strings.Contains(lines[1], span.SpanContext().TraceID().String()) || !strings.Contains(lines[1], "span_id") {
		t.Fatalf("inside a span the log needs trace_id and span_id: %s", lines[1])
	}
}
