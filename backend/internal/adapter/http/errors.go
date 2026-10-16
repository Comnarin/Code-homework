package httpapi

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/gofiber/fiber/v3"

	"github.com/comnarin/foodstore/internal/domain"
)

var errBadRequest = errors.New("invalid request body")

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// errorHandler is the single place where errors become HTTP responses.
// Handlers just return errors; unexpected ones are logged and never leaked.
func errorHandler(c fiber.Ctx, err error) error {
	status, detail := mapError(err)
	if status == fiber.StatusInternalServerError {
		slog.Error("request failed", "method", c.Method(), "path", c.Path(), "err", err)
	}
	return c.Status(status).JSON(errorBody{Error: detail})
}

func mapError(err error) (int, errorDetail) {
	var held *domain.SlotHeldError
	var fiberErr *fiber.Error
	switch {
	case errors.As(err, &held):
		return fiber.StatusConflict, errorDetail{
			Code:    "LIMITED_PRODUCT_UNAVAILABLE",
			Message: fmt.Sprintf("%s is reserved by another customer right now. Please try again later.", held.ProductName),
		}
	case errors.Is(err, domain.ErrInvalidMemberCard):
		return fiber.StatusBadRequest, errorDetail{Code: "INVALID_MEMBER_CARD", Message: "Member card not found."}
	case errors.Is(err, errBadRequest),
		errors.Is(err, domain.ErrEmptyOrder),
		errors.Is(err, domain.ErrInvalidQuantity),
		errors.Is(err, domain.ErrUnknownProduct):
		return fiber.StatusBadRequest, errorDetail{Code: "INVALID_REQUEST", Message: err.Error()}
	case errors.As(err, &fiberErr) && fiberErr.Code == fiber.StatusNotFound:
		return fiber.StatusNotFound, errorDetail{Code: "NOT_FOUND", Message: "Route not found."}
	default:
		return fiber.StatusInternalServerError, errorDetail{Code: "INTERNAL", Message: "Something went wrong. Please try again."}
	}
}
