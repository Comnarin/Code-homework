package httpapi

import (
	"context"
	"fmt"

	"github.com/gofiber/fiber/v3"

	"github.com/comnarin/foodstore/internal/domain"
	"github.com/comnarin/foodstore/internal/usecase"
)

// The handlers depend on these small interfaces, not on concrete use cases.
type ProductLister interface {
	Execute(ctx context.Context) ([]domain.Product, error)
}

type OrderCalculator interface {
	Execute(ctx context.Context, in usecase.CalculateInput) (domain.Quote, error)
}

type handlers struct {
	products   ProductLister
	calculator OrderCalculator
}

func (h handlers) health(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }

func (h handlers) listProducts(c fiber.Ctx) error {
	items, err := h.products.Execute(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(toProductDTOs(items))
}

func (h handlers) calculate(c fiber.Ctx) error {
	var req calculateRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fmt.Errorf("%w: %v", errBadRequest, err)
	}
	in := usecase.CalculateInput{
		MemberCard: req.MemberCard,
		Items:      make([]usecase.ItemInput, len(req.Items)),
	}
	for i, it := range req.Items {
		in.Items[i] = usecase.ItemInput{ProductID: it.ProductID, Quantity: it.Quantity}
	}
	quote, err := h.calculator.Execute(c.Context(), in)
	if err != nil {
		return err
	}
	return c.JSON(toCalculateResponse(quote))
}
