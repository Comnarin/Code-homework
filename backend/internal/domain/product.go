package domain

import "strings"

type ProductID string

type Product struct {
	ID    ProductID
	Name  string
	Price Money
}

type Member struct {
	CardNumber string
	FullName   string
	Active     bool
}

// NormalizeCard puts a member card number into its stored form.
func NormalizeCard(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// Line is one product and how many sets of it were ordered.
type Line struct {
	Product  Product
	Quantity int
}
