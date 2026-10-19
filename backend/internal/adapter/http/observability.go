package httpapi

import (
	"strconv"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// HTTPMetrics are the RED metrics of the public API. Labels are bounded on
// purpose: the route is always a registered route pattern (or "unmatched"),
// never the raw path, and unusual methods collapse into "OTHER".
type HTTPMetrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inFlight prometheus.Gauge
}

// NewHTTPMetrics registers the HTTP metrics on reg. It never touches the
// default registry, so it can be called once per test app.
func NewHTTPMetrics(reg prometheus.Registerer) *HTTPMetrics {
	m := &HTTPMetrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_server_requests_total",
			Help: "HTTP requests served, by method, route pattern and status code.",
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_server_request_duration_seconds",
			Help:    "HTTP request latency, by method and route pattern.",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		}, []string{"method", "route"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "http_server_requests_in_flight",
			Help: "HTTP requests currently being served.",
		}),
	}
	reg.MustRegister(m.requests, m.duration, m.inFlight)
	return m
}

// finish runs the rest of the chain and turns any returned error into the
// response now, so middleware that sits outside the error handler can read the
// final status code. It always returns nil.
func finish(c fiber.Ctx, err error) int {
	if err != nil {
		if herr := c.App().ErrorHandler(c, err); herr != nil {
			_ = c.SendStatus(fiber.StatusInternalServerError)
		}
	}
	return c.Response().StatusCode()
}

// routeLabels maps a matched Fiber route to its registered pattern. Anything
// that is not a registered route (404s for scanners, random paths) becomes
// "unmatched", which keeps the label set bounded.
type routeLabels struct {
	once sync.Once
	set  map[string]struct{}
}

func (r *routeLabels) label(c fiber.Ctx) string {
	r.once.Do(func() {
		r.set = map[string]struct{}{}
		for _, rt := range c.App().GetRoutes(true) {
			r.set[rt.Method+" "+rt.Path] = struct{}{}
		}
	})
	rt := c.Route()
	if rt == nil {
		return "unmatched"
	}
	if _, ok := r.set[rt.Method+" "+rt.Path]; !ok {
		return "unmatched"
	}
	return rt.Path
}

func methodLabel(m string) string {
	switch m {
	case fiber.MethodGet, fiber.MethodHead, fiber.MethodPost, fiber.MethodPut,
		fiber.MethodPatch, fiber.MethodDelete, fiber.MethodOptions:
		return m
	}
	return "OTHER"
}

func (m *HTTPMetrics) middleware(routes *routeLabels) fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()
		m.inFlight.Inc()
		defer m.inFlight.Dec()

		status := finish(c, c.Next())

		method, route := methodLabel(c.Method()), routes.label(c)
		m.requests.WithLabelValues(method, route, strconv.Itoa(status)).Inc()
		m.duration.WithLabelValues(method, route).Observe(time.Since(start).Seconds())
		return nil
	}
}

// headerCarrier lets the OpenTelemetry propagator read request headers.
type headerCarrier struct{ h *fasthttp.RequestHeader }

func (c headerCarrier) Get(key string) string { return string(c.h.Peek(key)) }
func (c headerCarrier) Set(key, val string)   { c.h.Set(key, val) }
func (c headerCarrier) Keys() []string {
	var keys []string
	for k := range c.h.All() {
		keys = append(keys, string(k))
	}
	return keys
}

var _ propagation.TextMapCarrier = headerCarrier{}

// tracing starts a server span per request, continues a trace sent by the
// caller (W3C traceparent) and stores the span context in the request context
// so use cases and database/Redis calls become child spans.
func tracing(tr trace.Tracer, routes *routeLabels) fiber.Handler {
	return func(c fiber.Ctx) error {
		ctx := otel.GetTextMapPropagator().Extract(c.Context(), headerCarrier{&c.Request().Header})
		ctx, span := tr.Start(ctx, c.Method(), trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		c.SetContext(ctx)

		status := finish(c, c.Next())

		route := routes.label(c)
		span.SetName(c.Method() + " " + route)
		span.SetAttributes(
			attribute.String("http.request.method", methodLabel(c.Method())),
			attribute.String("http.route", route),
			attribute.Int("http.response.status_code", status),
		)
		if status >= 500 {
			span.SetStatus(codes.Error, "")
		}
		return nil
	}
}
