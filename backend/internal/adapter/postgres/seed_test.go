package postgres

import "testing"

func TestSeed_MatchesThePDF(t *testing.T) {
	gdb := testDB(t)

	var products []struct {
		ID          string
		PriceSatang int
	}
	if err := gdb.Raw("SELECT id, price_satang FROM products ORDER BY sort_order").Scan(&products).Error; err != nil {
		t.Fatal(err)
	}
	want := []struct {
		id    string
		price int
	}{
		{"red", 5000}, {"green", 4000}, {"blue", 3000}, {"yellow", 5000},
		{"pink", 8000}, {"purple", 9000}, {"orange", 12000},
	}
	if len(products) != len(want) {
		t.Fatalf("got %d products, want %d", len(products), len(want))
	}
	for i, w := range want {
		if products[i].ID != w.id || products[i].PriceSatang != w.price {
			t.Fatalf("row %d = %+v, want %+v", i, products[i], w)
		}
	}

	count := func(query string) int {
		t.Helper()
		var n int
		if err := gdb.Raw(query).Scan(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(`SELECT count(*) FROM rule_products rp JOIN rules r ON r.id = rp.rule_id
		WHERE r.code = 'PAIR_5' AND rp.product_id IN ('orange','pink','green')`); n != 3 {
		t.Fatalf("PAIR_5 products = %d, want 3", n)
	}
	if n := count("SELECT window_seconds FROM product_limits WHERE product_id = 'red'"); n != 3600 {
		t.Fatalf("red window = %d, want 3600", n)
	}
	if n := count("SELECT count(*) FROM members WHERE card_number IN ('MEM0001','MEM0002')"); n != 2 {
		t.Fatalf("members = %d, want 2", n)
	}
}
