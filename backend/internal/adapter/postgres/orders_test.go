package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/comnarin/foodstore/internal/domain"
	"github.com/comnarin/foodstore/internal/usecase"
)

var _ usecase.OrderRepository = (*OrderRepository)(nil)

func sampleOrder(card string) domain.Order {
	orange := domain.Product{ID: "orange", Name: "Orange set", Price: 12000}
	return domain.Order{
		ID:         uuid.NewString(),
		MemberCard: card,
		Carts:      []domain.Cart{{Product: orange, Quantity: 2}},
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

type savedOrderRow struct {
	MemberCard           *string
	TotalBeforeSatang    int
	MemberDiscountSatang int
	FinalTotalSatang     int
}

func loadOrder(t *testing.T, gdb *gorm.DB, id string) savedOrderRow {
	t.Helper()
	var row savedOrderRow
	res := gdb.Raw(`SELECT member_card, total_before_satang, member_discount_satang, final_total_satang
		FROM orders WHERE id::text = ?`, id).Scan(&row)
	if res.Error != nil || res.RowsAffected != 1 {
		t.Fatalf("load order: rows=%d err=%v", res.RowsAffected, res.Error)
	}
	return row
}

func count(t *testing.T, gdb *gorm.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := gdb.Raw(query, args...).Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func TestOrderRepository_SaveStoresOrderItemsAndDiscounts(t *testing.T) {
	gdb := testDB(t)
	o := sampleOrder("MEM0001")

	if err := NewOrderRepository(gdb).Save(context.Background(), o); err != nil {
		t.Fatal(err)
	}

	row := loadOrder(t, gdb, o.ID)
	if row.MemberCard == nil || *row.MemberCard != "MEM0001" ||
		row.TotalBeforeSatang != 24000 || row.MemberDiscountSatang != 2280 || row.FinalTotalSatang != 20520 {
		t.Fatalf("order row = %+v", row)
	}
	if q := count(t, gdb, "SELECT quantity FROM order_items WHERE order_id::text = ? AND product_id = 'orange'", o.ID); q != 2 {
		t.Fatalf("item qty = %d, want 2", q)
	}
	if n := count(t, gdb, "SELECT count(*) FROM order_discounts WHERE order_id::text = ?", o.ID); n != 2 {
		t.Fatalf("discount rows = %d, want 2", n)
	}
}

func TestOrderRepository_NonMemberOrderStoresNullCard(t *testing.T) {
	gdb := testDB(t)
	o := sampleOrder("")

	if err := NewOrderRepository(gdb).Save(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if row := loadOrder(t, gdb, o.ID); row.MemberCard != nil {
		t.Fatalf("member_card should be NULL, got %q", *row.MemberCard)
	}
}

func TestOrderRepository_StoresTheOrderTimestampWeGave(t *testing.T) {
	gdb := testDB(t)
	o := sampleOrder("")

	if err := NewOrderRepository(gdb).Save(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	var got time.Time
	if err := gdb.Raw("SELECT created_at FROM orders WHERE id::text = ?", o.ID).Scan(&got).Error; err != nil {
		t.Fatal(err)
	}
	if !got.Equal(o.CreatedAt) {
		t.Fatalf("created_at = %v, want %v (GORM must not overwrite it)", got, o.CreatedAt)
	}
}

func TestOrderRepository_FailedSaveLeavesNoPartialRows(t *testing.T) {
	gdb := testDB(t)
	o := sampleOrder("MEM0001")
	o.Carts[0].Product.ID = "no-such-product" // violates the order_items foreign key

	if err := NewOrderRepository(gdb).Save(context.Background(), o); err == nil {
		t.Fatal("want an error for an unknown product")
	}
	if n := count(t, gdb, "SELECT count(*) FROM orders"); n != 0 {
		t.Fatalf("orders = %d; the transaction must roll back", n)
	}
	if n := count(t, gdb, "SELECT count(*) FROM order_discounts"); n != 0 {
		t.Fatalf("order_discounts = %d; the transaction must roll back", n)
	}
}
