package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/comnarin/foodstore/internal/domain"
)

type fakeProducts struct {
	items     []domain.Product
	err       error
	listCalls int
}

func (f *fakeProducts) List(context.Context) ([]domain.Product, error) {
	f.listCalls++
	return f.items, f.err
}

func (f *fakeProducts) FindByID(_ context.Context, id domain.ProductID) (domain.Product, bool, error) {
	if f.err != nil {
		return domain.Product{}, false, f.err
	}
	for _, p := range f.items {
		if p.ID == id {
			return p, true, nil
		}
	}
	return domain.Product{}, false, nil
}

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

// fakeLimiter mimics the Redis semantics: the first holder of a product wins,
// the same holder may claim again, any other holder gets ErrSlotHeld, and
// Release only frees a claim its caller still owns.
type fakeLimiter struct {
	held     map[domain.ProductID]string
	claims   []domain.ProductID // fresh claims, in order
	released []domain.ProductID // successful releases, in order
	err      error
}

func (f *fakeLimiter) Claim(_ context.Context, p domain.ProductID, holder string, _ time.Duration) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	if current, ok := f.held[p]; ok {
		if current == holder {
			return false, nil
		}
		return false, domain.ErrSlotHeld
	}
	if f.held == nil {
		f.held = map[domain.ProductID]string{}
	}
	f.held[p] = holder
	f.claims = append(f.claims, p)
	return true, nil
}

func (f *fakeLimiter) Release(_ context.Context, p domain.ProductID, holder string) error {
	if f.err != nil {
		return f.err
	}
	if f.held[p] == holder {
		delete(f.held, p)
		f.released = append(f.released, p)
	}
	return nil
}

func (f *fakeLimiter) Clear(_ context.Context, p domain.ProductID) error {
	if f.err != nil {
		return f.err
	}
	delete(f.held, p)
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
