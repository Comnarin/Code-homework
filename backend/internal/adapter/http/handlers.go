package httpapi

import (
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/comnarin/foodstore/internal/domain"
	"github.com/comnarin/foodstore/internal/usecase"
)

type handlers struct {
	images     ImageGetter
	products   ProductUsecase
	promotions PromotionUsecase
	calculator OrderCalculator
	reset      LimitResetter
	ready      ReadinessChecker
}

// maxClientKeyLength bounds the optional X-Client-Key header.
const maxClientKeyLength = 128

func (h handlers) health(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }

func (h handlers) readiness(c fiber.Ctx) error {
	r, err := h.ready.Check(c.Context())
	if err != nil {
		slog.ErrorContext(c.Context(), "not ready", "err", err)
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "unavailable"})
	}
	if len(r.Degraded) > 0 {
		// Serving, but an optional dependency is down. The detail is logged by
		// the checker, never sent to clients.
		return c.JSON(fiber.Map{"status": "degraded"})
	}
	return c.JSON(fiber.Map{"status": "ok"})
}

func (h handlers) listProducts(c fiber.Ctx) error {
	items, err := h.products.List(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(toProductDTOs(items))
}

// productImage serves a product's picture, or the default one. Images are
// untrusted bytes from object storage, so the response is locked down: no type
// sniffing and a sandboxing policy, even though domain.NewImage already limits
// what can get here.
func (h handlers) productImage(c fiber.Ctx) error {
	res, err := h.images.Execute(c.Context(), c.Params("id"))
	if errors.Is(err, domain.ErrUnknownProduct) {
		return fiber.ErrNotFound
	}
	if err != nil {
		return err
	}
	c.Set("Content-Type", res.Image.ContentType)
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	if res.Source == usecase.ImageFromStore {
		c.Set("Cache-Control", "public, max-age=300")
	} else {
		// A stand-in must not be cached, or the real picture could not appear
		// once storage recovers or an image is uploaded.
		c.Set("Cache-Control", "no-store")
	}
	return c.Send(res.Image.Data)
}

func (h handlers) listPromotions(c fiber.Ctx) error {
	items, err := h.promotions.List(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(toPromotionDTOs(items))
}

func (h handlers) resetLimits(c fiber.Ctx) error {
	if err := h.reset.Execute(c.Context()); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h handlers) calculate(c fiber.Ctx) error {
	if mediaType, _, err := mime.ParseMediaType(c.Get(fiber.HeaderContentType)); err != nil || mediaType != fiber.MIMEApplicationJSON {
		return fiber.ErrUnsupportedMediaType
	}
	clientKey := strings.TrimSpace(c.Get("X-Client-Key"))
	if len(clientKey) > maxClientKeyLength {
		return fmt.Errorf("%w: client key is too long", errBadRequest)
	}
	var req calculateRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fmt.Errorf("%w: %v", errBadRequest, err)
	}
	in := usecase.CalculateInput{
		MemberCard: req.MemberCard,
		ClientKey:  clientKey,
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
