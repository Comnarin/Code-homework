package domain

import (
	"errors"
	"fmt"
)

var (
	ErrEmptyOrder        = errors.New("order has no items")
	ErrInvalidQuantity   = errors.New("quantity must be a positive integer within the allowed limit")
	ErrUnknownProduct    = errors.New("unknown product")
	ErrInvalidMemberCard = errors.New("member card is not valid")
	ErrSlotHeld          = errors.New("limited product was already ordered within its window")
	ErrInvalidRule       = errors.New("invalid discount rule")
)

// SlotHeldError says which limited product was ordered recently and is not
// available until its window ends.
type SlotHeldError struct {
	ProductID   ProductID
	ProductName string
}

func (e *SlotHeldError) Error() string {
	return fmt.Sprintf("%s was ordered within its window and is not available yet", e.ProductName)
}

func (e *SlotHeldError) Unwrap() error { return ErrSlotHeld }
