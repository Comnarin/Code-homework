package telemetry

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/comnarin/foodstore/internal/platform/db/dbtest"
)

func TestGormTracing_RecordsStatementsWithoutArguments(t *testing.T) {
	gdb := dbtest.Fresh(t, "_telemetry")
	rec := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))
	if err := gdb.Use(NewGormTracing()); err != nil {
		t.Fatal(err)
	}

	var n int
	if err := gdb.WithContext(context.Background()).Raw("SELECT count(*) FROM members WHERE card_number = ?", "SECRET-CARD").Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	spans := rec.Ended()
	if len(spans) == 0 {
		t.Fatal("no span recorded")
	}
	for _, sp := range spans {
		for _, a := range sp.Attributes() {
			if strings.Contains(a.Value.Emit(), "SECRET-CARD") {
				t.Fatalf("argument value leaked into span attribute %s", a.Key)
			}
		}
	}
}
