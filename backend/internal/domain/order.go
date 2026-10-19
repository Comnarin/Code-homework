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
	// ClientKey is the optional stable key of the customer (for example one
	// browser session). Empty means every order is a different holder.
	ClientKey string
}

// Holder identifies who owns a limited product claimed by this order: the
// client key when there is one, otherwise the order itself.
func (o Order) Holder() string {
	if o.ClientKey != "" {
		return o.ClientKey
	}
	return o.ID
}
