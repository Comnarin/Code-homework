package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/comnarin/foodstore/internal/domain"
	"github.com/comnarin/foodstore/internal/usecase"
)

type fakeLister struct {
	items []domain.Product
	err   error
}

func (f fakeLister) Execute(context.Context) ([]domain.Product, error) { return f.items, f.err }

type fakeCalculator struct {
	got   usecase.CalculateInput
	quote domain.Quote
	err   error
}

func (f *fakeCalculator) Execute(_ context.Context, in usecase.CalculateInput) (domain.Quote, error) {
	f.got = in
	return f.quote, f.err
}

type env struct {
	app  *fiber.App
	calc *fakeCalculator
}

func newEnv(t *testing.T, calc *fakeCalculator, lister fakeLister) env {
	t.Helper()
	return env{
		app:  NewApp(Deps{Products: lister, Calculator: calc, NewID: uuid.NewString}),
		calc: calc,
	}
}

func (e env) do(t *testing.T, method, path, body, cookie string) (*http.Response, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.Header.Set("Cookie", "cid="+cookie)
	}
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	return resp, string(raw)
}

func cidFrom(resp *http.Response) string {
	for _, c := range resp.Cookies() {
		if c.Name == "cid" {
			return c.Value
		}
	}
	return ""
}

func TestHealthz(t *testing.T) {
	e := newEnv(t, &fakeCalculator{}, fakeLister{})
	resp, _ := e.do(t, "GET", "/healthz", "", "")
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestGetProducts_ReturnsTheCatalogInSatang(t *testing.T) {
	e := newEnv(t, &fakeCalculator{}, fakeLister{items: []domain.Product{{ID: "red", Name: "Red set", Price: 5000}}})

	resp, body := e.do(t, "GET", "/api/products", "", "")

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d body=%s", resp.StatusCode, body)
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0]["id"] != "red" || got[0]["name"] != "Red set" || got[0]["priceSatang"] != float64(5000) {
		t.Fatalf("body = %s", body)
	}
}

func TestCid_IssuedWhenMissingAndReusedWhenValid(t *testing.T) {
	e := newEnv(t, &fakeCalculator{}, fakeLister{})

	first, _ := e.do(t, "GET", "/api/products", "", "")
	issued := cidFrom(first)
	if _, err := uuid.Parse(issued); err != nil {
		t.Fatalf("first response should set a UUID cid, got %q", issued)
	}
	var cookie *http.Cookie
	for _, c := range first.Cookies() {
		if c.Name == "cid" {
			cookie = c
		}
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" {
		t.Fatalf("cid cookie flags wrong: %+v", cookie)
	}

	second, _ := e.do(t, "GET", "/api/products", "", issued)
	if cidFrom(second) != "" {
		t.Fatal("a valid cid must be reused, not re-issued")
	}
}

func TestCid_InvalidCookieIsReplaced(t *testing.T) {
	e := newEnv(t, &fakeCalculator{}, fakeLister{})

	resp, _ := e.do(t, "GET", "/api/products", "", "not-a-uuid")

	if _, err := uuid.Parse(cidFrom(resp)); err != nil {
		t.Fatalf("an invalid cid should be replaced with a fresh UUID, got %q", cidFrom(resp))
	}
}

func TestPostCalculate_PassesTheCookieCustomerAndReturnsTheBreakdown(t *testing.T) {
	calc := &fakeCalculator{quote: domain.Quote{
		TotalBefore: 24000,
		Discounts: []domain.Discount{
			{RuleCode: "PAIR_5", Scope: domain.ScopeItem, ProductID: "orange", Name: "Orange set", Bundles: 1, Amount: 1200},
			{RuleCode: "MEMBER_10", Scope: domain.ScopeMember, Amount: 2280},
		},
		FinalTotal: 20520,
	}}
	e := newEnv(t, calc, fakeLister{})
	cid := uuid.NewString()

	resp, body := e.do(t, "POST", "/api/calculate",
		`{"memberCard":"MEM0001","items":[{"productId":"orange","quantity":2}]}`, cid)

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d body=%s", resp.StatusCode, body)
	}
	if calc.got.CustomerID != cid || calc.got.MemberCard != "MEM0001" ||
		len(calc.got.Items) != 1 || calc.got.Items[0].ProductID != "orange" || calc.got.Items[0].Quantity != 2 {
		t.Fatalf("use case input = %+v", calc.got)
	}
	var got struct {
		TotalBeforeSatang    int `json:"totalBeforeSatang"`
		MemberDiscountSatang int `json:"memberDiscountSatang"`
		FinalTotalSatang     int `json:"finalTotalSatang"`
		ItemDiscounts        []struct {
			RuleCode     string `json:"ruleCode"`
			ProductID    string `json:"productId"`
			Name         string `json:"name"`
			Bundles      int    `json:"bundles"`
			AmountSatang int    `json:"amountSatang"`
		} `json:"itemDiscounts"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if got.TotalBeforeSatang != 24000 || got.MemberDiscountSatang != 2280 || got.FinalTotalSatang != 20520 ||
		len(got.ItemDiscounts) != 1 || got.ItemDiscounts[0].AmountSatang != 1200 || got.ItemDiscounts[0].Name != "Orange set" {
		t.Fatalf("body = %s", body)
	}
}

func TestPostCalculate_NoDiscountsSerializesAsEmptyArrayNotNull(t *testing.T) {
	e := newEnv(t, &fakeCalculator{quote: domain.Quote{TotalBefore: 3000, FinalTotal: 3000}}, fakeLister{})

	_, body := e.do(t, "POST", "/api/calculate", `{"items":[{"productId":"blue","quantity":1}]}`, "")

	if !strings.Contains(body, `"itemDiscounts":[]`) {
		t.Fatalf("itemDiscounts must be [] not null: %s", body)
	}
}

func TestErrors_MapToStatusAndCode(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantInMsg  string
	}{
		{"empty order", domain.ErrEmptyOrder, 400, "INVALID_REQUEST", ""},
		{"bad quantity", domain.ErrInvalidQuantity, 400, "INVALID_REQUEST", ""},
		{"unknown product", domain.ErrUnknownProduct, 400, "INVALID_REQUEST", ""},
		{"bad member card", domain.ErrInvalidMemberCard, 400, "INVALID_MEMBER_CARD", "Member card not found"},
		{"slot held", &domain.SlotHeldError{ProductID: "red", ProductName: "Red set"}, 409, "LIMITED_PRODUCT_UNAVAILABLE", "Red set"},
		{"unexpected failure", errors.New("redis: connection refused"), 500, "INTERNAL", "Something went wrong"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, &fakeCalculator{err: tt.err}, fakeLister{})

			resp, body := e.do(t, "POST", "/api/calculate", `{"items":[{"productId":"red","quantity":1}]}`, "")

			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d (%s)", resp.StatusCode, tt.wantStatus, body)
			}
			var got struct {
				Error struct{ Code, Message string } `json:"error"`
			}
			if err := json.Unmarshal([]byte(body), &got); err != nil {
				t.Fatal(err)
			}
			if got.Error.Code != tt.wantCode || !strings.Contains(got.Error.Message, tt.wantInMsg) {
				t.Fatalf("error = %+v", got.Error)
			}
			if strings.Contains(body, "connection refused") {
				t.Fatalf("internal error details must not leak to clients: %s", body)
			}
		})
	}
}

func TestPostCalculate_MalformedOrEmptyBodyIs400(t *testing.T) {
	for name, body := range map[string]string{
		"empty body":    "",
		"not json":      "{{{",
		"wrong shape":   `{"items":"oops"}`,
		"quantity text": `{"items":[{"productId":"red","quantity":"two"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, &fakeCalculator{}, fakeLister{})

			resp, out := e.do(t, "POST", "/api/calculate", body, "")

			if resp.StatusCode != 400 || !strings.Contains(out, "INVALID_REQUEST") {
				t.Fatalf("status = %d body = %s, want 400 INVALID_REQUEST", resp.StatusCode, out)
			}
		})
	}
}

func TestUnknownRouteIsJSON404(t *testing.T) {
	e := newEnv(t, &fakeCalculator{}, fakeLister{})

	resp, body := e.do(t, "GET", "/api/nope", "", "")

	if resp.StatusCode != 404 || !strings.Contains(body, "NOT_FOUND") {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
}

func TestGetProducts_ListerFailureIs500(t *testing.T) {
	e := newEnv(t, &fakeCalculator{}, fakeLister{err: errors.New("db down")})

	resp, body := e.do(t, "GET", "/api/products", "", "")

	if resp.StatusCode != 500 || strings.Contains(body, "db down") {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
}
