package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestSlotHeldError_MatchesErrSlotHeldAndNamesProduct(t *testing.T) {
	err := error(&SlotHeldError{ProductID: "red", ProductName: "Red set"})

	if !errors.Is(err, ErrSlotHeld) {
		t.Fatal("SlotHeldError should match ErrSlotHeld via errors.Is")
	}
	var held *SlotHeldError
	if !errors.As(err, &held) || held.ProductName != "Red set" {
		t.Fatalf("errors.As failed or wrong name: %+v", held)
	}
	if !strings.Contains(err.Error(), "Red set") {
		t.Fatalf("message %q should name the product", err.Error())
	}
}
