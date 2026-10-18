package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/comnarin/foodstore/internal/domain"
	"github.com/comnarin/foodstore/internal/usecase"
)

type fakeLister struct {
	items []domain.Product
	err   error
}

func (f fakeLister) List(context.Context) ([]domain.Product, error) {
	return f.items, f.err
}

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
		app:  NewApp(Deps{Products: lister, Calculator: calc}),
		calc: calc,
	}
}

func (e env) do(t *testing.T, method, path, body string) (*http.Response, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	return resp, string(raw)
}

func TestHealthz(t *testing.T) {
	e := newEnv(t, &fakeCalculator{}, fakeLister{})
	resp, _ := e.do(t, "GET", "/healthz", "")
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestGetProducts_ReturnsTheCatalogInSatang(t *testing.T) {
	e := newEnv(t, &fakeCalculator{}, fakeLister{items: []domain.Product{{ID: "red", Name: "Red set", Price: 5000}}})

	resp, body := e.do(t, "GET", "/api/products", "")

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

func TestPostCalculate_PassesTheOrderAndReturnsTheBreakdown(t *testing.T) {
	calc := &fakeCalculator{quote: domain.Quote{
		TotalBefore: 24000,
		Discounts: []domain.Discount{
			{RuleCode: "PAIR_5", Scope: domain.ScopeItem, ProductID: "orange", Name: "Orange set", Bundles: 1, Amount: 1200},
			{RuleCode: "MEMBER_10", Scope: domain.ScopeMember, Amount: 2280},
		},
		FinalTotal: 20520,
	}}
	e := newEnv(t, calc, fakeLister{})

	resp, body := e.do(t, "POST", "/api/calculate",
		`{"memberCard":"MEM0001","items":[{"productId":"orange","quantity":2}]}`)

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d body=%s", resp.StatusCode, body)
	}
	if calc.got.MemberCard != "MEM0001" ||
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

	_, body := e.do(t, "POST", "/api/calculate", `{"items":[{"productId":"blue","quantity":1}]}`)

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
		{"empty order", domain.ErrEmptyOrder, 400, "INVALID_REQUEST", invalidRequestMessage},
		{"bad quantity", domain.ErrInvalidQuantity, 400, "INVALID_REQUEST", invalidRequestMessage},
		{"unknown product", domain.ErrUnknownProduct, 400, "INVALID_REQUEST", invalidRequestMessage},
		{"bad member card", domain.ErrInvalidMemberCard, 400, "INVALID_MEMBER_CARD", "Member card not found"},
		{"slot held", &domain.SlotHeldError{ProductID: "red", ProductName: "Red set"}, 409, "LIMITED_PRODUCT_UNAVAILABLE", "Red set"},
		{"unexpected failure", errors.New("redis: connection refused"), 500, "INTERNAL", "Something went wrong"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, &fakeCalculator{err: tt.err}, fakeLister{})

			resp, body := e.do(t, "POST", "/api/calculate", `{"items":[{"productId":"red","quantity":1}]}`)

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

			resp, out := e.do(t, "POST", "/api/calculate", body)

			if resp.StatusCode != 400 || !strings.Contains(out, "INVALID_REQUEST") {
				t.Fatalf("status = %d body = %s, want 400 INVALID_REQUEST", resp.StatusCode, out)
			}
		})
	}
}

func TestUnknownRouteIsJSON404(t *testing.T) {
	e := newEnv(t, &fakeCalculator{}, fakeLister{})

	resp, body := e.do(t, "GET", "/api/nope", "")

	if resp.StatusCode != 404 || !strings.Contains(body, "NOT_FOUND") {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
}

func TestGetProducts_ListerFailureIs500(t *testing.T) {
	e := newEnv(t, &fakeCalculator{}, fakeLister{err: errors.New("db down")})

	resp, body := e.do(t, "GET", "/api/products", "")

	if resp.StatusCode != 500 || strings.Contains(body, "db down") {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
}

func TestWrongMethodIsJSON405NotA500(t *testing.T) {
	e := newEnv(t, &fakeCalculator{}, fakeLister{})

	resp, body := e.do(t, "GET", "/api/calculate", "")

	if resp.StatusCode != 405 || !strings.Contains(body, "METHOD_NOT_ALLOWED") {
		t.Fatalf("status = %d body = %s, want 405 METHOD_NOT_ALLOWED", resp.StatusCode, body)
	}
}

func TestMapError_KeepsFrameworkClientErrorStatuses(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"not found", fiber.ErrNotFound, 404, "NOT_FOUND"},
		{"method not allowed", fiber.ErrMethodNotAllowed, 405, "METHOD_NOT_ALLOWED"},
		{"payload too large", fiber.ErrRequestEntityTooLarge, 413, "PAYLOAD_TOO_LARGE"},
		{"unsupported media type", fiber.ErrUnsupportedMediaType, 415, "UNSUPPORTED_MEDIA_TYPE"},
		{"other client error", fiber.ErrUnprocessableEntity, 422, "BAD_REQUEST"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, detail := mapError(tt.err)
			if status != tt.wantStatus || detail.Code != tt.wantCode {
				t.Fatalf("mapError = %d %q, want %d %q", status, detail.Code, tt.wantStatus, tt.wantCode)
			}
		})
	}
}

func TestNoCustomerCookieIsIssued(t *testing.T) {
	e := newEnv(t, &fakeCalculator{}, fakeLister{})

	resp, _ := e.do(t, "GET", "/api/products", "")

	if got := resp.Header.Get("Set-Cookie"); got != "" {
		t.Fatalf("the API identifies no customers, but set a cookie: %q", got)
	}
}

type fakePromotions struct{ items []domain.Promotion }

func (f fakePromotions) List(context.Context) ([]domain.Promotion, error) { return f.items, nil }

type fakeReset struct{ called bool }

func (f *fakeReset) Execute(context.Context) error { f.called = true; return nil }

func TestGetPromotions_ReturnsRuleParameters(t *testing.T) {
	app := NewApp(Deps{Promotions: fakePromotions{items: []domain.Promotion{
		{Code: "PAIR_5", Kind: domain.PromotionBundle, Percent: 5, BundleSize: 2, ProductIDs: []domain.ProductID{"yellow"}},
		{Code: "MEMBER_10", Kind: domain.PromotionMember, Percent: 10},
	}}})
	resp, err := app.Test(httptest.NewRequest("GET", "/api/promotions", nil))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)

	want := `[{"code":"PAIR_5","type":"BUNDLE_PERCENT","percent":5,"bundleSize":2,"productIds":["yellow"]},{"code":"MEMBER_10","type":"MEMBER_PERCENT","percent":10}]`
	if resp.StatusCode != 200 || string(raw) != want {
		t.Fatalf("status=%d body=%s", resp.StatusCode, raw)
	}
}

func TestResetLimitsRoute_ExistsOnlyWhenEnabled(t *testing.T) {
	off := NewApp(Deps{})
	resp, _ := off.Test(httptest.NewRequest("POST", "/api/test/reset-limits", nil))
	if resp.StatusCode != 404 {
		t.Fatalf("disabled status = %d, want 404", resp.StatusCode)
	}

	reset := &fakeReset{}
	on := NewApp(Deps{ResetLimits: reset})
	resp, _ = on.Test(httptest.NewRequest("POST", "/api/test/reset-limits", nil))
	if resp.StatusCode != 204 || !reset.called {
		t.Fatalf("enabled status = %d called = %v", resp.StatusCode, reset.called)
	}
}

func TestPostCalculate_ListsEachOrderedProductWithUnitPriceAndLineTotal(t *testing.T) {
	calc := &fakeCalculator{quote: domain.Quote{
		TotalBefore: 27000,
		Lines: []domain.Cart{
			{Product: domain.Product{ID: "orange", Name: "Orange set", Price: 12000}, Quantity: 2},
			{Product: domain.Product{ID: "blue", Name: "Blue set", Price: 3000}, Quantity: 1},
		},
		FinalTotal: 27000,
	}}
	e := newEnv(t, calc, fakeLister{})

	resp, body := e.do(t, "POST", "/api/calculate", `{"items":[{"productId":"orange","quantity":2},{"productId":"blue","quantity":1}]}`)

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d body=%s", resp.StatusCode, body)
	}
	var got struct {
		Items []struct {
			ProductID       string `json:"productId"`
			Name            string `json:"name"`
			Quantity        int    `json:"quantity"`
			UnitPriceSatang int    `json:"unitPriceSatang"`
			LineTotalSatang int    `json:"lineTotalSatang"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 2 {
		t.Fatalf("items = %+v, want 2", got.Items)
	}
	first, second := got.Items[0], got.Items[1]
	if first.ProductID != "orange" || first.Name != "Orange set" || first.Quantity != 2 ||
		first.UnitPriceSatang != 12000 || first.LineTotalSatang != 24000 {
		t.Fatalf("first item = %+v", first)
	}
	if second.ProductID != "blue" || second.Quantity != 1 || second.LineTotalSatang != 3000 {
		t.Fatalf("second item = %+v", second)
	}
}

type fakeImages struct {
	got    string
	result usecase.ImageResult
	err    error
}

func (f *fakeImages) Execute(_ context.Context, productID string) (usecase.ImageResult, error) {
	f.got = productID
	return f.result, f.err
}

func newEnvWithImages(t *testing.T, images ImageGetter) env {
	t.Helper()
	calc := &fakeCalculator{}
	return env{app: NewApp(Deps{Products: fakeLister{}, Calculator: calc, ProductImages: images}), calc: calc}
}

func TestGetProductImage_ServesARealImageWithSafeHeadersAndCaching(t *testing.T) {
	images := &fakeImages{result: usecase.ImageResult{
		Image:  domain.Image{Data: []byte("png-bytes"), ContentType: "image/png"},
		Source: usecase.ImageFromStore,
	}}
	e := newEnvWithImages(t, images)

	resp, body := e.do(t, "GET", "/api/products/red/image", "")

	if resp.StatusCode != 200 || body != "png-bytes" {
		t.Fatalf("status = %d body = %q", resp.StatusCode, body)
	}
	if images.got != "red" {
		t.Fatalf("asked for product %q, want red", images.got)
	}
	if got := resp.Header.Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := resp.Header.Get("Content-Security-Policy"); !strings.Contains(got, "sandbox") {
		t.Fatalf("Content-Security-Policy = %q, want a sandbox policy", got)
	}
	if got := resp.Header.Get("Cache-Control"); !strings.Contains(got, "max-age=300") {
		t.Fatalf("a real image may be cached briefly, Cache-Control = %q", got)
	}
}

func TestGetProductImage_NeverCachesTheStandInPicture(t *testing.T) {
	images := &fakeImages{result: usecase.ImageResult{
		Image:  domain.Image{Data: []byte("<svg/>"), ContentType: "image/svg+xml"},
		Source: usecase.ImageFallback,
	}}
	e := newEnvWithImages(t, images)

	resp, _ := e.do(t, "GET", "/api/products/red/image", "")

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, the default picture is a normal 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q: a stand-in must never be cached, or the real image could not appear once MinIO recovers", got)
	}
}

func TestGetProductImage_UnknownProductIsA404(t *testing.T) {
	e := newEnvWithImages(t, &fakeImages{err: domain.ErrUnknownProduct})

	resp, body := e.do(t, "GET", "/api/products/black/image", "")

	if resp.StatusCode != 404 || !strings.Contains(body, "NOT_FOUND") {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
}

func TestGetProductImage_UnexpectedFailureIsAGeneric500(t *testing.T) {
	e := newEnvWithImages(t, &fakeImages{err: errors.New("db down")})

	resp, body := e.do(t, "GET", "/api/products/red/image", "")

	if resp.StatusCode != 500 || strings.Contains(body, "db down") {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
}

func TestGetProducts_EachProductPointsAtItsImageEndpoint(t *testing.T) {
	e := newEnv(t, &fakeCalculator{}, fakeLister{items: []domain.Product{{ID: "red", Name: "Red set", Price: 5000, ImageKey: "red.png"}, {ID: "blue", Name: "Blue set", Price: 3000}}})

	_, body := e.do(t, "GET", "/api/products", "")

	// Every product has an imageUrl, with or without an uploaded picture:
	// the endpoint serves the default picture when there is none.
	if !strings.Contains(body, `"imageUrl":"/api/products/red/image"`) || !strings.Contains(body, `"imageUrl":"/api/products/blue/image"`) {
		t.Fatalf("body = %s", body)
	}
	if strings.Contains(body, "red.png") {
		t.Fatalf("the storage key is an internal detail and must not leak: %s", body)
	}
}

func TestInvalidRequest_MessageIsFixedAndNeverEchoesTheDetail(t *testing.T) {
	err := fmt.Errorf("%w: %q has quantity 5000", domain.ErrInvalidQuantity, "secret-product-id")
	e := newEnv(t, &fakeCalculator{err: err}, fakeLister{})

	resp, body := e.do(t, "POST", "/api/calculate", `{"items":[{"productId":"red","quantity":1}]}`)

	if resp.StatusCode != 400 || !strings.Contains(body, invalidRequestMessage) {
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	if strings.Contains(body, "secret-product-id") || strings.Contains(body, "5000") {
		t.Fatalf("the response must not echo request detail: %s", body)
	}
}

func TestPostCalculate_RequiresAJSONContentType(t *testing.T) {
	for name, contentType := range map[string]string{"missing": "", "form": "application/x-www-form-urlencoded", "text": "text/plain"} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, &fakeCalculator{}, fakeLister{})
			req := httptest.NewRequest("POST", "/api/calculate", strings.NewReader(`{"items":[{"productId":"red","quantity":1}]}`))
			if contentType != "" {
				req.Header.Set("Content-Type", contentType)
			}
			resp, err := e.app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != 415 || !strings.Contains(string(raw), "UNSUPPORTED_MEDIA_TYPE") {
				t.Fatalf("status = %d body = %s, want 415", resp.StatusCode, raw)
			}
		})
	}
}

func TestPostCalculate_AcceptsJSONWithACharset(t *testing.T) {
	e := newEnv(t, &fakeCalculator{}, fakeLister{})
	req := httptest.NewRequest("POST", "/api/calculate", strings.NewReader(`{"items":[{"productId":"red","quantity":1}]}`))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestPostCalculate_PassesTheClientKeyHeader(t *testing.T) {
	calc := &fakeCalculator{}
	e := newEnv(t, calc, fakeLister{})
	req := httptest.NewRequest("POST", "/api/calculate", strings.NewReader(`{"items":[{"productId":"red","quantity":1}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Client-Key", "browser-1")
	if _, err := e.app.Test(req); err != nil {
		t.Fatal(err)
	}
	if calc.got.ClientKey != "browser-1" {
		t.Fatalf("ClientKey = %q, want browser-1", calc.got.ClientKey)
	}
}

func TestPostCalculate_ClientKeyIsOptionalAndBounded(t *testing.T) {
	e := newEnv(t, &fakeCalculator{}, fakeLister{})
	if _, body := e.do(t, "POST", "/api/calculate", `{"items":[{"productId":"red","quantity":1}]}`); strings.Contains(body, "error") {
		t.Fatalf("a missing key must be fine: %s", body)
	}

	req := httptest.NewRequest("POST", "/api/calculate", strings.NewReader(`{"items":[{"productId":"red","quantity":1}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Client-Key", strings.Repeat("k", maxClientKeyLength+1))
	resp, err := e.app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 400 {
		t.Fatalf("status = %d, want 400 for an oversized key", resp.StatusCode)
	}
}

type fakeReady struct{ err error }

func (f fakeReady) Check(context.Context) error { return f.err }

func TestReadyz_ReportsDependencyHealthWithoutLeakingDetail(t *testing.T) {
	ok := NewApp(Deps{Ready: fakeReady{}})
	resp, err := ok.Test(httptest.NewRequest("GET", "/readyz", nil))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("ready: err=%v status=%v", err, resp)
	}

	down := NewApp(Deps{Ready: fakeReady{err: errors.New("dial tcp 10.0.0.5: refused")}})
	resp, err = down.Test(httptest.NewRequest("GET", "/readyz", nil))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 503 || strings.Contains(string(raw), "10.0.0.5") {
		t.Fatalf("status = %d body = %s, want 503 with no detail", resp.StatusCode, raw)
	}
}

func TestHealthz_StaysLivenessOnlyWhenDependenciesAreDown(t *testing.T) {
	app := NewApp(Deps{Ready: fakeReady{err: errors.New("down")}})
	resp, err := app.Test(httptest.NewRequest("GET", "/healthz", nil))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("healthz must not depend on Postgres or Redis: err=%v", err)
	}
}
