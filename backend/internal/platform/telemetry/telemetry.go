// Package telemetry sets up OpenTelemetry tracing and trace-aware logging.
//
// Tracing is off unless OTEL_EXPORTER_OTLP_ENDPOINT is set. Only traces are
// exported; metrics go through Prometheus. Other settings (headers, sampler,
// timeouts) use the standard OTEL_* environment variables.
package telemetry

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const defaultServiceName = "foodstore-api"

// Enabled reports whether tracing is configured.
func Enabled() bool { return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" }

// Setup installs a global tracer provider that exports spans over OTLP/HTTP.
// With no endpoint configured it does nothing and returns a no-op shutdown.
// Call the returned function at shutdown to flush pending spans.
func Setup(ctx context.Context, version string) (shutdown func(context.Context) error, err error) {
	noop := func(context.Context) error { return nil }
	if !Enabled() {
		return noop, nil
	}
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return noop, fmt.Errorf("create trace exporter: %w", err)
	}
	name := os.Getenv("OTEL_SERVICE_NAME")
	if name == "" {
		name = defaultServiceName
	}
	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(
		attribute.String("service.name", name),
		attribute.String("service.version", version),
	))
	if err != nil {
		return noop, fmt.Errorf("build resource: %w", err)
	}
	// No WithSampler: the SDK reads OTEL_TRACES_SAMPLER{,_ARG} itself.
	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter), sdktrace.WithResource(res))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return tp.Shutdown, nil
}
