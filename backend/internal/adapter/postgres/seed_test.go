package postgres

import (
	"context"
	"testing"
)

func TestSeed_MatchesThePDF(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	rows, err := pool.Query(ctx, "SELECT id, price_satang FROM products ORDER BY sort_order")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := []struct {
		id    string
		price int
	}{
		{"red", 5000}, {"green", 4000}, {"blue", 3000}, {"yellow", 5000},
		{"pink", 8000}, {"purple", 9000}, {"orange", 12000},
	}
	i := 0
	for rows.Next() {
		var id string
		var price int
		if err := rows.Scan(&id, &price); err != nil {
			t.Fatal(err)
		}
		if i >= len(want) || id != want[i].id || price != want[i].price {
			t.Fatalf("row %d = %s/%d, want %+v", i, id, price, want[i])
		}
		i++
	}
	if i != len(want) {
		t.Fatalf("got %d products, want %d", i, len(want))
	}

	var pairProducts int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM rule_products rp JOIN rules r ON r.id = rp.rule_id
		 WHERE r.code = 'PAIR_5' AND rp.product_id IN ('orange','pink','green')`).Scan(&pairProducts); err != nil || pairProducts != 3 {
		t.Fatalf("PAIR_5 products = %d, err = %v; want 3", pairProducts, err)
	}

	var redWindow int
	if err := pool.QueryRow(ctx, "SELECT window_seconds FROM product_limits WHERE product_id = 'red'").Scan(&redWindow); err != nil || redWindow != 3600 {
		t.Fatalf("red window = %d, err = %v; want 3600", redWindow, err)
	}

	var members int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM members WHERE card_number IN ('MEM0001','MEM0002')").Scan(&members); err != nil || members != 2 {
		t.Fatalf("members = %d, err = %v; want 2", members, err)
	}
}
