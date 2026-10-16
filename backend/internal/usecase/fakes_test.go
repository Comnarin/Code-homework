package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/comnarin/foodstore/internal/domain"
)

type fakeProducts struct {
	items []domain.Product
	err   error
}

func (f *fakeProducts) List(context.Context) ([]domain.Product, error) { return f.items, f.err }

type fakeMembers struct{ byCard map[string]domain.Member }

func (f *fakeMembers) FindByCard(_ context.Context, card string) (domain.Member, bool, error) {
	m, ok := f.byCard[card]
	return m, ok, nil
}

type fakeRules struct{ rules []domain.Rule }

func (f *fakeRules) ActiveRules(context.Context) ([]domain.Rule, error) { return f.rules, nil }

type fakeLimits struct {
	limits map[domain.ProductID]time.Duration
}

func (f *fakeLimits) ActiveLimits(context.Context) (map[domain.ProductID]time.Duration, error) {
	return f.limits, nil
}

type fakeOrders struct {
	saved []domain.Order
	err   error
}

func (f *fakeOrders) Save(_ context.Context, o domain.Order) error {
	if f.err != nil {
		return f.err
	}
	f.saved = append(f.saved, o)
	return nil
}

// fakeLimiter mimics the Redis semantics: free slot -> claim; same owner -> ok;
// other owner -> ErrSlotHeld.
type fakeLimiter struct {
	holders map[domain.ProductID]string
	claims  []domain.ProductID
	err     error
}

func (f *fakeLimiter) Claim(_ context.Context, p domain.ProductID, owner string, _ time.Duration) error {
	if f.err != nil {
		return f.err
	}
	if holder, ok := f.holders[p]; ok && holder != owner {
		return domain.ErrSlotHeld
	}
	if f.holders == nil {
		f.holders = map[domain.ProductID]string{}
	}
	f.holders[p] = owner
	f.claims = append(f.claims, p)
	return nil
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

type seqIDs struct{ n int }

func (s *seqIDs) NewID() string {
	s.n++
	return fmt.Sprintf("order-%d", s.n)
}

func seedProducts() []domain.Product {
	return []domain.Product{
		{ID: "red", Name: "Red set", Price: 5000},
		{ID: "green", Name: "Green set", Price: 4000},
		{ID: "blue", Name: "Blue set", Price: 3000},
		{ID: "yellow", Name: "Yellow set", Price: 5000},
		{ID: "pink", Name: "Pink set", Price: 8000},
		{ID: "purple", Name: "Purple set", Price: 9000},
		{ID: "orange", Name: "Orange set", Price: 12000},
	}
}
