package httpapi

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/comnarin/foodstore/internal/domain"
	"github.com/comnarin/foodstore/internal/usecase"
)

type funcCalculator func(ctx context.Context, in usecase.CalculateInput) (domain.Quote, error)

func (f funcCalculator) Execute(ctx context.Context, in usecase.CalculateInput) (domain.Quote, error) {
	return f(ctx, in)
}

func TestMetrics_StatusOfSlotHeldUnexpectedErrorAndPanic(t *testing.T) {
	cases := []struct {
		name string
		fn   funcCalculator
		want string
	}{
		{"slot held", func(context.Context, usecase.CalculateInput) (domain.Quote, error) {
			return domain.Quote{}, &domain.SlotHeldError{ProductID: "red", ProductName: "Red set"}
		}, "409"},
		{"unexpected", func(context.Context, usecase.CalculateInput) (domain.Quote, error) {
			return domain.Quote{}, fmt.Errorf("boom")
		}, "500"},
		{"panic", func(context.Context, usecase.CalculateInput) (domain.Quote, error) { panic("kaboom") }, "500"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := prometheus.NewRegistry()
			m := NewHTTPMetrics(reg)
			app := NewApp(Deps{Calculator: tc.fn, HTTPMetrics: m})
			req := httptest.NewRequest("POST", "/api/calculate", strings.NewReader(`{"items":[{"productId":"red","quantity":1}]}`))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(resp.StatusCode) != tc.want {
				t.Fatalf("status = %d, want %s", resp.StatusCode, tc.want)
			}
			if got := testutil.ToFloat64(m.requests.WithLabelValues("POST", "/api/calculate", tc.want)); got != 1 {
				t.Fatalf("counter{status=%s} = %v, want 1", tc.want, got)
			}
			if got := testutil.ToFloat64(m.inFlight); got != 0 {
				t.Fatalf("in flight = %v after the request, want 0", got)
			}
		})
	}
}

func TestMetrics_UnknownPathsCollapseIntoOneUnmatchedSeries(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewHTTPMetrics(reg)
	app := NewApp(Deps{HTTPMetrics: m})
	for i := 0; i < 50; i++ {
		resp, err := app.Test(httptest.NewRequest("GET", fmt.Sprintf("/scan/%d/%d.php", i, i*7919), nil))
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 404 {
			t.Fatalf("status = %d", resp.StatusCode)
		}
	}
	if n := testutil.CollectAndCount(m.requests); n != 1 {
		t.Fatalf("request series = %d, want exactly 1", n)
	}
	if got := testutil.ToFloat64(m.requests.WithLabelValues("GET", "unmatched", "404")); got != 50 {
		t.Fatalf("unmatched 404 = %v, want 50", got)
	}
	if n := testutil.CollectAndCount(m.duration); n != 1 {
		t.Fatalf("duration series = %d, want 1", n)
	}
}

func TestMetrics_RouteLabelIsThePatternNotThePath(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewHTTPMetrics(reg)
	app := NewApp(Deps{HTTPMetrics: m, ProductImages: &fakeImages{result: usecase.ImageResult{Image: domain.Image{Data: []byte("x"), ContentType: "image/png"}}}})
	resp, err := app.Test(httptest.NewRequest("GET", "/api/products/red/image", nil))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("err=%v resp=%v", err, resp)
	}
	if got := testutil.ToFloat64(m.requests.WithLabelValues("GET", "/api/products/:id/image", "200")); got != 1 {
		t.Fatalf("pattern counter = %v, want 1", got)
	}
}

func TestMetrics_IsNotServedOnThePublicApp(t *testing.T) {
	app := NewApp(Deps{HTTPMetrics: NewHTTPMetrics(prometheus.NewRegistry())})
	resp, err := app.Test(httptest.NewRequest("GET", "/metrics", nil))
	if err != nil || resp.StatusCode != 404 {
		t.Fatalf("/metrics on the public app: err=%v status=%v, want 404", err, resp)
	}
}

func recorderTracer() (*tracetest.SpanRecorder, *sdktrace.TracerProvider) {
	rec := tracetest.NewSpanRecorder()
	return rec, sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
}

func TestTracing_ServerSpanWithChildAndIncomingTraceparent(t *testing.T) {
	otel.SetTextMapPropagator(propagation.TraceContext{})
	rec, tp := recorderTracer()
	tr := tp.Tracer("test")
	calc := funcCalculator(func(ctx context.Context, _ usecase.CalculateInput) (domain.Quote, error) {
		_, span := tr.Start(ctx, "CalculateOrder")
		span.End()
		return domain.Quote{}, nil
	})
	app := NewApp(Deps{Calculator: calc, Tracer: tr})

	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	req := httptest.NewRequest("POST", "/api/calculate", strings.NewReader(`{"items":[{"productId":"red","quantity":1}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("traceparent", "00-"+traceID+"-00f067aa0ba902b7-01")
	if resp, err := app.Test(req); err != nil || resp.StatusCode != 200 {
		t.Fatalf("err=%v resp=%v", err, resp)
	}

	spans := rec.Ended()
	if len(spans) != 2 {
		t.Fatalf("spans = %d, want 2", len(spans))
	}
	child, server := spans[0], spans[1]
	if server.Name() != "POST /api/calculate" || child.Name() != "CalculateOrder" {
		t.Fatalf("names = %q, %q", server.Name(), child.Name())
	}
	if child.Parent().SpanID() != server.SpanContext().SpanID() {
		t.Fatal("CalculateOrder must be a child of the server span")
	}
	if server.SpanContext().TraceID().String() != traceID {
		t.Fatalf("trace id = %s, want the incoming one", server.SpanContext().TraceID())
	}
	attrs := map[string]string{}
	for _, a := range server.Attributes() {
		attrs[string(a.Key)] = a.Value.Emit()
	}
	if attrs["http.route"] != "/api/calculate" || attrs["http.response.status_code"] != "200" || attrs["http.request.method"] != "POST" {
		t.Fatalf("attrs = %v", attrs)
	}
}

func TestTracing_OnlyServerErrorsMarkTheSpanAsError(t *testing.T) {
	otel.SetTextMapPropagator(propagation.TraceContext{})
	for _, tc := range []struct {
		err     error
		isError bool
	}{
		{&domain.SlotHeldError{ProductID: "red", ProductName: "Red set"}, false},
		{fmt.Errorf("boom"), true},
	} {
		rec, tp := recorderTracer()
		app := NewApp(Deps{Tracer: tp.Tracer("t"), Calculator: funcCalculator(func(context.Context, usecase.CalculateInput) (domain.Quote, error) {
			return domain.Quote{}, tc.err
		})})
		req := httptest.NewRequest("POST", "/api/calculate", strings.NewReader(`{"items":[{"productId":"red","quantity":1}]}`))
		req.Header.Set("Content-Type", "application/json")
		if _, err := app.Test(req); err != nil {
			t.Fatal(err)
		}
		got := rec.Ended()[0].Status().Code.String()
		if (got == "Error") != tc.isError {
			t.Fatalf("err %v: span status = %s", tc.err, got)
		}
	}
}
