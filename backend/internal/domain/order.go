package domain

import "time"

// Order is a priced order ready to be stored. MemberCard is empty when the
// customer is not a member.
type Order struct {
	ID         string
	MemberCard string
	Carts      []Cart
	Quote      Quote
	CreatedAt  time.Time
}
