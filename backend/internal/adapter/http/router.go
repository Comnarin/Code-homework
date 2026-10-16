// Package httpapi is the Fiber delivery adapter: it parses requests, calls
// use cases and presents results. It holds no business rules.
package httpapi

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
)

type Deps struct {
	Products     ProductLister
	Calculator   OrderCalculator
	NewID        func() string
	CookieSecure bool
}

func NewApp(d Deps) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: errorHandler})
	app.Use(recover.New())
	app.Use(cidMiddleware(d.NewID, d.CookieSecure))

	h := handlers{products: d.Products, calculator: d.Calculator}
	app.Get("/healthz", h.health)
	api := app.Group("/api")
	api.Get("/products", h.listProducts)
	api.Post("/calculate", h.calculate)
	return app
}
