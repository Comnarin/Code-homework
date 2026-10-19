package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/comnarin/foodstore/internal/domain"
	"github.com/comnarin/foodstore/internal/usecase"
)

var _ usecase.LimitWindowClearer = (*OrderRepository)(nil)

var t0 = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

func limitedOrder(key string, at time.Time, products ...domain.ProductID) domain.Order {
	o := domain.Order{ID: uuid.NewString(), ClientKey: key, CreatedAt: at}
	for _, p := range products {
		o.Carts = append(o.Carts, domain.Cart{Product: domain.Product{ID: p, Name: strings.ToUpper(string(p[:1])) + string(p[1:]) + " set", Price: 5000}, Quantity: 1})
	}
	return o
}

func startedFlag(t *testing.T, gdb *gorm.DB, orderID, product string) bool {
	t.Helper()
	var b bool
	if err := gdb.Raw("SELECT started_limit_window FROM order_items WHERE order_id::text = ? AND product_id = ?", orderID, product).Scan(&b).Error; err != nil {
		t.Fatal(err)
	}
	return b
}

func mustHeld(t *testing.T, err error, product string) {
	t.Helper()
	var held *domain.SlotHeldError
	if !errors.As(err, &held) || string(held.ProductID) != product {
		t.Fatalf("err = %v, want *SlotHeldError for %s", err, product)
	}
}

func TestSave_FirstRedStoresClientKeyAndOpensTheWindow(t *testing.T) {
	gdb := testDB(t)
	repo := NewOrderRepository(gdb)
	a := limitedOrder("holder-a", t0, "red")
	if err := repo.Save(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if !startedFlag(t, gdb, a.ID, "red") {
		t.Fatal("first red order must open the window")
	}
	if n := count(t, gdb, "SELECT count(*) FROM orders WHERE id::text = ? AND client_key = 'holder-a'", a.ID); n != 1 {
		t.Fatalf("client_key not stored")
	}
}

func TestSave_OtherHolderRejectedSameHolderAllowedWithoutExtendingTheWindow(t *testing.T) {
	gdb := testDB(t)
	repo := NewOrderRepository(gdb)
	ctx := context.Background()
	if err := repo.Save(ctx, limitedOrder("a", t0, "red")); err != nil {
		t.Fatal(err)
	}

	b := limitedOrder("b", t0.Add(30*time.Minute), "red")
	err := repo.Save(ctx, b)
	mustHeld(t, err, "red")
	var held *domain.SlotHeldError
	errors.As(err, &held)
	if held.ProductName != "Red set" {
		t.Fatalf("name = %q", held.ProductName)
	}
	if n := count(t, gdb, "SELECT count(*) FROM orders WHERE id::text = ?", b.ID); n != 0 {
		t.Fatal("a rejected order must write nothing")
	}

	a2 := limitedOrder("a", t0.Add(30*time.Minute), "red")
	if err := repo.Save(ctx, a2); err != nil {
		t.Fatalf("same holder re-price: %v", err)
	}
	if startedFlag(t, gdb, a2.ID, "red") {
		t.Fatal("a repeat order must not open a new window")
	}

	// The re-price at +30m did not extend the window: b gets in after +60m.
	b2 := limitedOrder("b", t0.Add(time.Hour+time.Second), "red")
	if err := repo.Save(ctx, b2); err != nil {
		t.Fatalf("other holder after the window: %v", err)
	}
	if !startedFlag(t, gdb, b2.ID, "red") {
		t.Fatal("b opens a new window")
	}
}

func TestSave_WindowEndsExactlyAtTheWindowLength(t *testing.T) {
	gdb := testDB(t)
	repo := NewOrderRepository(gdb)
	if err := repo.Save(context.Background(), limitedOrder("a", t0, "red")); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(context.Background(), limitedOrder("b", t0.Add(time.Hour), "red")); err != nil {
		t.Fatalf("exactly at +window must be allowed: %v", err)
	}
}

func TestSave_NoClientKeyMeansDifferentHolders(t *testing.T) {
	gdb := testDB(t)
	repo := NewOrderRepository(gdb)
	if err := repo.Save(context.Background(), limitedOrder("", t0, "red")); err != nil {
		t.Fatal(err)
	}
	mustHeld(t, repo.Save(context.Background(), limitedOrder("", t0.Add(time.Minute), "red")), "red")
}

func TestSave_InactiveLimitIsNotEnforced(t *testing.T) {
	gdb := testDB(t)
	if err := gdb.Exec("UPDATE product_limits SET active = false WHERE product_id = 'red'").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { gdb.Exec("UPDATE product_limits SET active = true WHERE product_id = 'red'") })
	repo := NewOrderRepository(gdb)
	a := limitedOrder("a", t0, "red")
	if err := repo.Save(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(context.Background(), limitedOrder("b", t0.Add(time.Minute), "red")); err != nil {
		t.Fatalf("inactive limit must not block: %v", err)
	}
	if startedFlag(t, gdb, a.ID, "red") {
		t.Fatal("no window is opened for an inactive limit")
	}
}

func TestSave_TwoLimitedProductsInReverseOrderDoNotDeadlock(t *testing.T) {
	gdb := testDB(t)
	if err := gdb.Exec("INSERT INTO product_limits (product_id, window_seconds, active) VALUES ('pink', 3600, true) ON CONFLICT DO NOTHING").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { gdb.Exec("DELETE FROM product_limits WHERE product_id = 'pink'") })
	repo := NewOrderRepository(gdb)

	var wg sync.WaitGroup
	errs := make(chan error, 100)
	for g, order := range [][]domain.ProductID{{"red", "pink"}, {"pink", "red"}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				key := fmt.Sprintf("g%d-%d", g, i)
				if err := repo.Save(context.Background(), limitedOrder(key, t0, order...)); err != nil {
					var held *domain.SlotHeldError
					if !errors.As(err, &held) {
						errs <- err
					}
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("unexpected error (deadlock?): %v", err)
	}
}

func TestSave_ContentionExactlyOneHolderWins(t *testing.T) {
	gdb := testDB(t)
	repo := NewOrderRepository(gdb)
	const n = 150
	var ready, start sync.WaitGroup
	ready.Add(n)
	start.Add(1)
	results := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			o := limitedOrder(fmt.Sprintf("race-%d", i), t0, "red")
			ready.Done()
			start.Wait()
			results <- repo.Save(context.Background(), o)
		}()
	}
	ready.Wait()
	start.Done()

	var ok, held, other int
	for i := 0; i < n; i++ {
		err := <-results
		var h *domain.SlotHeldError
		switch {
		case err == nil:
			ok++
		case errors.As(err, &h):
			held++
		default:
			other++
			t.Logf("other error: %v", err)
		}
	}
	if ok != 1 || held != n-1 || other != 0 {
		t.Fatalf("ok=%d held=%d other=%d, want 1/%d/0", ok, held, other, n-1)
	}
	if c := count(t, gdb, "SELECT count(*) FROM order_items WHERE started_limit_window"); c != 1 {
		t.Fatalf("started_limit_window rows = %d, want 1", c)
	}
}

func TestClearLimitWindows_LetsANewHolderIn(t *testing.T) {
	gdb := testDB(t)
	repo := NewOrderRepository(gdb)
	ctx := context.Background()
	if err := repo.Save(ctx, limitedOrder("a", t0, "red")); err != nil {
		t.Fatal(err)
	}
	mustHeld(t, repo.Save(ctx, limitedOrder("b", t0.Add(time.Minute), "red")), "red")
	if err := repo.ClearLimitWindows(ctx); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, limitedOrder("b", t0.Add(2*time.Minute), "red")); err != nil {
		t.Fatalf("after clear: %v", err)
	}
}
