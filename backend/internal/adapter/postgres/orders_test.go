package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/comnarin/foodstore/internal/domain"
	"github.com/comnarin/foodstore/internal/usecase"
)

var _ usecase.OrderRepository = (*OrderRepository)(nil)

func sampleOrder(card string) domain.Order {
	orange := domain.Product{ID: "orange", Name: "Orange set", Price: 12000}
	return domain.Order{
		ID:         uuid.NewString(),
		CustomerID: "cust-1",
		MemberCard: card,
		Lines:      []domain.Line{{Product: orange, Quantity: 2}},
		Quote: domain.Quote{
			TotalBefore: 24000,
			Discounts: []domain.Discount{
				{RuleCode: "PAIR_5", Scope: domain.ScopeItem, ProductID: "orange", Name: "Orange set", Bundles: 1, Amount: 1200},
				{RuleCode: "MEMBER_10", Scope: domain.ScopeMember, Amount: 2280},
			},
			FinalTotal: 20520,
		},
		CreatedAt: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC),
	}
}

func TestOrderRepository_SaveStoresOrderItemsAndDiscounts(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	o := sampleOrder("MEM0001")

	if err := NewOrderRepository(pool).Save(ctx, o); err != nil {
		t.Fatal(err)
	}

	var customer, card string
	var before, member, final int
	if err := pool.QueryRow(ctx,
		`SELECT customer_id, member_card, total_before_satang, member_discount_satang, final_total_satang
		 FROM orders WHERE id = $1::text::uuid`, o.ID).Scan(&customer, &card, &before, &member, &final); err != nil {
		t.Fatal(err)
	}
	if customer != "cust-1" || card != "MEM0001" || before != 24000 || member != 2280 || final != 20520 {
		t.Fatalf("order row = %s %s %d %d %d", customer, card, before, member, final)
	}

	var qty int
	if err := pool.QueryRow(ctx, "SELECT quantity FROM order_items WHERE order_id = $1::text::uuid AND product_id = 'orange'", o.ID).Scan(&qty); err != nil || qty != 2 {
		t.Fatalf("item qty = %d err = %v", qty, err)
	}
	var discounts int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM order_discounts WHERE order_id = $1::text::uuid", o.ID).Scan(&discounts); err != nil || discounts != 2 {
		t.Fatalf("discount rows = %d err = %v, want 2", discounts, err)
	}
}

func TestOrderRepository_NonMemberOrderStoresNullCard(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	o := sampleOrder("")

	if err := NewOrderRepository(pool).Save(ctx, o); err != nil {
		t.Fatal(err)
	}
	var isNull bool
	if err := pool.QueryRow(ctx, "SELECT member_card IS NULL FROM orders WHERE id = $1::text::uuid", o.ID).Scan(&isNull); err != nil || !isNull {
		t.Fatalf("member_card should be NULL: isNull=%v err=%v", isNull, err)
	}
}

func TestOrderRepository_FailedSaveLeavesNoPartialRows(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	o := sampleOrder("MEM0001")
	o.Lines[0].Product.ID = "no-such-product" // violates the order_items foreign key

	if err := NewOrderRepository(pool).Save(ctx, o); err == nil {
		t.Fatal("want an error for an unknown product")
	}
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM orders").Scan(&n); err != nil || n != 0 {
		t.Fatalf("orders = %d err = %v; the transaction must roll back", n, err)
	}
}
