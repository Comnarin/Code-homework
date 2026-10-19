package telemetry

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

// GormTracing is a tiny GORM plugin that records one client span per
// statement. It stores the SQL with its $1 placeholders and never the
// argument values, so member cards and other data stay out of traces. (The
// off-the-shelf plugin drags the ClickHouse and MySQL drivers into the binary.)
type GormTracing struct{ tracer trace.Tracer }

func NewGormTracing() *GormTracing {
	return &GormTracing{tracer: otel.Tracer("github.com/comnarin/foodstore/gorm")}
}

func (*GormTracing) Name() string { return "foodstore:tracing" }

func (p *GormTracing) Initialize(db *gorm.DB) error {
	for _, step := range []struct {
		op     string
		before func(string, func(*gorm.DB)) error
		after  func(string, func(*gorm.DB)) error
	}{
		{"create", db.Callback().Create().Before("gorm:create").Register, db.Callback().Create().After("gorm:create").Register},
		{"query", db.Callback().Query().Before("gorm:query").Register, db.Callback().Query().After("gorm:query").Register},
		{"update", db.Callback().Update().Before("gorm:update").Register, db.Callback().Update().After("gorm:update").Register},
		{"delete", db.Callback().Delete().Before("gorm:delete").Register, db.Callback().Delete().After("gorm:delete").Register},
		{"row", db.Callback().Row().Before("gorm:row").Register, db.Callback().Row().After("gorm:row").Register},
		{"raw", db.Callback().Raw().Before("gorm:raw").Register, db.Callback().Raw().After("gorm:raw").Register},
	} {
		if err := step.before("foodstore:before_"+step.op, p.start(step.op)); err != nil {
			return err
		}
		if err := step.after("foodstore:after_"+step.op, p.finish); err != nil {
			return err
		}
	}
	return nil
}

func (p *GormTracing) start(op string) func(*gorm.DB) {
	return func(db *gorm.DB) {
		ctx, span := p.tracer.Start(db.Statement.Context, "db."+op, trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attribute.String("db.system", "postgresql")))
		db.Statement.Context = ctx
		db.InstanceSet("foodstore:span", span)
	}
}

func (p *GormTracing) finish(db *gorm.DB) {
	v, ok := db.InstanceGet("foodstore:span")
	if !ok {
		return
	}
	span := v.(trace.Span)
	defer span.End()
	if db.Statement.Table != "" {
		span.SetAttributes(attribute.String("db.sql.table", db.Statement.Table))
	}
	span.SetAttributes(attribute.String("db.statement", db.Statement.SQL.String()))
	if err := db.Error; err != nil && err != gorm.ErrRecordNotFound {
		span.RecordError(err)
		span.SetStatus(codes.Error, "")
	}
}
