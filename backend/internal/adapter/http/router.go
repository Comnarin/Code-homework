// Package httpapi is the Fiber delivery adapter: it parses requests, calls
// use cases and presents results. It holds no business rules.
package httpapi

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
)

type Deps struct {
	Products   ProductUsecase
	Promotions PromotionUsecase
	Calculator OrderCalculator
	// ProductImages is optional. When nil, the image route is not registered.
	ProductImages ImageGetter
	// ResetLimits is optional. When nil, the test-only reset route is not registered.
	ResetLimits LimitResetter
	// Ready backs GET /readyz. When nil, the route is not registered.
	Ready ReadinessChecker
}

func NewApp(d Deps) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: errorHandler,
		// Bound slow or stalled clients so they cannot hold connections open.
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	})
	app.Use(recover.New())

	h := handlers{images: d.ProductImages, products: d.Products, promotions: d.Promotions, calculator: d.Calculator, reset: d.ResetLimits, ready: d.Ready}
	// healthz is liveness only: it never touches dependencies, so a database
	// outage does not get the process restarted. readyz checks them.
	app.Get("/healthz", h.health)
	if d.Ready != nil {
		app.Get("/readyz", h.readiness)
	}
	api := app.Group("/api")
	api.Get("/products", h.listProducts)
	if d.ProductImages != nil {
		api.Get("/products/:id/image", h.productImage)
	}
	api.Get("/promotions", h.listPromotions)
	api.Post("/calculate", h.calculate)
	if d.ResetLimits != nil {
		api.Post("/test/reset-limits", h.resetLimits)
	}
	return app
}
