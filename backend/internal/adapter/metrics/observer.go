// Package metrics exposes business and dependency metrics for Prometheus.
//
// Everything registers on a registry the caller passes in (never the default
// one), so tests can build as many as they like. Label values are bounded:
// products, promotion codes and Redis command names come from fixed sets.
package metrics

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/comnarin/foodstore/internal/domain"
	"github.com/comnarin/foodstore/internal/usecase"
)

// Observer implements usecase.Observer on Prometheus counters.
type Observer struct {
	orders    *prometheus.CounterVec
	promos    *prometheus.CounterVec
	claims    *prometheus.CounterVec
	fallbacks *prometheus.CounterVec
	images    *prometheus.CounterVec
}

var _ usecase.Observer = (*Observer)(nil)

func NewObserver(reg prometheus.Registerer) *Observer {
	o := &Observer{
		orders: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "foodstore_orders_calculated_total", Help: "Orders priced and saved, by whether a member card was used.",
		}, []string{"member"}),
		promos: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "foodstore_promotion_applied_total", Help: "Discounts applied to saved orders, by rule code.",
		}, []string{"rule"}),
		claims: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "foodstore_limit_claims_total", Help: "Limited-product claims, by product, deciding system (redis|db) and outcome.",
		}, []string{"product", "source", "outcome"}),
		fallbacks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "foodstore_limiter_fallback_total", Help: "Times Redis could not be used and the Postgres guard decided.",
		}, []string{"product"}),
		images: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "foodstore_product_images_total", Help: "Product pictures served, by source (store|fallback).",
		}, []string{"source"}),
	}
	reg.MustRegister(o.orders, o.promos, o.claims, o.fallbacks, o.images)
	return o
}

func (o *Observer) OrderCalculated(q domain.Quote, member bool) {
	o.orders.WithLabelValues(strconv.FormatBool(member)).Inc()
	for _, d := range q.Discounts {
		o.promos.WithLabelValues(d.RuleCode).Inc()
	}
}

func (o *Observer) LimitClaim(p domain.ProductID, src usecase.ClaimSource, out usecase.ClaimOutcome) {
	o.claims.WithLabelValues(string(p), string(src), string(out)).Inc()
}

func (o *Observer) LimiterFallback(p domain.ProductID) {
	o.fallbacks.WithLabelValues(string(p)).Inc()
}

func (o *Observer) ImageServed(fromStore bool) {
	source := "fallback"
	if fromStore {
		source = "store"
	}
	o.images.WithLabelValues(source).Inc()
}
