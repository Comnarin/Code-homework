package domain

import "strings"

type ProductID string

type Product struct {
	ID    ProductID
	Name  string
	Price Money
	// ImageKey names the product's picture in object storage. Empty means no
	// picture has been uploaded yet.
	ImageKey string
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

// Cart is one product and how many sets of it were ordered.
type Cart struct {
	Product  Product
	Quantity int
}
