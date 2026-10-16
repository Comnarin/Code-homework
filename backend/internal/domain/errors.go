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
	ErrSlotHeld          = errors.New("limited product is held by another customer")
	ErrInvalidRule       = errors.New("invalid discount rule")
)

// SlotHeldError says which limited product another customer is holding.
type SlotHeldError struct {
	ProductID   ProductID
	ProductName string
}

func (e *SlotHeldError) Error() string {
	return fmt.Sprintf("%s is held by another customer", e.ProductName)
}

func (e *SlotHeldError) Unwrap() error { return ErrSlotHeld }
