// Package httpapi is the Fiber delivery adapter: it parses requests, calls
// use cases and presents results. It holds no business rules.
package httpapi

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
)

type Deps struct {
	Products   ProductLister
	Promotions PromotionLister
	Calculator OrderCalculator
	// ResetLimits is optional. When nil, the test-only reset route is not registered.
	ResetLimits LimitResetter
}

func NewApp(d Deps) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: errorHandler})
	app.Use(recover.New())

	h := handlers{products: d.Products, promotions: d.Promotions, calculator: d.Calculator, reset: d.ResetLimits}
	app.Get("/healthz", h.health)
	api := app.Group("/api")
	api.Get("/products", h.listProducts)
	api.Get("/promotions", h.listPromotions)
	api.Post("/calculate", h.calculate)
	if d.ResetLimits != nil {
		api.Post("/test/reset-limits", h.resetLimits)
	}
	return app
}
