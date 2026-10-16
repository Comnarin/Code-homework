# Food Store Calculator Implementation Plan

> **Note (revision after execution):** this plan is kept as written. Two things changed during execution: Postgres access uses GORM instead of raw pgx, and customer identity (the `cid` cookie, `customer_id` column and the Lua "holder may re-claim" claim) was removed in favor of a plain `SET NX` "Red ordered in the last hour" rule. See the spec's revision note and the git history for the current design.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a food store price calculator: Go Fiber API (Clean Architecture, Postgres, Redis) and a Next.js UI, built test-first, runnable with `docker compose up`.

**Architecture:** Backend has four layers (`domain` -> `usecase` -> `adapter` -> `platform`) with dependencies pointing inward only. Pricing is a pure domain function; discount rules and Red-set limits are rows in Postgres mapped to domain rules; the one-customer-per-hour slot is an atomic Redis Lua claim. Frontend is a Next.js 16 App Router app: a Server Component loads products, one Client Component island handles the calculator.

**Tech Stack:** Go 1.26, Fiber v3.6.1, pgx v5, golang-migrate, go-redis v9, miniredis (tests), Postgres 17, Redis 7, Next.js 16, React 19, TypeScript strict, Tailwind v4, Zod, Vitest, React Testing Library, Docker Compose.

**Spec:** `docs/superpowers/specs/2026-10-16-food-store-calculator-design.md`

## Global Constraints

- Money is integer satang (1/100 THB), type `domain.Money`; never floats. Discounts use basis points (500 = 5%) and round half-up per discount.
- Dependency rule: `adapter -> usecase -> domain`. `domain` imports only the standard library. `usecase` never imports Fiber, pgx or go-redis.
- Ports (interfaces) live in `usecase/ports.go`, except the two HTTP-consumer interfaces which live in `adapter/http`.
- TDD for every task: write one failing test, watch it fail for the right reason, write minimal code, run the whole package, commit. No production code without a failing test first.
- Go module path: `github.com/comnarin/foodstore` (in `backend/`). Run Go commands from `backend/`.
- Tests: table-driven, named by behavior, `go test -race`. Frontend tests query by role and label.
- Seed data mirrors the PDF: Red 5000, Green 4000, Blue 3000, Yellow 5000, Pink 8000, Purple 9000, Orange 12000 satang; rule `PAIR_5` (BUNDLE_PERCENT, 500 bp, size 2, priority 10) on orange/pink/green; rule `MEMBER_10` (MEMBER_PERCENT, 1000 bp, priority 100); Red limit 3600 s; demo members `MEM0001` ACTIVE, `MEM0002` SUSPENDED.
- A pair is two sets of the same colour, not a mix.
- Member discount applies after item discounts. Unknown or SUSPENDED card -> 400 `INVALID_MEMBER_CARD`.
- Redis key: `limit:{productID}:holder`. The slot holder may re-claim; the TTL is never extended.
- `MaxQuantityPerLine = 1000`.
- Error JSON shape everywhere: `{"error":{"code":"...","message":"..."}}`. Codes: `INVALID_REQUEST`, `INVALID_MEMBER_CARD`, `LIMITED_PRODUCT_UNAVAILABLE`, `NOT_FOUND`, `INTERNAL`.
- Ports: web 3000, api 8080, Postgres 55432 (host side, for tests), Redis internal only.
- Every commit message ends with the trailer line `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>` (passed as a second `-m`).

## Review Focus

Inputs and failures the spec implies but does not spell out. Each has a test in the task named in brackets.

1. The same product appears twice in one request (Orange 1 + Orange 1) -> merged into one line, so the pair discount applies. [Task 7]
2. Quantity 0, negative, or above 1000 -> 400 `INVALID_REQUEST`; it must never reach Redis or overflow. [Task 7]
3. Empty or malformed JSON body on `POST /api/calculate` -> 400 `INVALID_REQUEST`, not 500. [Task 11]
4. Redis down or limiter error mid-request -> 500 `INTERNAL`, no order saved, no partial success message. [Task 7, Task 11]
5. Browser cannot reach the API (network failure) -> the UI shows a retryable message and keeps the cart. [Task 14, Task 16]

---

## File Structure

```
backend/
  go.mod  go.sum  Dockerfile  .dockerignore
  cmd/server/main.go                          composition root
  internal/domain/
    money.go  product.go  errors.go  order.go  pricing.go  rules.go
    fixtures_test.go  money_test.go  product_test.go  errors_test.go
    pricing_test.go  rules_test.go  pdf_examples_test.go
  internal/usecase/
    ports.go  list_products.go  calculate_order.go
    fakes_test.go  list_products_test.go  calculate_order_test.go
  internal/adapter/
    redis/slot_limiter.go  slot_limiter_test.go
    postgres/products.go members.go rules.go limits.go orders.go  helper_test.go *_test.go
    http/router.go handlers.go dto.go errors.go cid.go  *_test.go
  internal/platform/
    config/config.go  config_test.go
    db/db.go
  migrations/migrations.go  000001_init.{up,down}.sql  000002_seed.{up,down}.sql
frontend/                                      (created by create-next-app)
  Dockerfile  next.config.ts  vitest.config.ts  vitest.setup.ts
  src/app/page.tsx  error.tsx  layout.tsx
  src/lib/money.ts  env.ts  money.test.ts
  src/features/calculator/
    api.ts  api.test.ts  state.ts  state.test.ts  useCalculator.ts
    components/ProductRow.tsx  ResultSummary.tsx  Calculator.tsx  *.test.tsx
docker-compose.yml  README.md  .gitignore
docs/INTERVIEW.md
```

---

## Task 1: Backend scaffold and `Money`

**Files:**
- Create: `.gitignore`, `backend/go.mod`, `backend/internal/domain/money.go`
- Test: `backend/internal/domain/money_test.go`

**Interfaces:**
- Produces: `type Money int64`; `func (m Money) ApplyBP(bp int) Money` (m x bp / 10000, rounded half-up).

- [ ] **Step 1: Scaffold the module and ignore file**

```bash
cd /Users/comnarin/Documents/Stelligence/Code-homework
cat > .gitignore <<'EOF'
node_modules/
.next/
coverage/
.env
.DS_Store
backend/bin/
EOF
mkdir -p backend/internal/domain && cd backend
go mod init github.com/comnarin/foodstore
go mod edit -go=1.26.0
```

- [ ] **Step 2: Write the failing test**

`backend/internal/domain/money_test.go`:
```go
package domain

import "testing"

func TestMoney_ApplyBP(t *testing.T) {
	tests := []struct {
		name string
		m    Money
		bp   int
		want Money
	}{
		{"5 percent of 240 THB", 24000, 500, 1200},
		{"10 percent of 228 THB", 22800, 1000, 2280},
		{"exactly half a satang rounds up", 10, 500, 1},
		{"below half a satang rounds down", 9, 500, 0},
		{"zero amount", 0, 1000, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.ApplyBP(tt.bp); got != tt.want {
				t.Fatalf("Money(%d).ApplyBP(%d) = %d, want %d", tt.m, tt.bp, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 3: Run it and watch it fail**

Run: `cd backend && go test ./internal/domain/ -run TestMoney -v`
Expected: FAIL, `undefined: Money`.

- [ ] **Step 4: Minimal implementation**

`backend/internal/domain/money.go`:
```go
package domain

// Money is an amount in satang (1/100 THB). Integers avoid float rounding errors.
type Money int64

// ApplyBP returns m x bp / 10000 rounded half-up. bp is basis points: 500 = 5%.
func (m Money) ApplyBP(bp int) Money {
	return Money((int64(m)*int64(bp) + 5000) / 10000)
}
```

- [ ] **Step 5: Run it and watch it pass**

Run: `cd backend && go test ./internal/domain/ -v`
Expected: PASS (5 subtests).

- [ ] **Step 6: Commit**

```bash
cd /Users/comnarin/Documents/Stelligence/Code-homework
git add .gitignore backend
git commit -m "feat(domain): add Money with half-up basis-point rounding" -m "Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>"
```

---

## Task 2: Domain entities and errors

**Files:**
- Create: `backend/internal/domain/product.go`, `errors.go`, `order.go`
- Test: `backend/internal/domain/product_test.go`, `errors_test.go`

**Interfaces:**
- Produces:
  - `type ProductID string`
  - `type Product struct{ ID ProductID; Name string; Price Money }`
  - `type Member struct{ CardNumber, FullName string; Active bool }`
  - `func NormalizeCard(s string) string` (trim + uppercase)
  - `type Line struct{ Product Product; Quantity int }`
  - `type Order struct{ ID, CustomerID, MemberCard string; Lines []Line; Quote Quote; CreatedAt time.Time }` (Quote is defined in Task 3; this task defines Order after Quote exists, see Step 5)
  - errors: `ErrEmptyOrder, ErrInvalidQuantity, ErrUnknownProduct, ErrInvalidMemberCard, ErrSlotHeld, ErrInvalidRule`
  - `type SlotHeldError struct{ ProductID ProductID; ProductName string }` with `Error()` and `Unwrap() error` returning `ErrSlotHeld`

- [ ] **Step 1: Write the failing tests**

`backend/internal/domain/product_test.go`:
```go
package domain

import "testing"

func TestNormalizeCard(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"trims spaces", "  MEM0001  ", "MEM0001"},
		{"uppercases", "mem0001", "MEM0001"},
		{"blank stays blank", "   ", ""},
		{"empty stays empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeCard(tt.in); got != tt.want {
				t.Fatalf("NormalizeCard(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
```

`backend/internal/domain/errors_test.go`:
```go
package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestSlotHeldError_MatchesErrSlotHeldAndNamesProduct(t *testing.T) {
	err := error(&SlotHeldError{ProductID: "red", ProductName: "Red set"})

	if !errors.Is(err, ErrSlotHeld) {
		t.Fatal("SlotHeldError should match ErrSlotHeld via errors.Is")
	}
	var held *SlotHeldError
	if !errors.As(err, &held) || held.ProductName != "Red set" {
		t.Fatalf("errors.As failed or wrong name: %+v", held)
	}
	if !strings.Contains(err.Error(), "Red set") {
		t.Fatalf("message %q should name the product", err.Error())
	}
}
```

- [ ] **Step 2: Run and watch them fail**

Run: `cd backend && go test ./internal/domain/ -v`
Expected: FAIL, `undefined: NormalizeCard`, `undefined: SlotHeldError`.

- [ ] **Step 3: Minimal implementation**

`backend/internal/domain/product.go`:
```go
package domain

import "strings"

type ProductID string

type Product struct {
	ID    ProductID
	Name  string
	Price Money
}

type Member struct {
	CardNumber string
	FullName   string
	Active     bool
}

// NormalizeCard puts a member card number into its stored form.
func NormalizeCard(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// Line is one product and how many sets of it were ordered.
type Line struct {
	Product  Product
	Quantity int
}
```

`backend/internal/domain/errors.go`:
```go
package domain

import (
	"errors"
	"fmt"
)

var (
	ErrEmptyOrder        = errors.New("order has no items")
	ErrInvalidQuantity   = errors.New("quantity must be a positive integer within the allowed limit")
	ErrUnknownProduct    = errors.New("unknown product")
	ErrInvalidMemberCard = errors.New("member card is not valid")
	ErrSlotHeld          = errors.New("limited product is held by another customer")
	ErrInvalidRule       = errors.New("invalid discount rule")
)

// SlotHeldError says which limited product another customer is holding.
type SlotHeldError struct {
	ProductID   ProductID
	ProductName string
}

func (e *SlotHeldError) Error() string {
	return fmt.Sprintf("%s is held by another customer", e.ProductName)
}

func (e *SlotHeldError) Unwrap() error { return ErrSlotHeld }
```

- [ ] **Step 4: Run and watch them pass**

Run: `cd backend && go test ./internal/domain/ -v`
Expected: PASS.

- [ ] **Step 5: Add `Order` (needs `Quote`, so it is added in Task 3 Step 4)**

No action now; Task 3 creates `order.go` together with `Quote`.

- [ ] **Step 6: Commit**

```bash
cd /Users/comnarin/Documents/Stelligence/Code-homework
git add backend
git commit -m "feat(domain): add Product, Member, Line and domain errors" -m "Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>"
```

---

## Task 3: Pricing calculator (`Calculate`, `Rule`, `Quote`, `Order`)

**Files:**
- Create: `backend/internal/domain/pricing.go`, `order.go`, `fixtures_test.go`
- Test: `backend/internal/domain/pricing_test.go`

**Interfaces:**
- Consumes: `Money`, `Line`, `Product`, `ProductID`, `ErrEmptyOrder`, `ErrInvalidQuantity`.
- Produces:
  - `type Scope int` with `ScopeItem`, `ScopeMember`
  - `type Discount struct{ RuleCode string; Scope Scope; ProductID ProductID; Name string; Bundles int; Amount Money }`
  - `type Rule interface{ Code() string; Priority() int; Apply(lines []Line, subtotal Money, isMember bool) []Discount }`
  - `type Quote struct{ TotalBefore Money; Discounts []Discount; FinalTotal Money }`, `func (Quote) ItemDiscounts() []Discount`, `func (Quote) MemberDiscount() Money`
  - `func Calculate(lines []Line, isMember bool, rules []Rule) (Quote, error)`
  - `type Order struct{ ID, CustomerID, MemberCard string; Lines []Line; Quote Quote; CreatedAt time.Time }`

- [ ] **Step 1: Write fixtures and the failing tests**

`backend/internal/domain/fixtures_test.go`:
```go
package domain

var (
	red    = Product{ID: "red", Name: "Red set", Price: 5000}
	green  = Product{ID: "green", Name: "Green set", Price: 4000}
	blue   = Product{ID: "blue", Name: "Blue set", Price: 3000}
	yellow = Product{ID: "yellow", Name: "Yellow set", Price: 5000}
	pink   = Product{ID: "pink", Name: "Pink set", Price: 8000}
	purple = Product{ID: "purple", Name: "Purple set", Price: 9000}
	orange = Product{ID: "orange", Name: "Orange set", Price: 12000}
)
```

`backend/internal/domain/pricing_test.go`:
```go
package domain

import (
	"errors"
	"testing"
)

// stubRule is a scripted rule that records the subtotal it was given.
type stubRule struct {
	code        string
	priority    int
	scope       Scope
	amount      Money
	gotSubtotal *Money
}

func (s stubRule) Code() string  { return s.code }
func (s stubRule) Priority() int { return s.priority }
func (s stubRule) Apply(_ []Line, subtotal Money, _ bool) []Discount {
	if s.gotSubtotal != nil {
		*s.gotSubtotal = subtotal
	}
	return []Discount{{RuleCode: s.code, Scope: s.scope, Amount: s.amount}}
}

func TestCalculate_NoRules_TotalsTheLines(t *testing.T) {
	q, err := Calculate([]Line{{blue, 2}, {yellow, 1}}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if q.TotalBefore != 11000 || q.FinalTotal != 11000 || len(q.Discounts) != 0 {
		t.Fatalf("got %+v", q)
	}
}

func TestCalculate_RejectsBadOrders(t *testing.T) {
	tests := []struct {
		name  string
		lines []Line
		want  error
	}{
		{"empty order", nil, ErrEmptyOrder},
		{"zero quantity", []Line{{blue, 0}}, ErrInvalidQuantity},
		{"negative quantity", []Line{{blue, -1}}, ErrInvalidQuantity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Calculate(tt.lines, false, nil); !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCalculate_RunsRulesInPriorityOrderOnTheRunningSubtotal(t *testing.T) {
	var seenByLate Money
	early := stubRule{code: "EARLY", priority: 10, scope: ScopeItem, amount: 1000}
	late := stubRule{code: "LATE", priority: 100, scope: ScopeMember, amount: 500, gotSubtotal: &seenByLate}

	// Passed in the wrong order on purpose.
	q, err := Calculate([]Line{{blue, 10}}, true, []Rule{late, early})
	if err != nil {
		t.Fatal(err)
	}
	if seenByLate != 29000 { // 30000 - 1000 from the earlier rule
		t.Fatalf("late rule saw subtotal %d, want 29000", seenByLate)
	}
	if q.FinalTotal != 28500 {
		t.Fatalf("FinalTotal = %d, want 28500", q.FinalTotal)
	}
	if q.Discounts[0].RuleCode != "EARLY" || q.Discounts[1].RuleCode != "LATE" {
		t.Fatalf("discount order wrong: %+v", q.Discounts)
	}
}

func TestCalculate_DoesNotReorderTheCallersSlice(t *testing.T) {
	rules := []Rule{
		stubRule{code: "B", priority: 20, amount: 1},
		stubRule{code: "A", priority: 10, amount: 1},
	}
	if _, err := Calculate([]Line{{blue, 1}}, false, rules); err != nil {
		t.Fatal(err)
	}
	if rules[0].Code() != "B" {
		t.Fatal("Calculate must not mutate the rules slice it was given")
	}
}

func TestQuote_SplitsItemAndMemberDiscounts(t *testing.T) {
	q := Quote{Discounts: []Discount{
		{RuleCode: "PAIR_5", Scope: ScopeItem, ProductID: "pink", Amount: 1600},
		{RuleCode: "MEMBER_10", Scope: ScopeMember, Amount: 2000},
	}}
	if got := q.ItemDiscounts(); len(got) != 1 || got[0].RuleCode != "PAIR_5" {
		t.Fatalf("ItemDiscounts = %+v", got)
	}
	if got := q.MemberDiscount(); got != 2000 {
		t.Fatalf("MemberDiscount = %d, want 2000", got)
	}
}
```

- [ ] **Step 2: Run and watch it fail**

Run: `cd backend && go test ./internal/domain/ -v`
Expected: FAIL, `undefined: Calculate`, `undefined: Scope`.

- [ ] **Step 3: Minimal implementation**

`backend/internal/domain/pricing.go`:
```go
package domain

import "sort"

// Scope says what a discount applies to.
type Scope int

const (
	ScopeItem   Scope = iota // a specific product line
	ScopeMember              // the whole order, because the customer is a member
)

// Discount is one amount taken off an order by one rule.
type Discount struct {
	RuleCode  string
	Scope     Scope
	ProductID ProductID // empty for ScopeMember
	Name      string
	Bundles   int // how many groups (for example pairs) were discounted
	Amount    Money
}

// Rule is a pricing condition. Rules never mutate their inputs.
type Rule interface {
	Code() string
	Priority() int // lower runs first
	// Apply returns the discounts this rule grants. subtotal is the running
	// total after earlier rules.
	Apply(lines []Line, subtotal Money, isMember bool) []Discount
}

// Quote is the priced result of an order.
type Quote struct {
	TotalBefore Money
	Discounts   []Discount
	FinalTotal  Money
}

func (q Quote) ItemDiscounts() []Discount {
	var out []Discount
	for _, d := range q.Discounts {
		if d.Scope == ScopeItem {
			out = append(out, d)
		}
	}
	return out
}

func (q Quote) MemberDiscount() Money {
	var sum Money
	for _, d := range q.Discounts {
		if d.Scope == ScopeMember {
			sum += d.Amount
		}
	}
	return sum
}

// Calculate prices lines with rules. It is pure: same input, same output.
// Lines must have distinct products; the caller merges duplicates.
func Calculate(lines []Line, isMember bool, rules []Rule) (Quote, error) {
	if len(lines) == 0 {
		return Quote{}, ErrEmptyOrder
	}
	var total Money
	for _, l := range lines {
		if l.Quantity <= 0 {
			return Quote{}, ErrInvalidQuantity
		}
		total += l.Product.Price * Money(l.Quantity)
	}

	ordered := append([]Rule(nil), rules...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Priority() < ordered[j].Priority() })

	q := Quote{TotalBefore: total, FinalTotal: total}
	for _, r := range ordered {
		for _, d := range r.Apply(lines, q.FinalTotal, isMember) {
			q.Discounts = append(q.Discounts, d)
			q.FinalTotal -= d.Amount
		}
	}
	return q, nil
}
```

`backend/internal/domain/order.go`:
```go
package domain

import "time"

// Order is a priced order ready to be stored. MemberCard is empty when the
// customer is not a member.
type Order struct {
	ID         string
	CustomerID string
	MemberCard string
	Lines      []Line
	Quote      Quote
	CreatedAt  time.Time
}
```

- [ ] **Step 4: Run and watch it pass**

Run: `cd backend && go test -race ./internal/domain/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/comnarin/Documents/Stelligence/Code-homework
git add backend
git commit -m "feat(domain): add pure Calculate with priority-ordered rules" -m "Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>"
```

---

## Task 4: `BundlePercent` and `MemberPercent` rules, plus the PDF examples

**Files:**
- Create: `backend/internal/domain/rules.go`
- Test: `backend/internal/domain/rules_test.go`, `backend/internal/domain/pdf_examples_test.go`

**Interfaces:**
- Consumes: `Rule`, `Discount`, `Line`, `Money`, `ErrInvalidRule`, `Calculate`.
- Produces:
  - `func NewBundlePercent(code string, priority int, products []ProductID, size, percentBP int) (BundlePercent, error)`
  - `func NewMemberPercent(code string, priority, percentBP int) (MemberPercent, error)`
  - both implement `Rule` on value receivers.

- [ ] **Step 1: Write the failing rule tests**

`backend/internal/domain/rules_test.go`:
```go
package domain

import (
	"errors"
	"testing"
)

func pairRule(t *testing.T) BundlePercent {
	t.Helper()
	r, err := NewBundlePercent("PAIR_5", 10, []ProductID{"orange", "pink", "green"}, 2, 500)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestBundlePercent_Apply(t *testing.T) {
	tests := []struct {
		name        string
		lines       []Line
		wantBundles int
		wantAmount  Money
		wantNone    bool
	}{
		{"one orange is not a pair", []Line{{orange, 1}}, 0, 0, true},
		{"orange x2 is one pair", []Line{{orange, 2}}, 1, 1200, false},
		{"pink x4 is two pairs", []Line{{pink, 4}}, 2, 1600, false},
		{"green x3 is one pair plus a full-price set", []Line{{green, 3}}, 1, 400, false},
		{"ineligible product gets nothing", []Line{{blue, 6}}, 0, 0, true},
		{"red gets nothing", []Line{{red, 2}}, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pairRule(t).Apply(tt.lines, 0, false)
			if tt.wantNone {
				if len(got) != 0 {
					t.Fatalf("want no discounts, got %+v", got)
				}
				return
			}
			if len(got) != 1 || got[0].Bundles != tt.wantBundles || got[0].Amount != tt.wantAmount {
				t.Fatalf("got %+v, want bundles=%d amount=%d", got, tt.wantBundles, tt.wantAmount)
			}
			if got[0].Scope != ScopeItem || got[0].RuleCode != "PAIR_5" {
				t.Fatalf("scope or code wrong: %+v", got[0])
			}
		})
	}
}

func TestBundlePercent_MixedColoursDoNotPairWithEachOther(t *testing.T) {
	got := pairRule(t).Apply([]Line{{orange, 1}, {pink, 1}}, 0, false)
	if len(got) != 0 {
		t.Fatalf("one orange plus one pink must not form a pair, got %+v", got)
	}
}

func TestBundlePercent_DiscountsEachEligibleLineSeparately(t *testing.T) {
	got := pairRule(t).Apply([]Line{{orange, 2}, {pink, 2}}, 0, false)
	if len(got) != 2 || got[0].ProductID != "orange" || got[1].ProductID != "pink" {
		t.Fatalf("got %+v", got)
	}
	if got[0].Name != "Orange set" {
		t.Fatalf("discount should carry the product name, got %q", got[0].Name)
	}
}

func TestNewBundlePercent_RejectsInvalidConfig(t *testing.T) {
	ok := []ProductID{"orange"}
	tests := []struct {
		name     string
		code     string
		products []ProductID
		size     int
		bp       int
	}{
		{"empty code", "", ok, 2, 500},
		{"no products", "X", nil, 2, 500},
		{"size below 2", "X", ok, 1, 500},
		{"percent zero", "X", ok, 2, 0},
		{"percent above 100", "X", ok, 2, 10001},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewBundlePercent(tt.code, 10, tt.products, tt.size, tt.bp)
			if !errors.Is(err, ErrInvalidRule) {
				t.Fatalf("err = %v, want ErrInvalidRule", err)
			}
		})
	}
}

func TestMemberPercent_Apply(t *testing.T) {
	r, err := NewMemberPercent("MEMBER_10", 100, 1000)
	if err != nil {
		t.Fatal(err)
	}

	if got := r.Apply(nil, 22800, false); len(got) != 0 {
		t.Fatalf("non-member should get nothing, got %+v", got)
	}
	got := r.Apply(nil, 22800, true)
	if len(got) != 1 || got[0].Amount != 2280 || got[0].Scope != ScopeMember || got[0].RuleCode != "MEMBER_10" {
		t.Fatalf("got %+v", got)
	}
	if got := r.Apply(nil, 0, true); len(got) != 0 {
		t.Fatalf("zero subtotal should yield no discount, got %+v", got)
	}
}

func TestNewMemberPercent_RejectsInvalidConfig(t *testing.T) {
	for name, tc := range map[string]struct {
		code string
		bp   int
	}{
		"empty code":    {"", 1000},
		"percent zero":  {"X", 0},
		"percent > 100": {"X", 10001},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewMemberPercent(tc.code, 100, tc.bp); !errors.Is(err, ErrInvalidRule) {
				t.Fatalf("err = %v, want ErrInvalidRule", err)
			}
		})
	}
}
```

`backend/internal/domain/pdf_examples_test.go`:
```go
package domain

import "testing"

// Every example and condition in "Code Homework Test.pdf" as an executable test.
func TestCalculate_PDFExamples(t *testing.T) {
	pair, err := NewBundlePercent("PAIR_5", 10, []ProductID{"orange", "pink", "green"}, 2, 500)
	if err != nil {
		t.Fatal(err)
	}
	member, err := NewMemberPercent("MEMBER_10", 100, 1000)
	if err != nil {
		t.Fatal(err)
	}
	rules := []Rule{pair, member}

	tests := []struct {
		name       string
		lines      []Line
		isMember   bool
		wantBefore Money
		wantMember Money
		wantFinal  Money
	}{
		{"Orange x2 = (120+120) - 5%", []Line{{orange, 2}}, false, 24000, 0, 22800},
		{"Pink x4 = two discounted pairs", []Line{{pink, 4}}, false, 32000, 0, 30400},
		{"Green x3 = one discounted pair + 40", []Line{{green, 3}}, false, 12000, 0, 11600},
		{"Blue gets no pair discount", []Line{{blue, 2}}, false, 6000, 0, 6000},
		{"member gets 10% of the total", []Line{{blue, 2}}, true, 6000, 600, 5400},
		{"member 10% applies after pair discount", []Line{{orange, 2}}, true, 24000, 2280, 20520},
		{"mixed order", []Line{{orange, 2}, {green, 3}, {blue, 1}}, false, 24000 + 12000 + 3000, 0, 39000 - 1200 - 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Calculate(tt.lines, tt.isMember, rules)
			if err != nil {
				t.Fatal(err)
			}
			if q.TotalBefore != tt.wantBefore || q.MemberDiscount() != tt.wantMember || q.FinalTotal != tt.wantFinal {
				t.Fatalf("got before=%d member=%d final=%d; want %d/%d/%d",
					q.TotalBefore, q.MemberDiscount(), q.FinalTotal, tt.wantBefore, tt.wantMember, tt.wantFinal)
			}
		})
	}
}
```

- [ ] **Step 2: Run and watch them fail**

Run: `cd backend && go test ./internal/domain/ -v`
Expected: FAIL, `undefined: NewBundlePercent`, `undefined: NewMemberPercent`.

- [ ] **Step 3: Minimal implementation**

`backend/internal/domain/rules.go`:
```go
package domain

import "fmt"

var (
	_ Rule = BundlePercent{}
	_ Rule = MemberPercent{}
)

// BundlePercent gives percentBP off every complete group of `size` sets of the
// same eligible product (for example every pair of Orange).
type BundlePercent struct {
	code      string
	priority  int
	eligible  map[ProductID]struct{}
	size      int
	percentBP int
}

func NewBundlePercent(code string, priority int, products []ProductID, size, percentBP int) (BundlePercent, error) {
	switch {
	case code == "":
		return BundlePercent{}, fmt.Errorf("%w: code is required", ErrInvalidRule)
	case len(products) == 0:
		return BundlePercent{}, fmt.Errorf("%w: %s needs at least one product", ErrInvalidRule, code)
	case size < 2:
		return BundlePercent{}, fmt.Errorf("%w: %s bundle size must be at least 2", ErrInvalidRule, code)
	case percentBP < 1 || percentBP > 10000:
		return BundlePercent{}, fmt.Errorf("%w: %s percent must be between 1 and 10000 bp", ErrInvalidRule, code)
	}
	eligible := make(map[ProductID]struct{}, len(products))
	for _, p := range products {
		eligible[p] = struct{}{}
	}
	return BundlePercent{code: code, priority: priority, eligible: eligible, size: size, percentBP: percentBP}, nil
}

func (r BundlePercent) Code() string  { return r.code }
func (r BundlePercent) Priority() int { return r.priority }

func (r BundlePercent) Apply(lines []Line, _ Money, _ bool) []Discount {
	var out []Discount
	for _, l := range lines {
		if _, ok := r.eligible[l.Product.ID]; !ok {
			continue
		}
		bundles := l.Quantity / r.size
		if bundles == 0 {
			continue
		}
		base := l.Product.Price * Money(bundles*r.size)
		out = append(out, Discount{
			RuleCode:  r.code,
			Scope:     ScopeItem,
			ProductID: l.Product.ID,
			Name:      l.Product.Name,
			Bundles:   bundles,
			Amount:    base.ApplyBP(r.percentBP),
		})
	}
	return out
}

// MemberPercent gives percentBP off the running subtotal to members.
type MemberPercent struct {
	code      string
	priority  int
	percentBP int
}

func NewMemberPercent(code string, priority, percentBP int) (MemberPercent, error) {
	switch {
	case code == "":
		return MemberPercent{}, fmt.Errorf("%w: code is required", ErrInvalidRule)
	case percentBP < 1 || percentBP > 10000:
		return MemberPercent{}, fmt.Errorf("%w: %s percent must be between 1 and 10000 bp", ErrInvalidRule, code)
	}
	return MemberPercent{code: code, priority: priority, percentBP: percentBP}, nil
}

func (r MemberPercent) Code() string  { return r.code }
func (r MemberPercent) Priority() int { return r.priority }

func (r MemberPercent) Apply(_ []Line, subtotal Money, isMember bool) []Discount {
	if !isMember {
		return nil
	}
	amount := subtotal.ApplyBP(r.percentBP)
	if amount == 0 {
		return nil
	}
	return []Discount{{RuleCode: r.code, Scope: ScopeMember, Amount: amount}}
}
```

- [ ] **Step 4: Run and watch them pass, with coverage**

Run: `cd backend && go test -race -cover ./internal/domain/`
Expected: PASS, coverage close to 100%.

- [ ] **Step 5: Commit**

```bash
cd /Users/comnarin/Documents/Stelligence/Code-homework
git add backend
git commit -m "feat(domain): add BundlePercent and MemberPercent rules with PDF examples" -m "Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>"
```

---

## Task 5: Use-case ports, fakes and `ListProducts`

**Files:**
- Create: `backend/internal/usecase/ports.go`, `list_products.go`
- Test: `backend/internal/usecase/fakes_test.go`, `list_products_test.go`

**Interfaces:**
- Consumes: `domain.*`.
- Produces (all in package `usecase`):
```go
type ProductRepository interface{ List(ctx context.Context) ([]domain.Product, error) }
type MemberRepository interface{ FindByCard(ctx context.Context, card string) (domain.Member, bool, error) }
type RuleRepository interface{ ActiveRules(ctx context.Context) ([]domain.Rule, error) }
type LimitRepository interface{ ActiveLimits(ctx context.Context) (map[domain.ProductID]time.Duration, error) }
type OrderRepository interface{ Save(ctx context.Context, o domain.Order) error }
type SlotLimiter interface{ Claim(ctx context.Context, productID domain.ProductID, ownerID string, ttl time.Duration) error }
type Clock interface{ Now() time.Time }
type IDGenerator interface{ NewID() string }
func NewListProducts(products ProductRepository) *ListProducts
func (u *ListProducts) Execute(ctx context.Context) ([]domain.Product, error)
```
- `SlotLimiter.Claim` returns an error matching `domain.ErrSlotHeld` when another owner holds the slot.

- [ ] **Step 1: Write fakes and the failing test**

`backend/internal/usecase/fakes_test.go`:
```go
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

type fakeLimits struct{ limits map[domain.ProductID]time.Duration }

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
```

`backend/internal/usecase/list_products_test.go`:
```go
package usecase

import (
	"context"
	"errors"
	"testing"
)

func TestListProducts_ReturnsTheCatalog(t *testing.T) {
	uc := NewListProducts(&fakeProducts{items: seedProducts()})

	got, err := uc.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 7 || got[0].Name != "Red set" {
		t.Fatalf("got %+v", got)
	}
}

func TestListProducts_PropagatesRepositoryErrors(t *testing.T) {
	boom := errors.New("db down")
	uc := NewListProducts(&fakeProducts{err: boom})

	if _, err := uc.Execute(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}
```

- [ ] **Step 2: Run and watch it fail**

Run: `cd backend && go test ./internal/usecase/ -v`
Expected: FAIL, `undefined: NewListProducts`.

- [ ] **Step 3: Minimal implementation**

`backend/internal/usecase/ports.go`:
```go
package usecase

import (
	"context"
	"time"

	"github.com/comnarin/foodstore/internal/domain"
)

// Ports: what the use cases need from the outside world. Adapters implement them.

type ProductRepository interface {
	List(ctx context.Context) ([]domain.Product, error)
}

type MemberRepository interface {
	// FindByCard expects a normalized card number.
	FindByCard(ctx context.Context, card string) (domain.Member, bool, error)
}

type RuleRepository interface {
	ActiveRules(ctx context.Context) ([]domain.Rule, error)
}

type LimitRepository interface {
	// ActiveLimits maps each limited product to the length of its exclusive window.
	ActiveLimits(ctx context.Context) (map[domain.ProductID]time.Duration, error)
}

type OrderRepository interface {
	Save(ctx context.Context, o domain.Order) error
}

type SlotLimiter interface {
	// Claim reserves the slot for ownerID. It succeeds if the slot is free or
	// already held by ownerID (without extending it). If another owner holds
	// it, the returned error matches domain.ErrSlotHeld.
	Claim(ctx context.Context, productID domain.ProductID, ownerID string, ttl time.Duration) error
}

type Clock interface{ Now() time.Time }

type IDGenerator interface{ NewID() string }
```

`backend/internal/usecase/list_products.go`:
```go
package usecase

import (
	"context"
	"fmt"

	"github.com/comnarin/foodstore/internal/domain"
)

type ListProducts struct{ products ProductRepository }

func NewListProducts(products ProductRepository) *ListProducts {
	return &ListProducts{products: products}
}

func (u *ListProducts) Execute(ctx context.Context) ([]domain.Product, error) {
	items, err := u.products.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	return items, nil
}
```

- [ ] **Step 4: Run and watch it pass**

Run: `cd backend && go test -race ./internal/usecase/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/comnarin/Documents/Stelligence/Code-homework
git add backend
git commit -m "feat(usecase): add ports, fakes and ListProducts" -m "Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>"
```

---

## Task 6: `CalculateOrder` happy path (pricing, member, order saved)

**Files:**
- Create: `backend/internal/usecase/calculate_order.go`
- Test: `backend/internal/usecase/calculate_order_test.go`

**Interfaces:**
- Consumes: all ports and fakes from Task 5; `domain.Calculate`, `domain.NewBundlePercent`, `domain.NewMemberPercent`.
- Produces:
```go
const MaxQuantityPerLine = 1000
type ItemInput struct{ ProductID string; Quantity int }
type CalculateInput struct{ CustomerID, MemberCard string; Items []ItemInput }
type CalculateOrderDeps struct {
	Products ProductRepository; Members MemberRepository; Rules RuleRepository
	Limits LimitRepository; Orders OrderRepository; Limiter SlotLimiter
	Clock Clock; IDs IDGenerator
}
func NewCalculateOrder(d CalculateOrderDeps) *CalculateOrder
func (u *CalculateOrder) Execute(ctx context.Context, in CalculateInput) (domain.Quote, error)
```

This task implements the full `Execute` flow in the order: merge/validate items -> resolve products -> check member -> load rules -> price -> load limits and claim slots -> save. Tests in this task cover the happy paths; Task 7 adds the rejection and failure tests, each written and watched failing first (the code that satisfies them is already in place only for behaviors the happy path requires; see the note in Task 7).

- [ ] **Step 1: Write the failing happy-path tests**

`backend/internal/usecase/calculate_order_test.go`:
```go
package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/comnarin/foodstore/internal/domain"
)

type fixture struct {
	products *fakeProducts
	members  *fakeMembers
	rules    *fakeRules
	limits   *fakeLimits
	orders   *fakeOrders
	limiter  *fakeLimiter
	uc       *CalculateOrder
}

var testNow = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pair, err := domain.NewBundlePercent("PAIR_5", 10, []domain.ProductID{"orange", "pink", "green"}, 2, 500)
	if err != nil {
		t.Fatal(err)
	}
	member, err := domain.NewMemberPercent("MEMBER_10", 100, 1000)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{
		products: &fakeProducts{items: seedProducts()},
		members: &fakeMembers{byCard: map[string]domain.Member{
			"MEM0001": {CardNumber: "MEM0001", FullName: "Active Member", Active: true},
			"MEM0002": {CardNumber: "MEM0002", FullName: "Suspended Member", Active: false},
		}},
		rules:   &fakeRules{rules: []domain.Rule{pair, member}},
		limits:  &fakeLimits{limits: map[domain.ProductID]time.Duration{"red": time.Hour}},
		orders:  &fakeOrders{},
		limiter: &fakeLimiter{},
	}
	f.uc = NewCalculateOrder(CalculateOrderDeps{
		Products: f.products, Members: f.members, Rules: f.rules, Limits: f.limits,
		Orders: f.orders, Limiter: f.limiter, Clock: fixedClock{testNow}, IDs: &seqIDs{},
	})
	return f
}

func items(pairs ...any) []ItemInput {
	var out []ItemInput
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, ItemInput{ProductID: pairs[i].(string), Quantity: pairs[i+1].(int)})
	}
	return out
}

func TestCalculateOrder_PricesAPairDiscountAndSavesTheOrder(t *testing.T) {
	f := newFixture(t)

	q, err := f.uc.Execute(context.Background(), CalculateInput{CustomerID: "cust-1", Items: items("orange", 2)})
	if err != nil {
		t.Fatal(err)
	}
	if q.TotalBefore != 24000 || q.FinalTotal != 22800 {
		t.Fatalf("got %+v", q)
	}
	if len(f.orders.saved) != 1 {
		t.Fatalf("want 1 saved order, got %d", len(f.orders.saved))
	}
	o := f.orders.saved[0]
	if o.ID != "order-1" || o.CustomerID != "cust-1" || !o.CreatedAt.Equal(testNow) || o.MemberCard != "" {
		t.Fatalf("saved order wrong: %+v", o)
	}
	if len(o.Lines) != 1 || o.Lines[0].Product.ID != "orange" || o.Lines[0].Quantity != 2 {
		t.Fatalf("saved lines wrong: %+v", o.Lines)
	}
}

func TestCalculateOrder_AppliesMemberDiscountAfterPairDiscount(t *testing.T) {
	f := newFixture(t)

	// Card is entered messily on purpose: the use case normalizes it.
	q, err := f.uc.Execute(context.Background(), CalculateInput{
		CustomerID: "cust-1", MemberCard: "  mem0001 ", Items: items("orange", 2),
	})
	if err != nil {
		t.Fatal(err)
	}
	if q.MemberDiscount() != 2280 || q.FinalTotal != 20520 {
		t.Fatalf("got member=%d final=%d, want 2280/20520", q.MemberDiscount(), q.FinalTotal)
	}
	if got := f.orders.saved[0].MemberCard; got != "MEM0001" {
		t.Fatalf("saved card = %q, want MEM0001", got)
	}
}

func TestCalculateOrder_MergesDuplicateProductLines(t *testing.T) {
	f := newFixture(t)

	q, err := f.uc.Execute(context.Background(), CalculateInput{
		CustomerID: "cust-1", Items: items("orange", 1, "orange", 1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if q.FinalTotal != 22800 { // one pair discount, as if orange x2
		t.Fatalf("FinalTotal = %d, want 22800", q.FinalTotal)
	}
	if len(f.orders.saved[0].Lines) != 1 {
		t.Fatalf("duplicate lines should merge, got %+v", f.orders.saved[0].Lines)
	}
}

func TestCalculateOrder_SameCustomerMayOrderRedTwiceInTheHour(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if _, err := f.uc.Execute(ctx, CalculateInput{CustomerID: "cust-1", Items: items("red", 1)}); err != nil {
		t.Fatal(err)
	}
	q, err := f.uc.Execute(ctx, CalculateInput{CustomerID: "cust-1", MemberCard: "MEM0001", Items: items("red", 1)})
	if err != nil {
		t.Fatalf("second Red order by the same customer should be allowed: %v", err)
	}
	if q.FinalTotal != 4500 { // 5000 - 10% member
		t.Fatalf("FinalTotal = %d, want 4500", q.FinalTotal)
	}
	if len(f.orders.saved) != 2 {
		t.Fatalf("want 2 saved orders, got %d", len(f.orders.saved))
	}
}

func TestCalculateOrder_OnlyClaimsSlotsForLimitedProducts(t *testing.T) {
	f := newFixture(t)

	if _, err := f.uc.Execute(context.Background(), CalculateInput{CustomerID: "cust-1", Items: items("blue", 1, "red", 1)}); err != nil {
		t.Fatal(err)
	}
	if len(f.limiter.claims) != 1 || f.limiter.claims[0] != "red" {
		t.Fatalf("claims = %v, want only [red]", f.limiter.claims)
	}
}
```

- [ ] **Step 2: Run and watch it fail**

Run: `cd backend && go test ./internal/usecase/ -v`
Expected: FAIL, `undefined: NewCalculateOrder`.

- [ ] **Step 3: Implementation**

`backend/internal/usecase/calculate_order.go`:
```go
package usecase

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/comnarin/foodstore/internal/domain"
)

// MaxQuantityPerLine bounds a single product line so a typo cannot overflow money math.
const MaxQuantityPerLine = 1000

type ItemInput struct {
	ProductID string
	Quantity  int
}

type CalculateInput struct {
	CustomerID string
	MemberCard string // optional, any casing/spacing
	Items      []ItemInput
}

type CalculateOrderDeps struct {
	Products ProductRepository
	Members  MemberRepository
	Rules    RuleRepository
	Limits   LimitRepository
	Orders   OrderRepository
	Limiter  SlotLimiter
	Clock    Clock
	IDs      IDGenerator
}

type CalculateOrder struct{ d CalculateOrderDeps }

func NewCalculateOrder(d CalculateOrderDeps) *CalculateOrder { return &CalculateOrder{d: d} }

// Execute validates the request, prices it, reserves any limited products for
// the customer, saves the order and returns the quote. Nothing is reserved or
// saved unless every validation step has passed.
func (u *CalculateOrder) Execute(ctx context.Context, in CalculateInput) (domain.Quote, error) {
	merged, err := mergeItems(in.Items)
	if err != nil {
		return domain.Quote{}, err
	}

	lines, err := u.resolveLines(ctx, merged)
	if err != nil {
		return domain.Quote{}, err
	}

	card := domain.NormalizeCard(in.MemberCard)
	if card != "" {
		if err := u.checkMember(ctx, card); err != nil {
			return domain.Quote{}, err
		}
	}

	rules, err := u.d.Rules.ActiveRules(ctx)
	if err != nil {
		return domain.Quote{}, fmt.Errorf("load rules: %w", err)
	}
	quote, err := domain.Calculate(lines, card != "", rules)
	if err != nil {
		return domain.Quote{}, err
	}

	if err := u.claimLimited(ctx, in.CustomerID, lines); err != nil {
		return domain.Quote{}, err
	}

	order := domain.Order{
		ID:         u.d.IDs.NewID(),
		CustomerID: in.CustomerID,
		MemberCard: card,
		Lines:      lines,
		Quote:      quote,
		CreatedAt:  u.d.Clock.Now(),
	}
	if err := u.d.Orders.Save(ctx, order); err != nil {
		return domain.Quote{}, fmt.Errorf("save order: %w", err)
	}
	return quote, nil
}

type mergedItem struct {
	id  domain.ProductID
	qty int
}

// mergeItems validates quantities and merges repeated products, keeping first-seen order.
func mergeItems(items []ItemInput) ([]mergedItem, error) {
	if len(items) == 0 {
		return nil, domain.ErrEmptyOrder
	}
	index := make(map[domain.ProductID]int, len(items))
	var out []mergedItem
	for _, it := range items {
		if it.Quantity < 1 || it.Quantity > MaxQuantityPerLine {
			return nil, fmt.Errorf("%w: %q has quantity %d", domain.ErrInvalidQuantity, it.ProductID, it.Quantity)
		}
		id := domain.ProductID(it.ProductID)
		if i, ok := index[id]; ok {
			out[i].qty += it.Quantity
			continue
		}
		index[id] = len(out)
		out = append(out, mergedItem{id: id, qty: it.Quantity})
	}
	for _, m := range out {
		if m.qty > MaxQuantityPerLine {
			return nil, fmt.Errorf("%w: %q has quantity %d", domain.ErrInvalidQuantity, m.id, m.qty)
		}
	}
	return out, nil
}

func (u *CalculateOrder) resolveLines(ctx context.Context, merged []mergedItem) ([]domain.Line, error) {
	catalog, err := u.d.Products.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	byID := make(map[domain.ProductID]domain.Product, len(catalog))
	for _, p := range catalog {
		byID[p.ID] = p
	}
	lines := make([]domain.Line, 0, len(merged))
	for _, m := range merged {
		p, ok := byID[m.id]
		if !ok {
			return nil, fmt.Errorf("%w: %q", domain.ErrUnknownProduct, m.id)
		}
		lines = append(lines, domain.Line{Product: p, Quantity: m.qty})
	}
	return lines, nil
}

func (u *CalculateOrder) checkMember(ctx context.Context, card string) error {
	m, found, err := u.d.Members.FindByCard(ctx, card)
	if err != nil {
		return fmt.Errorf("find member: %w", err)
	}
	if !found || !m.Active {
		return domain.ErrInvalidMemberCard
	}
	return nil
}

// claimLimited reserves every ordered product that has an active limit, in
// product-ID order so concurrent orders cannot deadlock each other.
func (u *CalculateOrder) claimLimited(ctx context.Context, customerID string, lines []domain.Line) error {
	limits, err := u.d.Limits.ActiveLimits(ctx)
	if err != nil {
		return fmt.Errorf("load limits: %w", err)
	}
	ordered := append([]domain.Line(nil), lines...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Product.ID < ordered[j].Product.ID })

	for _, l := range ordered {
		window, limited := limits[l.Product.ID]
		if !limited {
			continue
		}
		err := u.d.Limiter.Claim(ctx, l.Product.ID, customerID, window)
		switch {
		case err == nil:
		case errors.Is(err, domain.ErrSlotHeld):
			return &domain.SlotHeldError{ProductID: l.Product.ID, ProductName: l.Product.Name}
		default:
			return fmt.Errorf("claim %s: %w", l.Product.ID, err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run and watch it pass**

Run: `cd backend && go test -race ./internal/usecase/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/comnarin/Documents/Stelligence/Code-homework
git add backend
git commit -m "feat(usecase): add CalculateOrder happy path" -m "Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>"
```

---

## Task 7: `CalculateOrder` rejections and failures

These tests pin behaviors that the Task 6 implementation already provides by design (validation order, claim-after-validation, error mapping). Write each test, run it, and confirm it passes for the right reason. To prove each test can fail, temporarily break the line it guards (listed per test), see it go red, then restore.

**Files:**
- Modify: `backend/internal/usecase/calculate_order_test.go` (append)

**Interfaces:**
- Consumes: `newFixture`, `items`, fakes from Tasks 5 and 6.

- [ ] **Step 1: Append the tests**

```go
func TestCalculateOrder_RejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name  string
		items []ItemInput
		want  error
	}{
		{"empty order", nil, domain.ErrEmptyOrder},
		{"zero quantity", items("blue", 0), domain.ErrInvalidQuantity},
		{"negative quantity", items("blue", -3), domain.ErrInvalidQuantity},
		{"quantity above the limit", items("blue", MaxQuantityPerLine+1), domain.ErrInvalidQuantity},
		{"duplicates that add up above the limit", items("blue", 600, "blue", 600), domain.ErrInvalidQuantity},
		{"unknown product", items("black", 1), domain.ErrUnknownProduct},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			_, err := f.uc.Execute(context.Background(), CalculateInput{CustomerID: "cust-1", Items: tt.items})
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if len(f.orders.saved) != 0 || len(f.limiter.claims) != 0 {
				t.Fatal("an invalid request must not save an order or claim a slot")
			}
		})
	}
}

func TestCalculateOrder_RejectsUnknownOrSuspendedCardWithoutClaimingRed(t *testing.T) {
	for _, card := range []string{"NOPE", "MEM0002"} {
		t.Run(card, func(t *testing.T) {
			f := newFixture(t)
			_, err := f.uc.Execute(context.Background(), CalculateInput{
				CustomerID: "cust-1", MemberCard: card, Items: items("red", 1),
			})
			if !errors.Is(err, domain.ErrInvalidMemberCard) {
				t.Fatalf("err = %v, want ErrInvalidMemberCard", err)
			}
			if len(f.limiter.claims) != 0 || len(f.orders.saved) != 0 {
				t.Fatal("a rejected card must not claim the Red slot or save an order")
			}
		})
	}
}

func TestCalculateOrder_DifferentCustomerCannotOrderRedWhileHeld(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.uc.Execute(ctx, CalculateInput{CustomerID: "cust-1", Items: items("red", 1)}); err != nil {
		t.Fatal(err)
	}

	_, err := f.uc.Execute(ctx, CalculateInput{CustomerID: "cust-2", MemberCard: "MEM0001", Items: items("red", 1)})

	var held *domain.SlotHeldError
	if !errors.As(err, &held) || held.ProductName != "Red set" || !errors.Is(err, domain.ErrSlotHeld) {
		t.Fatalf("err = %v, want SlotHeldError for Red set", err)
	}
	if len(f.orders.saved) != 1 {
		t.Fatalf("the rejected order must not be saved, saved = %d", len(f.orders.saved))
	}
}

func TestCalculateOrder_LimiterOutageFailsTheRequestAndSavesNothing(t *testing.T) {
	f := newFixture(t)
	boom := errors.New("redis: connection refused")
	f.limiter.err = boom

	_, err := f.uc.Execute(context.Background(), CalculateInput{CustomerID: "cust-1", Items: items("red", 1)})

	if !errors.Is(err, boom) || errors.Is(err, domain.ErrSlotHeld) {
		t.Fatalf("err = %v, want the raw limiter error (not ErrSlotHeld)", err)
	}
	if len(f.orders.saved) != 0 {
		t.Fatal("no order may be saved when the limiter fails")
	}
}

func TestCalculateOrder_SaveFailureIsReturned(t *testing.T) {
	f := newFixture(t)
	boom := errors.New("db down")
	f.orders.err = boom

	if _, err := f.uc.Execute(context.Background(), CalculateInput{CustomerID: "cust-1", Items: items("blue", 1)}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

func TestCalculateOrder_ProductListFailureIsReturned(t *testing.T) {
	f := newFixture(t)
	boom := errors.New("db down")
	f.products.err = boom

	if _, err := f.uc.Execute(context.Background(), CalculateInput{CustomerID: "cust-1", Items: items("blue", 1)}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}
```
Add `"errors"` to the test file's import block.

- [ ] **Step 2: Run them**

Run: `cd backend && go test -race ./internal/usecase/ -v`
Expected: PASS.

- [ ] **Step 3: Prove the tests can fail (mutation check)**

1. In `calculate_order.go`, move the `claimLimited` call above the `checkMember` block. Run the package: `TestCalculateOrder_RejectsUnknownOrSuspendedCardWithoutClaimingRed` must FAIL. Revert.
2. Change `it.Quantity > MaxQuantityPerLine` to `it.Quantity > MaxQuantityPerLine+1`. The "quantity above the limit" case must FAIL. Revert.
3. Delete the duplicate-merge `for _, m := range out` check. "duplicates that add up above the limit" must FAIL. Revert.

Run: `cd backend && go test -race -cover ./internal/usecase/`
Expected: PASS, coverage above 90%.

- [ ] **Step 4: Commit**

```bash
cd /Users/comnarin/Documents/Stelligence/Code-homework
git add backend
git commit -m "test(usecase): pin CalculateOrder rejections, slot ordering and failure paths" -m "Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>"
```

---

## Task 8: Redis `SlotLimiter`

**Files:**
- Create: `backend/internal/adapter/redis/slot_limiter.go`
- Test: `backend/internal/adapter/redis/slot_limiter_test.go`

**Interfaces:**
- Consumes: `domain.ProductID`, `domain.ErrSlotHeld`.
- Produces: `func NewSlotLimiter(client goredis.Scripter) *SlotLimiter` and `func (l *SlotLimiter) Claim(ctx, productID domain.ProductID, ownerID string, ttl time.Duration) error`, which satisfies `usecase.SlotLimiter`. Key: `limit:{productID}:holder`.

- [ ] **Step 1: Add dependencies**

```bash
cd backend
go get github.com/redis/go-redis/v9 github.com/alicebob/miniredis/v2
```

- [ ] **Step 2: Write the failing tests**

`backend/internal/adapter/redis/slot_limiter_test.go`:
```go
package redis

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"github.com/comnarin/foodstore/internal/domain"
	"github.com/comnarin/foodstore/internal/usecase"
)

var _ usecase.SlotLimiter = (*SlotLimiter)(nil)

func newLimiter(t *testing.T) (*SlotLimiter, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewSlotLimiter(client), mr
}

func TestSlotLimiter_FreeSlotIsClaimedWithTTL(t *testing.T) {
	l, mr := newLimiter(t)

	if err := l.Claim(context.Background(), "red", "cust-1", time.Hour); err != nil {
		t.Fatal(err)
	}
	if got, _ := mr.Get("limit:red:holder"); got != "cust-1" {
		t.Fatalf("holder = %q, want cust-1", got)
	}
	if ttl := mr.TTL("limit:red:holder"); ttl != time.Hour {
		t.Fatalf("ttl = %v, want 1h", ttl)
	}
}

func TestSlotLimiter_SameOwnerMayClaimAgainWithoutExtendingTheTTL(t *testing.T) {
	l, mr := newLimiter(t)
	ctx := context.Background()
	if err := l.Claim(ctx, "red", "cust-1", time.Hour); err != nil {
		t.Fatal(err)
	}
	mr.FastForward(30 * time.Minute)

	if err := l.Claim(ctx, "red", "cust-1", time.Hour); err != nil {
		t.Fatalf("holder should be able to re-claim: %v", err)
	}
	if ttl := mr.TTL("limit:red:holder"); ttl != 30*time.Minute {
		t.Fatalf("ttl = %v, want 30m (must not be extended)", ttl)
	}
}

func TestSlotLimiter_OtherOwnerIsRejectedWhileHeld(t *testing.T) {
	l, _ := newLimiter(t)
	ctx := context.Background()
	if err := l.Claim(ctx, "red", "cust-1", time.Hour); err != nil {
		t.Fatal(err)
	}

	if err := l.Claim(ctx, "red", "cust-2", time.Hour); !errors.Is(err, domain.ErrSlotHeld) {
		t.Fatalf("err = %v, want ErrSlotHeld", err)
	}
}

func TestSlotLimiter_SlotFreesAfterTheWindow(t *testing.T) {
	l, mr := newLimiter(t)
	ctx := context.Background()
	if err := l.Claim(ctx, "red", "cust-1", time.Hour); err != nil {
		t.Fatal(err)
	}
	mr.FastForward(time.Hour + time.Second)

	if err := l.Claim(ctx, "red", "cust-2", time.Hour); err != nil {
		t.Fatalf("slot should be free after the window: %v", err)
	}
}

func TestSlotLimiter_ProductsAreIndependent(t *testing.T) {
	l, _ := newLimiter(t)
	ctx := context.Background()
	if err := l.Claim(ctx, "red", "cust-1", time.Hour); err != nil {
		t.Fatal(err)
	}

	if err := l.Claim(ctx, "diamond", "cust-2", time.Hour); err != nil {
		t.Fatalf("a different product has its own slot: %v", err)
	}
}

func TestSlotLimiter_ExactlyOneOfManyConcurrentOwnersWins(t *testing.T) {
	l, _ := newLimiter(t)
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(owner string) {
			defer wg.Done()
			if err := l.Claim(context.Background(), "red", owner, time.Hour); err == nil {
				wins.Add(1)
			}
		}(fmt.Sprintf("cust-%d", i))
	}
	wg.Wait()

	if wins.Load() != 1 {
		t.Fatalf("%d owners won the slot, want exactly 1", wins.Load())
	}
}

func TestSlotLimiter_RedisErrorIsReturnedNotMistakenForHeld(t *testing.T) {
	l, mr := newLimiter(t)
	mr.Close()

	err := l.Claim(context.Background(), "red", "cust-1", time.Hour)
	if err == nil || errors.Is(err, domain.ErrSlotHeld) {
		t.Fatalf("err = %v, want a connection error that is not ErrSlotHeld", err)
	}
}
```

- [ ] **Step 3: Run and watch it fail**

Run: `cd backend && go test ./internal/adapter/redis/ -v`
Expected: FAIL, `undefined: NewSlotLimiter`.

- [ ] **Step 4: Minimal implementation**

`backend/internal/adapter/redis/slot_limiter.go`:
```go
// Package redis implements usecase.SlotLimiter on Redis.
package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/comnarin/foodstore/internal/domain"
)

// claimScript is atomic: a check-then-set done in Go would let two customers
// both see an empty slot. Returns 1 when ARGV[1] now holds the slot, else 0.
var claimScript = goredis.NewScript(`
local holder = redis.call('GET', KEYS[1])
if not holder then
  redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
  return 1
end
if holder == ARGV[1] then
  return 1
end
return 0
`)

type SlotLimiter struct{ client goredis.Scripter }

func NewSlotLimiter(client goredis.Scripter) *SlotLimiter { return &SlotLimiter{client: client} }

func slotKey(p domain.ProductID) string { return "limit:" + string(p) + ":holder" }

func (l *SlotLimiter) Claim(ctx context.Context, productID domain.ProductID, ownerID string, ttl time.Duration) error {
	won, err := claimScript.Run(ctx, l.client, []string{slotKey(productID)}, ownerID, ttl.Milliseconds()).Int()
	if err != nil {
		return fmt.Errorf("redis claim %s: %w", productID, err)
	}
	if won != 1 {
		return domain.ErrSlotHeld
	}
	return nil
}
```

- [ ] **Step 5: Run and watch it pass**

Run: `cd backend && go test -race ./internal/adapter/redis/ -v`
Expected: PASS (7 tests).

- [ ] **Step 6: Commit**

```bash
cd /Users/comnarin/Documents/Stelligence/Code-homework
git add backend
git commit -m "feat(adapter): add atomic Redis SlotLimiter" -m "Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>"
```

---

## Task 9: Migrations and DB platform code

**Files:**
- Create: `backend/migrations/migrations.go`, `000001_init.up.sql`, `000001_init.down.sql`, `000002_seed.up.sql`, `000002_seed.down.sql`, `backend/internal/platform/db/db.go`
- Test: `backend/internal/adapter/postgres/helper_test.go`, `seed_test.go`

**Interfaces:**
- Produces:
  - `package migrations`: `var FS embed.FS`
  - `package db`: `func NewPool(ctx context.Context, url string) (*pgxpool.Pool, error)`, `func Migrate(databaseURL string) error` (idempotent)
  - test helper `testPool(t) *pgxpool.Pool` in `adapter/postgres` tests: skips unless `DATABASE_URL` is set; migrates once; truncates `orders` (cascade) before each test.

- [ ] **Step 1: Start a test Postgres and add dependencies**

```bash
docker run -d --name foodstore-test-pg -e POSTGRES_PASSWORD=pw -e POSTGRES_DB=foodstore -p 55432:5432 postgres:17-alpine
export DATABASE_URL='postgres://postgres:pw@localhost:55432/foodstore?sslmode=disable'
cd backend
go get github.com/jackc/pgx/v5 github.com/golang-migrate/migrate/v4
```

- [ ] **Step 2: Write the failing seed test and helper**

`backend/internal/adapter/postgres/helper_test.go`:
```go
package postgres

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/comnarin/foodstore/internal/platform/db"
)

var migrateOnce sync.Once

// testPool returns a pool on a migrated database with no orders, or skips the
// test when DATABASE_URL is not set.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping Postgres integration test")
	}
	var migrateErr error
	migrateOnce.Do(func() { migrateErr = db.Migrate(url) })
	if migrateErr != nil {
		t.Fatalf("migrate: %v", migrateErr)
	}
	pool, err := db.NewPool(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(context.Background(), "TRUNCATE orders CASCADE"); err != nil {
		t.Fatal(err)
	}
	return pool
}
```

`backend/internal/adapter/postgres/seed_test.go`:
```go
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
```

- [ ] **Step 3: Run and watch it fail**

Run: `cd backend && go test ./internal/adapter/postgres/ -v`
Expected: FAIL to compile, `no required module provides package .../internal/platform/db`.

- [ ] **Step 4: Implement migrations and the db package**

`backend/migrations/000001_init.up.sql`:
```sql
CREATE TABLE products (
    id           text PRIMARY KEY,
    name         text NOT NULL,
    price_satang integer NOT NULL CHECK (price_satang > 0),
    sort_order   integer NOT NULL
);

CREATE TABLE members (
    card_number text PRIMARY KEY,
    full_name   text NOT NULL,
    status      text NOT NULL CHECK (status IN ('ACTIVE', 'SUSPENDED')),
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE rules (
    id          serial PRIMARY KEY,
    code        text NOT NULL UNIQUE,
    type        text NOT NULL CHECK (type IN ('BUNDLE_PERCENT', 'MEMBER_PERCENT')),
    percent_bp  integer NOT NULL CHECK (percent_bp BETWEEN 1 AND 10000),
    bundle_size integer,
    priority    integer NOT NULL,
    active      boolean NOT NULL DEFAULT true,
    CHECK ((type = 'BUNDLE_PERCENT') = (bundle_size IS NOT NULL))
);

CREATE TABLE rule_products (
    rule_id    integer NOT NULL REFERENCES rules (id) ON DELETE CASCADE,
    product_id text NOT NULL REFERENCES products (id),
    PRIMARY KEY (rule_id, product_id)
);

CREATE TABLE product_limits (
    product_id     text PRIMARY KEY REFERENCES products (id),
    window_seconds integer NOT NULL CHECK (window_seconds > 0),
    active         boolean NOT NULL DEFAULT true
);

CREATE TABLE orders (
    id                    uuid PRIMARY KEY,
    customer_id           text NOT NULL,
    member_card           text REFERENCES members (card_number),
    total_before_satang   integer NOT NULL,
    member_discount_satang integer NOT NULL,
    final_total_satang    integer NOT NULL,
    created_at            timestamptz NOT NULL
);

CREATE TABLE order_items (
    order_id   uuid NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    product_id text NOT NULL REFERENCES products (id),
    quantity   integer NOT NULL CHECK (quantity > 0),
    PRIMARY KEY (order_id, product_id)
);

CREATE TABLE order_discounts (
    id            serial PRIMARY KEY,
    order_id      uuid NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    rule_code     text NOT NULL,
    product_id    text REFERENCES products (id),
    amount_satang integer NOT NULL
);
```

`backend/migrations/000001_init.down.sql`:
```sql
DROP TABLE IF EXISTS order_discounts, order_items, orders, product_limits, rule_products, rules, members, products;
```

`backend/migrations/000002_seed.up.sql`:
```sql
INSERT INTO products (id, name, price_satang, sort_order) VALUES
    ('red',    'Red set',    5000, 1),
    ('green',  'Green set',  4000, 2),
    ('blue',   'Blue set',   3000, 3),
    ('yellow', 'Yellow set', 5000, 4),
    ('pink',   'Pink set',   8000, 5),
    ('purple', 'Purple set', 9000, 6),
    ('orange', 'Orange set', 12000, 7);

INSERT INTO members (card_number, full_name, status) VALUES
    ('MEM0001', 'Demo Member', 'ACTIVE'),
    ('MEM0002', 'Suspended Member', 'SUSPENDED');

INSERT INTO rules (code, type, percent_bp, bundle_size, priority) VALUES
    ('PAIR_5',    'BUNDLE_PERCENT', 500,  2,    10),
    ('MEMBER_10', 'MEMBER_PERCENT', 1000, NULL, 100);

INSERT INTO rule_products (rule_id, product_id)
SELECT r.id, p FROM rules r, unnest(ARRAY['orange', 'pink', 'green']) AS p
WHERE r.code = 'PAIR_5';

INSERT INTO product_limits (product_id, window_seconds) VALUES ('red', 3600);
```

`backend/migrations/000002_seed.down.sql`:
```sql
DELETE FROM product_limits;
DELETE FROM rule_products;
DELETE FROM rules;
DELETE FROM members;
DELETE FROM products;
```

`backend/migrations/migrations.go`:
```go
// Package migrations embeds the SQL schema and seed files.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
```

`backend/internal/platform/db/db.go`:
```go
// Package db holds Postgres connection and migration helpers.
package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/comnarin/foodstore/migrations"
)

func NewPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return pool, nil
}

// Migrate applies all pending migrations. It is safe to call on every start.
func Migrate(databaseURL string) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("open migrations: %w", err)
	}
	dsn := strings.Replace(databaseURL, "postgres://", "pgx5://", 1)
	dsn = strings.Replace(dsn, "postgresql://", "pgx5://", 1)
	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	if err != nil {
		return fmt.Errorf("init migrate: %w", err)
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Run and watch it pass**

Run: `cd backend && go mod tidy && go test -race ./internal/adapter/postgres/ -v`
Expected: PASS for `TestSeed_MatchesThePDF`. Run it twice to prove migrations are idempotent.

- [ ] **Step 6: Commit**

```bash
cd /Users/comnarin/Documents/Stelligence/Code-homework
git add backend
git commit -m "feat(platform): add schema, seed migrations and db helpers" -m "Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>"
```

---

## Task 10: Postgres repositories

**Files:**
- Create: `backend/internal/adapter/postgres/products.go`, `members.go`, `rules.go`, `limits.go`, `orders.go`
- Test: `products_test.go`, `members_test.go`, `rules_test.go`, `limits_test.go`, `orders_test.go` (same dir)

**Interfaces:**
- Consumes: `testPool` (Task 9), domain types, usecase ports.
- Produces (each satisfies the matching port; assert with `var _ usecase.X = (*Y)(nil)` in the tests):
  - `NewProductRepository(pool *pgxpool.Pool) *ProductRepository`
  - `NewMemberRepository(pool) *MemberRepository`
  - `NewRuleRepository(pool) *RuleRepository`
  - `NewLimitRepository(pool) *LimitRepository`
  - `NewOrderRepository(pool) *OrderRepository`

- [ ] **Step 1: Write the failing tests**

`backend/internal/adapter/postgres/products_test.go`:
```go
package postgres

import (
	"context"
	"testing"

	"github.com/comnarin/foodstore/internal/usecase"
)

var _ usecase.ProductRepository = (*ProductRepository)(nil)

func TestProductRepository_ListReturnsMenuInDisplayOrder(t *testing.T) {
	repo := NewProductRepository(testPool(t))

	got, err := repo.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 7 || got[0].ID != "red" || got[0].Price != 5000 || got[6].ID != "orange" || got[6].Name != "Orange set" {
		t.Fatalf("got %+v", got)
	}
}
```

`backend/internal/adapter/postgres/members_test.go`:
```go
package postgres

import (
	"context"
	"testing"

	"github.com/comnarin/foodstore/internal/usecase"
)

var _ usecase.MemberRepository = (*MemberRepository)(nil)

func TestMemberRepository_FindByCard(t *testing.T) {
	repo := NewMemberRepository(testPool(t))
	ctx := context.Background()

	m, found, err := repo.FindByCard(ctx, "MEM0001")
	if err != nil || !found || !m.Active || m.CardNumber != "MEM0001" {
		t.Fatalf("active member: %+v found=%v err=%v", m, found, err)
	}
	m, found, err = repo.FindByCard(ctx, "MEM0002")
	if err != nil || !found || m.Active {
		t.Fatalf("suspended member should be found but inactive: %+v found=%v err=%v", m, found, err)
	}
	if _, found, err = repo.FindByCard(ctx, "NOPE"); err != nil || found {
		t.Fatalf("unknown card: found=%v err=%v, want found=false err=nil", found, err)
	}
}
```

`backend/internal/adapter/postgres/rules_test.go`:
```go
package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/comnarin/foodstore/internal/domain"
	"github.com/comnarin/foodstore/internal/usecase"
)

var _ usecase.RuleRepository = (*RuleRepository)(nil)

func TestRuleRepository_ActiveRulesMapsSeedRowsToDomainRules(t *testing.T) {
	repo := NewRuleRepository(testPool(t))

	rules, err := repo.ActiveRules(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 || rules[0].Code() != "PAIR_5" || rules[1].Code() != "MEMBER_10" {
		t.Fatalf("rules = %v, want PAIR_5 then MEMBER_10 (by priority)", codes(rules))
	}

	// Behavior check: the mapped rules price the PDF example correctly.
	orange := domain.Product{ID: "orange", Name: "Orange set", Price: 12000}
	q, err := domain.Calculate([]domain.Line{{Product: orange, Quantity: 2}}, true, rules)
	if err != nil {
		t.Fatal(err)
	}
	if q.FinalTotal != 20520 {
		t.Fatalf("FinalTotal = %d, want 20520", q.FinalTotal)
	}
}

func TestRuleRepository_SkipsInactiveRules(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "UPDATE rules SET active = false WHERE code = 'MEMBER_10'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "UPDATE rules SET active = true WHERE code = 'MEMBER_10'") })

	rules, err := NewRuleRepository(pool).ActiveRules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].Code() != "PAIR_5" {
		t.Fatalf("rules = %v, want only PAIR_5", codes(rules))
	}
}

func TestRuleRepository_UnknownRuleTypeFailsLoudly(t *testing.T) {
	_, err := buildRule(ruleRow{code: "X", typ: "MYSTERY", percentBP: 500, priority: 1})
	if !errors.Is(err, domain.ErrInvalidRule) {
		t.Fatalf("err = %v, want ErrInvalidRule", err)
	}
}

func codes(rules []domain.Rule) []string {
	out := make([]string, len(rules))
	for i, r := range rules {
		out[i] = r.Code()
	}
	return out
}
```

`backend/internal/adapter/postgres/limits_test.go`:
```go
package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/comnarin/foodstore/internal/usecase"
)

var _ usecase.LimitRepository = (*LimitRepository)(nil)

func TestLimitRepository_ActiveLimitsReturnsRedForOneHour(t *testing.T) {
	got, err := NewLimitRepository(testPool(t)).ActiveLimits(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["red"] != time.Hour {
		t.Fatalf("limits = %v, want map[red:1h]", got)
	}
}
```

`backend/internal/adapter/postgres/orders_test.go`:
```go
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
```

- [ ] **Step 2: Run and watch them fail**

Run: `cd backend && go get github.com/google/uuid && go test ./internal/adapter/postgres/ -v`
Expected: FAIL to compile, `undefined: NewProductRepository` and friends.

- [ ] **Step 3: Implement the repositories**

`backend/internal/adapter/postgres/products.go`:
```go
// Package postgres implements the use-case ports on Postgres via pgx.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/comnarin/foodstore/internal/domain"
)

type ProductRepository struct{ pool *pgxpool.Pool }

func NewProductRepository(pool *pgxpool.Pool) *ProductRepository { return &ProductRepository{pool: pool} }

func (r *ProductRepository) List(ctx context.Context) ([]domain.Product, error) {
	rows, err := r.pool.Query(ctx, "SELECT id, name, price_satang FROM products ORDER BY sort_order")
	if err != nil {
		return nil, fmt.Errorf("query products: %w", err)
	}
	defer rows.Close()

	var out []domain.Product
	for rows.Next() {
		var p domain.Product
		var id string
		var price int64
		if err := rows.Scan(&id, &p.Name, &price); err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}
		p.ID, p.Price = domain.ProductID(id), domain.Money(price)
		out = append(out, p)
	}
	return out, rows.Err()
}
```

`backend/internal/adapter/postgres/members.go`:
```go
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/comnarin/foodstore/internal/domain"
)

type MemberRepository struct{ pool *pgxpool.Pool }

func NewMemberRepository(pool *pgxpool.Pool) *MemberRepository { return &MemberRepository{pool: pool} }

func (r *MemberRepository) FindByCard(ctx context.Context, card string) (domain.Member, bool, error) {
	var m domain.Member
	var status string
	err := r.pool.QueryRow(ctx, "SELECT card_number, full_name, status FROM members WHERE card_number = $1", card).
		Scan(&m.CardNumber, &m.FullName, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Member{}, false, nil
	}
	if err != nil {
		return domain.Member{}, false, fmt.Errorf("query member: %w", err)
	}
	m.Active = status == "ACTIVE"
	return m, true, nil
}
```

`backend/internal/adapter/postgres/rules.go`:
```go
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/comnarin/foodstore/internal/domain"
)

type RuleRepository struct{ pool *pgxpool.Pool }

func NewRuleRepository(pool *pgxpool.Pool) *RuleRepository { return &RuleRepository{pool: pool} }

type ruleRow struct {
	code       string
	typ        string
	percentBP  int
	bundleSize *int
	priority   int
	products   []string
}

const activeRulesSQL = `
SELECT r.code, r.type, r.percent_bp, r.bundle_size, r.priority,
       COALESCE(array_agg(rp.product_id) FILTER (WHERE rp.product_id IS NOT NULL), '{}')
FROM rules r
LEFT JOIN rule_products rp ON rp.rule_id = r.id
WHERE r.active
GROUP BY r.id
ORDER BY r.priority, r.id`

func (r *RuleRepository) ActiveRules(ctx context.Context) ([]domain.Rule, error) {
	rows, err := r.pool.Query(ctx, activeRulesSQL)
	if err != nil {
		return nil, fmt.Errorf("query rules: %w", err)
	}
	defer rows.Close()

	var out []domain.Rule
	for rows.Next() {
		var row ruleRow
		if err := rows.Scan(&row.code, &row.typ, &row.percentBP, &row.bundleSize, &row.priority, &row.products); err != nil {
			return nil, fmt.Errorf("scan rule: %w", err)
		}
		rule, err := buildRule(row)
		if err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

// buildRule maps a table row to a domain rule. The domain knows nothing about tables.
func buildRule(row ruleRow) (domain.Rule, error) {
	switch row.typ {
	case "BUNDLE_PERCENT":
		if row.bundleSize == nil {
			return nil, fmt.Errorf("%w: %s has no bundle_size", domain.ErrInvalidRule, row.code)
		}
		products := make([]domain.ProductID, len(row.products))
		for i, p := range row.products {
			products[i] = domain.ProductID(p)
		}
		return domain.NewBundlePercent(row.code, row.priority, products, *row.bundleSize, row.percentBP)
	case "MEMBER_PERCENT":
		return domain.NewMemberPercent(row.code, row.priority, row.percentBP)
	default:
		return nil, fmt.Errorf("%w: %s has unknown type %q", domain.ErrInvalidRule, row.code, row.typ)
	}
}
```

`backend/internal/adapter/postgres/limits.go`:
```go
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/comnarin/foodstore/internal/domain"
)

type LimitRepository struct{ pool *pgxpool.Pool }

func NewLimitRepository(pool *pgxpool.Pool) *LimitRepository { return &LimitRepository{pool: pool} }

func (r *LimitRepository) ActiveLimits(ctx context.Context) (map[domain.ProductID]time.Duration, error) {
	rows, err := r.pool.Query(ctx, "SELECT product_id, window_seconds FROM product_limits WHERE active")
	if err != nil {
		return nil, fmt.Errorf("query limits: %w", err)
	}
	defer rows.Close()

	out := map[domain.ProductID]time.Duration{}
	for rows.Next() {
		var id string
		var seconds int
		if err := rows.Scan(&id, &seconds); err != nil {
			return nil, fmt.Errorf("scan limit: %w", err)
		}
		out[domain.ProductID(id)] = time.Duration(seconds) * time.Second
	}
	return out, rows.Err()
}
```

`backend/internal/adapter/postgres/orders.go`:
```go
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/comnarin/foodstore/internal/domain"
)

type OrderRepository struct{ pool *pgxpool.Pool }

func NewOrderRepository(pool *pgxpool.Pool) *OrderRepository { return &OrderRepository{pool: pool} }

// Save writes the order, its items and its discounts in one transaction.
func (r *OrderRepository) Save(ctx context.Context, o domain.Order) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	var card *string
	if o.MemberCard != "" {
		card = &o.MemberCard
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO orders (id, customer_id, member_card, total_before_satang, member_discount_satang, final_total_satang, created_at)
		 VALUES ($1::text::uuid, $2, $3, $4, $5, $6, $7)`,
		o.ID, o.CustomerID, card, int64(o.Quote.TotalBefore), int64(o.Quote.MemberDiscount()), int64(o.Quote.FinalTotal), o.CreatedAt); err != nil {
		return fmt.Errorf("insert order: %w", err)
	}
	for _, l := range o.Lines {
		if _, err := tx.Exec(ctx,
			"INSERT INTO order_items (order_id, product_id, quantity) VALUES ($1::text::uuid, $2, $3)",
			o.ID, string(l.Product.ID), l.Quantity); err != nil {
			return fmt.Errorf("insert item %s: %w", l.Product.ID, err)
		}
	}
	for _, d := range o.Quote.Discounts {
		var productID *string
		if d.ProductID != "" {
			id := string(d.ProductID)
			productID = &id
		}
		if _, err := tx.Exec(ctx,
			"INSERT INTO order_discounts (order_id, rule_code, product_id, amount_satang) VALUES ($1::text::uuid, $2, $3, $4)",
			o.ID, d.RuleCode, productID, int64(d.Amount)); err != nil {
			return fmt.Errorf("insert discount %s: %w", d.RuleCode, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run and watch them pass**

Run: `cd backend && go mod tidy && go test -race ./internal/adapter/postgres/ -v`
Expected: PASS (needs `DATABASE_URL` exported from Task 9 Step 1). Without it, tests SKIP and you must set it before moving on.

- [ ] **Step 5: Commit**

```bash
cd /Users/comnarin/Documents/Stelligence/Code-homework
git add backend
git commit -m "feat(adapter): add Postgres repositories with rule row mapping" -m "Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>"
```

---

## Task 11: Fiber HTTP adapter

**Files:**
- Create: `backend/internal/adapter/http/dto.go`, `errors.go`, `cid.go`, `handlers.go`, `router.go`
- Test: `backend/internal/adapter/http/http_test.go`

**Interfaces:**
- Consumes: `usecase.CalculateInput`, `usecase.ItemInput`, `domain.*`.
- Produces (package `httpapi`):
```go
type ProductLister interface{ Execute(ctx context.Context) ([]domain.Product, error) }
type OrderCalculator interface{ Execute(ctx context.Context, in usecase.CalculateInput) (domain.Quote, error) }
type Deps struct{ Products ProductLister; Calculator OrderCalculator; NewID func() string; CookieSecure bool }
func NewApp(d Deps) *fiber.App
```
Routes: `GET /healthz`, `GET /api/products`, `POST /api/calculate`. Cookie `cid` (HttpOnly, SameSite=Lax, Path=/, 1-year max-age) holds a UUID; invalid or missing values are replaced.

- [ ] **Step 1: Add dependencies**

```bash
cd backend && go get github.com/gofiber/fiber/v3@v3.6.1
```

- [ ] **Step 2: Write the failing tests**

`backend/internal/adapter/http/http_test.go`:
```go
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
		app: NewApp(Deps{Products: lister, Calculator: calc, NewID: uuid.NewString}),
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
			RuleCode    string `json:"ruleCode"`
			ProductID   string `json:"productId"`
			Name        string `json:"name"`
			Bundles     int    `json:"bundles"`
			AmountSatang int   `json:"amountSatang"`
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
		"empty body":     "",
		"not json":       "{{{",
		"wrong shape":    `{"items":"oops"}`,
		"quantity text":  `{"items":[{"productId":"red","quantity":"two"}]}`,
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
```

- [ ] **Step 3: Run and watch it fail**

Run: `cd backend && go test ./internal/adapter/http/ -v`
Expected: FAIL, `undefined: NewApp`, `undefined: Deps`.

- [ ] **Step 4: Implement the adapter**

`backend/internal/adapter/http/dto.go`:
```go
package httpapi

import "github.com/comnarin/foodstore/internal/domain"

type productDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	PriceSatang int64  `json:"priceSatang"`
}

type calculateRequest struct {
	MemberCard string `json:"memberCard"`
	Items      []struct {
		ProductID string `json:"productId"`
		Quantity  int    `json:"quantity"`
	} `json:"items"`
}

type itemDiscountDTO struct {
	RuleCode     string `json:"ruleCode"`
	ProductID    string `json:"productId"`
	Name         string `json:"name"`
	Bundles      int    `json:"bundles"`
	AmountSatang int64  `json:"amountSatang"`
}

type calculateResponse struct {
	TotalBeforeSatang    int64             `json:"totalBeforeSatang"`
	ItemDiscounts        []itemDiscountDTO `json:"itemDiscounts"`
	MemberDiscountSatang int64             `json:"memberDiscountSatang"`
	FinalTotalSatang     int64             `json:"finalTotalSatang"`
}

func toProductDTOs(items []domain.Product) []productDTO {
	out := make([]productDTO, 0, len(items))
	for _, p := range items {
		out = append(out, productDTO{ID: string(p.ID), Name: p.Name, PriceSatang: int64(p.Price)})
	}
	return out
}

func toCalculateResponse(q domain.Quote) calculateResponse {
	items := make([]itemDiscountDTO, 0, len(q.Discounts))
	for _, d := range q.ItemDiscounts() {
		items = append(items, itemDiscountDTO{
			RuleCode: d.RuleCode, ProductID: string(d.ProductID), Name: d.Name,
			Bundles: d.Bundles, AmountSatang: int64(d.Amount),
		})
	}
	return calculateResponse{
		TotalBeforeSatang:    int64(q.TotalBefore),
		ItemDiscounts:        items,
		MemberDiscountSatang: int64(q.MemberDiscount()),
		FinalTotalSatang:     int64(q.FinalTotal),
	}
}
```

`backend/internal/adapter/http/errors.go`:
```go
package httpapi

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/gofiber/fiber/v3"

	"github.com/comnarin/foodstore/internal/domain"
)

var errBadRequest = errors.New("invalid request body")

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// errorHandler is the single place where errors become HTTP responses.
// Handlers just return errors; unexpected ones are logged and never leaked.
func errorHandler(c fiber.Ctx, err error) error {
	status, detail := mapError(err)
	if status == fiber.StatusInternalServerError {
		slog.Error("request failed", "method", c.Method(), "path", c.Path(), "err", err)
	}
	return c.Status(status).JSON(errorBody{Error: detail})
}

func mapError(err error) (int, errorDetail) {
	var held *domain.SlotHeldError
	var fiberErr *fiber.Error
	switch {
	case errors.As(err, &held):
		return fiber.StatusConflict, errorDetail{
			Code:    "LIMITED_PRODUCT_UNAVAILABLE",
			Message: fmt.Sprintf("%s is reserved by another customer right now. Please try again later.", held.ProductName),
		}
	case errors.Is(err, domain.ErrInvalidMemberCard):
		return fiber.StatusBadRequest, errorDetail{Code: "INVALID_MEMBER_CARD", Message: "Member card not found."}
	case errors.Is(err, errBadRequest),
		errors.Is(err, domain.ErrEmptyOrder),
		errors.Is(err, domain.ErrInvalidQuantity),
		errors.Is(err, domain.ErrUnknownProduct):
		return fiber.StatusBadRequest, errorDetail{Code: "INVALID_REQUEST", Message: err.Error()}
	case errors.As(err, &fiberErr) && fiberErr.Code == fiber.StatusNotFound:
		return fiber.StatusNotFound, errorDetail{Code: "NOT_FOUND", Message: "Route not found."}
	default:
		return fiber.StatusInternalServerError, errorDetail{Code: "INTERNAL", Message: "Something went wrong. Please try again."}
	}
}
```

`backend/internal/adapter/http/cid.go`:
```go
package httpapi

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

const cidCookie = "cid"

type cidKey struct{}

// cidMiddleware gives every browser a stable anonymous customer ID. The ID is
// a random UUID, so it works as an unguessable bearer value; it is HttpOnly so
// page scripts cannot read it.
func cidMiddleware(newID func() string, secure bool) fiber.Handler {
	return func(c fiber.Ctx) error {
		id := c.Cookies(cidCookie)
		if _, err := uuid.Parse(id); err != nil {
			id = newID()
			c.Cookie(&fiber.Cookie{
				Name:     cidCookie,
				Value:    id,
				Path:     "/",
				HTTPOnly: true,
				Secure:   secure,
				SameSite: "Lax",
				Expires:  time.Now().Add(365 * 24 * time.Hour),
			})
		}
		c.Locals(cidKey{}, id)
		return c.Next()
	}
}

func customerID(c fiber.Ctx) string {
	id, _ := c.Locals(cidKey{}).(string)
	return id
}
```

`backend/internal/adapter/http/handlers.go`:
```go
package httpapi

import (
	"context"
	"fmt"

	"github.com/gofiber/fiber/v3"

	"github.com/comnarin/foodstore/internal/domain"
	"github.com/comnarin/foodstore/internal/usecase"
)

// The handlers depend on these small interfaces, not on concrete use cases.
type ProductLister interface {
	Execute(ctx context.Context) ([]domain.Product, error)
}

type OrderCalculator interface {
	Execute(ctx context.Context, in usecase.CalculateInput) (domain.Quote, error)
}

type handlers struct {
	products   ProductLister
	calculator OrderCalculator
}

func (h handlers) health(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }

func (h handlers) listProducts(c fiber.Ctx) error {
	items, err := h.products.Execute(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(toProductDTOs(items))
}

func (h handlers) calculate(c fiber.Ctx) error {
	var req calculateRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fmt.Errorf("%w: %v", errBadRequest, err)
	}
	in := usecase.CalculateInput{
		CustomerID: customerID(c),
		MemberCard: req.MemberCard,
		Items:      make([]usecase.ItemInput, len(req.Items)),
	}
	for i, it := range req.Items {
		in.Items[i] = usecase.ItemInput{ProductID: it.ProductID, Quantity: it.Quantity}
	}
	quote, err := h.calculator.Execute(c.Context(), in)
	if err != nil {
		return err
	}
	return c.JSON(toCalculateResponse(quote))
}
```

`backend/internal/adapter/http/router.go`:
```go
// Package httpapi is the Fiber delivery adapter: it parses requests, calls
// use cases and presents results. It holds no business rules.
package httpapi

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
)

type Deps struct {
	Products     ProductLister
	Calculator   OrderCalculator
	NewID        func() string
	CookieSecure bool
}

func NewApp(d Deps) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: errorHandler})
	app.Use(recover.New())
	app.Use(cidMiddleware(d.NewID, d.CookieSecure))

	h := handlers{products: d.Products, calculator: d.Calculator}
	app.Get("/healthz", h.health)
	api := app.Group("/api")
	api.Get("/products", h.listProducts)
	api.Post("/calculate", h.calculate)
	return app
}
```

- [ ] **Step 5: Run and watch it pass**

Run: `cd backend && go mod tidy && go test -race -cover ./internal/adapter/http/ -v`
Expected: PASS. If a Fiber v3 detail differs (for example the JSON decode error text), adjust the implementation only, not the asserted behavior.

- [ ] **Step 6: Commit**

```bash
cd /Users/comnarin/Documents/Stelligence/Code-homework
git add backend
git commit -m "feat(adapter): add Fiber HTTP adapter with cid cookie and error mapping" -m "Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>"
```

---

## Task 12: Config, composition root and backend Docker image

**Files:**
- Create: `backend/internal/platform/config/config.go`, `backend/cmd/server/main.go`, `backend/Dockerfile`, `backend/.dockerignore`
- Test: `backend/internal/platform/config/config_test.go`

**Interfaces:**
- Produces: `config.Load(getenv func(string) string) (Config, error)` with `Config{Port, DatabaseURL, RedisAddr string; CookieSecure bool}`. Defaults: `PORT=8080`, `REDIS_ADDR=localhost:6379`, `COOKIE_SECURE=false`. `DATABASE_URL` is required.

- [ ] **Step 1: Write the failing config tests**

`backend/internal/platform/config/config_test.go`:
```go
package config

import "testing"

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestLoad_AppliesDefaults(t *testing.T) {
	c, err := Load(env(map[string]string{"DATABASE_URL": "postgres://x"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != "8080" || c.RedisAddr != "localhost:6379" || c.CookieSecure || c.DatabaseURL != "postgres://x" {
		t.Fatalf("got %+v", c)
	}
}

func TestLoad_ReadsOverrides(t *testing.T) {
	c, err := Load(env(map[string]string{
		"DATABASE_URL": "postgres://x", "PORT": "9000", "REDIS_ADDR": "redis:6379", "COOKIE_SECURE": "true",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != "9000" || c.RedisAddr != "redis:6379" || !c.CookieSecure {
		t.Fatalf("got %+v", c)
	}
}

func TestLoad_RequiresDatabaseURL(t *testing.T) {
	if _, err := Load(env(nil)); err == nil {
		t.Fatal("want an error when DATABASE_URL is missing")
	}
}

func TestLoad_RejectsBadBool(t *testing.T) {
	if _, err := Load(env(map[string]string{"DATABASE_URL": "x", "COOKIE_SECURE": "maybe"})); err == nil {
		t.Fatal("want an error for an unparsable COOKIE_SECURE")
	}
}
```

- [ ] **Step 2: Run and watch it fail**

Run: `cd backend && go test ./internal/platform/config/ -v`
Expected: FAIL, `undefined: Load`.

- [ ] **Step 3: Implement config**

`backend/internal/platform/config/config.go`:
```go
// Package config reads process configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"strconv"
)

type Config struct {
	Port         string
	DatabaseURL  string
	RedisAddr    string
	CookieSecure bool
}

func Load(getenv func(string) string) (Config, error) {
	c := Config{
		Port:        orDefault(getenv("PORT"), "8080"),
		DatabaseURL: getenv("DATABASE_URL"),
		RedisAddr:   orDefault(getenv("REDIS_ADDR"), "localhost:6379"),
	}
	if c.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if v := getenv("COOKIE_SECURE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("COOKIE_SECURE must be true or false: %w", err)
		}
		c.CookieSecure = b
	}
	return c, nil
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
```

- [ ] **Step 4: Run and watch it pass**

Run: `cd backend && go test -race ./internal/platform/config/ -v`
Expected: PASS.

- [ ] **Step 5: Write the composition root (no unit test; verified by the smoke test in Step 7)**

`backend/cmd/server/main.go`:
```go
// Command server wires the layers together and runs the API.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"

	httpapi "github.com/comnarin/foodstore/internal/adapter/http"
	"github.com/comnarin/foodstore/internal/adapter/postgres"
	redisadapter "github.com/comnarin/foodstore/internal/adapter/redis"
	"github.com/comnarin/foodstore/internal/platform/config"
	"github.com/comnarin/foodstore/internal/platform/db"
	"github.com/comnarin/foodstore/internal/usecase"
)

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

type uuidGenerator struct{}

func (uuidGenerator) NewID() string { return uuid.NewString() }

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	if err := db.Migrate(cfg.DatabaseURL); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	rdb := goredis.NewClient(&goredis.Options{Addr: cfg.RedisAddr})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return err
	}

	products := postgres.NewProductRepository(pool)
	calculate := usecase.NewCalculateOrder(usecase.CalculateOrderDeps{
		Products: products,
		Members:  postgres.NewMemberRepository(pool),
		Rules:    postgres.NewRuleRepository(pool),
		Limits:   postgres.NewLimitRepository(pool),
		Orders:   postgres.NewOrderRepository(pool),
		Limiter:  redisadapter.NewSlotLimiter(rdb),
		Clock:    systemClock{},
		IDs:      uuidGenerator{},
	})

	app := httpapi.NewApp(httpapi.Deps{
		Products:     usecase.NewListProducts(products),
		Calculator:   calculate,
		NewID:        uuid.NewString,
		CookieSecure: cfg.CookieSecure,
	})

	go func() {
		<-ctx.Done()
		_ = app.ShutdownWithTimeout(10 * time.Second)
	}()
	slog.Info("listening", "port", cfg.Port)
	return app.Listen(":" + cfg.Port)
}
```

- [ ] **Step 6: Dockerfile**

`backend/Dockerfile`:
```dockerfile
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/server ./cmd/server

FROM alpine:3.22
RUN adduser -D -u 10001 app
USER app
COPY --from=build /out/server /server
EXPOSE 8080
ENTRYPOINT ["/server"]
```

`backend/.dockerignore`:
```
bin/
*_test.go
```

- [ ] **Step 7: Smoke-test against the local Postgres and a throwaway Redis**

```bash
docker run -d --name foodstore-test-redis -p 56379:6379 redis:7-alpine
cd backend && go vet ./... && go build ./...
DATABASE_URL='postgres://postgres:pw@localhost:55432/foodstore?sslmode=disable' REDIS_ADDR=localhost:56379 go run ./cmd/server &
sleep 3
curl -s localhost:8080/api/products | head -c 200; echo
curl -s -c /tmp/fs.jar -b /tmp/fs.jar -X POST localhost:8080/api/calculate -H 'Content-Type: application/json' -d '{"memberCard":"mem0001","items":[{"productId":"orange","quantity":2}]}'; echo
curl -s -c /tmp/fs2.jar -X POST localhost:8080/api/calculate -H 'Content-Type: application/json' -d '{"items":[{"productId":"red","quantity":1}]}'; echo
curl -s -c /tmp/fs3.jar -X POST localhost:8080/api/calculate -H 'Content-Type: application/json' -d '{"items":[{"productId":"red","quantity":1}]}'; echo
kill %1
```
Expected: products JSON with 7 items; first calculate returns `finalTotalSatang:20520`; second returns 200 (first Red customer); third (new cookie jar) returns 409 `LIMITED_PRODUCT_UNAVAILABLE`. If you re-run, flush Redis with `docker exec foodstore-test-redis redis-cli FLUSHALL`.

- [ ] **Step 8: Commit**

```bash
cd /Users/comnarin/Documents/Stelligence/Code-homework
git add backend
git commit -m "feat(backend): add config, composition root and Dockerfile" -m "Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>"
```

---

## Task 13: Frontend scaffold, test tooling and money formatting

**Files:**
- Create: `frontend/` (via create-next-app), `frontend/vitest.config.ts`, `frontend/vitest.setup.ts`, `frontend/src/lib/money.ts`, `frontend/src/lib/env.ts`
- Test: `frontend/src/lib/money.test.ts`

**Interfaces:**
- Produces: `formatTHB(satang: number): string`; `API_URL: string` (server-side only).

- [ ] **Step 1: Scaffold the app and install tooling**

```bash
cd /Users/comnarin/Documents/Stelligence/Code-homework
npx create-next-app@latest frontend --ts --tailwind --eslint --app --src-dir --turbopack --import-alias "@/*" --use-npm --yes
cd frontend
npm install zod
npm install -D vitest @vitejs/plugin-react jsdom @testing-library/react @testing-library/user-event @testing-library/jest-dom
```
Then in `frontend/package.json` add to `"scripts"`: `"test": "vitest run"`, `"test:watch": "vitest"`, `"typecheck": "tsc --noEmit"`. In `tsconfig.json` set `"noUncheckedIndexedAccess": true` under `compilerOptions`. Replace `src/app/page.tsx` with `export default function Page() { return null; }` for now.

`frontend/vitest.config.ts`:
```ts
import react from "@vitejs/plugin-react";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  test: {
    environment: "jsdom",
    setupFiles: ["./vitest.setup.ts"],
    globals: false,
    css: false,
  },
});
```

`frontend/vitest.setup.ts`:
```ts
import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

afterEach(() => cleanup());
```

- [ ] **Step 2: Write the failing test**

`frontend/src/lib/money.test.ts`:
```ts
import { describe, expect, it } from "vitest";
import { formatTHB } from "./money";

describe("formatTHB", () => {
  it("formats satang as baht with two decimals", () => {
    expect(formatTHB(22800)).toMatch(/228\.00/);
    expect(formatTHB(22800)).toContain("฿");
  });

  it("adds thousands separators", () => {
    expect(formatTHB(123456700)).toMatch(/1,234,567\.00/);
  });

  it("formats zero", () => {
    expect(formatTHB(0)).toMatch(/0\.00/);
  });

  it("keeps satang precision", () => {
    expect(formatTHB(20520)).toMatch(/205\.20/);
    expect(formatTHB(1)).toMatch(/0\.01/);
  });
});
```

- [ ] **Step 3: Run and watch it fail**

Run: `cd frontend && npm test`
Expected: FAIL, cannot resolve `./money`.

- [ ] **Step 4: Implement**

`frontend/src/lib/money.ts`:
```ts
const thb = new Intl.NumberFormat("th-TH", { style: "currency", currency: "THB" });

/** Formats an integer amount of satang (1/100 THB) as baht, for example "฿228.00". */
export function formatTHB(satang: number): string {
  return thb.format(satang / 100);
}
```

`frontend/src/lib/env.ts`:
```ts
/** Internal address of the Go API. Used only by server code (Server Components, next.config). */
export const API_URL = process.env.API_URL ?? "http://localhost:8080";
```

- [ ] **Step 5: Run and watch it pass**

Run: `cd frontend && npm test && npm run typecheck`
Expected: PASS, no type errors.

- [ ] **Step 6: Commit**

```bash
cd /Users/comnarin/Documents/Stelligence/Code-homework
git add frontend
git commit -m "feat(frontend): scaffold Next.js app with Vitest and money formatting" -m "Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>"
```

---

## Task 14: Typed API client with Zod

**Files:**
- Create: `frontend/src/features/calculator/api.ts`
- Test: `frontend/src/features/calculator/api.test.ts`

**Interfaces:**
- Produces:
```ts
export type Product = { id: string; name: string; priceSatang: number };
export type Quote = { totalBeforeSatang: number; itemDiscounts: { ruleCode: string; productId: string; name: string; bundles: number; amountSatang: number }[]; memberDiscountSatang: number; finalTotalSatang: number };
export type CalculateRequest = { memberCard?: string; items: { productId: string; quantity: number }[] };
export type CalculateResult = { ok: true; quote: Quote } | { ok: false; code: string; message: string };
export type CalculateFn = (req: CalculateRequest) => Promise<CalculateResult>;
export function calculateOrder(req: CalculateRequest, fetchFn?: typeof fetch): Promise<CalculateResult>;
export function fetchProducts(baseUrl: string, fetchFn?: typeof fetch): Promise<Product[]>;
```
- Error codes produced client-side: `NETWORK`, `BAD_RESPONSE`, `HTTP_<status>`.

- [ ] **Step 1: Write the failing tests**

`frontend/src/features/calculator/api.test.ts`:
```ts
import { describe, expect, it, vi } from "vitest";
import { calculateOrder, fetchProducts } from "./api";

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

const validQuote = {
  totalBeforeSatang: 24000,
  itemDiscounts: [{ ruleCode: "PAIR_5", productId: "orange", name: "Orange set", bundles: 1, amountSatang: 1200 }],
  memberDiscountSatang: 2280,
  finalTotalSatang: 20520,
};

describe("calculateOrder", () => {
  it("posts the order and returns the parsed quote", async () => {
    const fetchFn = vi.fn().mockResolvedValue(json(validQuote));

    const result = await calculateOrder(
      { memberCard: "MEM0001", items: [{ productId: "orange", quantity: 2 }] },
      fetchFn,
    );

    expect(result).toEqual({ ok: true, quote: validQuote });
    const [url, init] = fetchFn.mock.calls[0]!;
    expect(url).toBe("/api/calculate");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body)).toEqual({ memberCard: "MEM0001", items: [{ productId: "orange", quantity: 2 }] });
  });

  it("returns the server's error code and message", async () => {
    const fetchFn = vi.fn().mockResolvedValue(
      json({ error: { code: "LIMITED_PRODUCT_UNAVAILABLE", message: "Red set is reserved by another customer right now." } }, 409),
    );

    const result = await calculateOrder({ items: [{ productId: "red", quantity: 1 }] }, fetchFn);

    expect(result).toEqual({
      ok: false,
      code: "LIMITED_PRODUCT_UNAVAILABLE",
      message: "Red set is reserved by another customer right now.",
    });
  });

  it("reports a network failure as a retryable message", async () => {
    const fetchFn = vi.fn().mockRejectedValue(new TypeError("Failed to fetch"));

    const result = await calculateOrder({ items: [{ productId: "blue", quantity: 1 }] }, fetchFn);

    expect(result).toMatchObject({ ok: false, code: "NETWORK" });
    expect(result.ok === false && result.message).toMatch(/try again/i);
  });

  it("rejects a response that does not match the schema", async () => {
    const fetchFn = vi.fn().mockResolvedValue(json({ finalTotalSatang: "lots" }));

    const result = await calculateOrder({ items: [{ productId: "blue", quantity: 1 }] }, fetchFn);

    expect(result).toMatchObject({ ok: false, code: "BAD_RESPONSE" });
  });

  it("handles a non-JSON error page", async () => {
    const fetchFn = vi.fn().mockResolvedValue(new Response("<html>Bad gateway</html>", { status: 502 }));

    const result = await calculateOrder({ items: [{ productId: "blue", quantity: 1 }] }, fetchFn);

    expect(result).toMatchObject({ ok: false, code: "HTTP_502" });
  });
});

describe("fetchProducts", () => {
  it("returns validated products", async () => {
    const fetchFn = vi.fn().mockResolvedValue(json([{ id: "red", name: "Red set", priceSatang: 5000 }]));

    const products = await fetchProducts("http://api:8080", fetchFn);

    expect(products).toEqual([{ id: "red", name: "Red set", priceSatang: 5000 }]);
    expect(fetchFn.mock.calls[0]![0]).toBe("http://api:8080/api/products");
  });

  it("throws when the API fails, so the error boundary can render", async () => {
    const fetchFn = vi.fn().mockResolvedValue(json({}, 500));

    await expect(fetchProducts("http://api:8080", fetchFn)).rejects.toThrow(/products/i);
  });

  it("throws when the payload has the wrong shape", async () => {
    const fetchFn = vi.fn().mockResolvedValue(json([{ id: 1 }]));

    await expect(fetchProducts("http://api:8080", fetchFn)).rejects.toThrow();
  });
});
```

- [ ] **Step 2: Run and watch it fail**

Run: `cd frontend && npm test -- api`
Expected: FAIL, cannot resolve `./api`.

- [ ] **Step 3: Implement**

`frontend/src/features/calculator/api.ts`:
```ts
import { z } from "zod";

const productSchema = z.object({
  id: z.string(),
  name: z.string(),
  priceSatang: z.number().int(),
});

const quoteSchema = z.object({
  totalBeforeSatang: z.number().int(),
  itemDiscounts: z.array(
    z.object({
      ruleCode: z.string(),
      productId: z.string(),
      name: z.string(),
      bundles: z.number().int(),
      amountSatang: z.number().int(),
    }),
  ),
  memberDiscountSatang: z.number().int(),
  finalTotalSatang: z.number().int(),
});

const apiErrorSchema = z.object({ error: z.object({ code: z.string(), message: z.string() }) });

export type Product = z.infer<typeof productSchema>;
export type Quote = z.infer<typeof quoteSchema>;
export type CalculateRequest = { memberCard?: string; items: { productId: string; quantity: number }[] };
export type CalculateResult = { ok: true; quote: Quote } | { ok: false; code: string; message: string };
export type CalculateFn = (req: CalculateRequest) => Promise<CalculateResult>;

/** Called from the browser. Same-origin `/api` is proxied to the Go API, so the `cid` cookie travels with it. */
export async function calculateOrder(req: CalculateRequest, fetchFn: typeof fetch = fetch): Promise<CalculateResult> {
  let res: Response;
  try {
    res = await fetchFn("/api/calculate", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(req),
      credentials: "same-origin",
    });
  } catch {
    return { ok: false, code: "NETWORK", message: "Cannot reach the server. Please try again." };
  }

  const body: unknown = await res.json().catch(() => null);

  if (res.ok) {
    const parsed = quoteSchema.safeParse(body);
    return parsed.success
      ? { ok: true, quote: parsed.data }
      : { ok: false, code: "BAD_RESPONSE", message: "Unexpected response from the server." };
  }

  const apiError = apiErrorSchema.safeParse(body);
  return apiError.success
    ? { ok: false, code: apiError.data.error.code, message: apiError.data.error.message }
    : { ok: false, code: `HTTP_${res.status}`, message: "Something went wrong. Please try again." };
}

/** Called from a Server Component with the internal API address. */
export async function fetchProducts(baseUrl: string, fetchFn: typeof fetch = fetch): Promise<Product[]> {
  const res = await fetchFn(`${baseUrl}/api/products`, { next: { revalidate: 60 } });
  if (!res.ok) throw new Error(`Could not load products (HTTP ${res.status})`);
  return z.array(productSchema).parse(await res.json());
}
```

- [ ] **Step 4: Run and watch it pass**

Run: `cd frontend && npm test && npm run typecheck`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/comnarin/Documents/Stelligence/Code-homework
git add frontend
git commit -m "feat(frontend): add typed API client validated with Zod" -m "Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>"
```

---

## Task 15: Calculator state reducer and hook

**Files:**
- Create: `frontend/src/features/calculator/state.ts`, `useCalculator.ts`
- Test: `frontend/src/features/calculator/state.test.ts`

**Interfaces:**
- Consumes: `Quote`, `CalculateFn`, `CalculateRequest` (Task 14).
- Produces:
```ts
export const MAX_QUANTITY = 1000;
export type State = { quantities: Record<string, number>; memberCard: string; status: "idle" | "loading" | "result" | "error"; quote: Quote | null; errorMessage: string | null };
export type Action = { type: "increment"; productId: string } | { type: "decrement"; productId: string } | { type: "setMemberCard"; value: string } | { type: "submit" } | { type: "succeeded"; quote: Quote } | { type: "failed"; message: string } | { type: "recalculate" };
export const initialState: State;
export function calculatorReducer(state: State, action: Action): State;
export function buildRequest(state: State): CalculateRequest;   // only quantity > 0, trims card, omits blank card
export function hasItems(state: State): boolean;
export function useCalculator(calculate: CalculateFn): { state: State; increment(id: string): void; decrement(id: string): void; setMemberCard(v: string): void; submit(): Promise<void>; recalculate(): void };
```

- [ ] **Step 1: Write the failing reducer tests**

`frontend/src/features/calculator/state.test.ts`:
```ts
import { describe, expect, it } from "vitest";
import type { Quote } from "./api";
import { buildRequest, calculatorReducer, hasItems, initialState, MAX_QUANTITY, type State } from "./state";

const quote: Quote = { totalBeforeSatang: 100, itemDiscounts: [], memberDiscountSatang: 0, finalTotalSatang: 100 };
const withQty = (quantities: Record<string, number>, extra: Partial<State> = {}): State => ({
  ...initialState,
  quantities,
  ...extra,
});

describe("calculatorReducer", () => {
  it("increments a product from zero", () => {
    const s = calculatorReducer(initialState, { type: "increment", productId: "pink" });
    expect(s.quantities).toEqual({ pink: 1 });
  });

  it("never goes below zero", () => {
    const s = calculatorReducer(initialState, { type: "decrement", productId: "pink" });
    expect(s.quantities.pink ?? 0).toBe(0);
  });

  it("decrements down to zero", () => {
    const s = calculatorReducer(withQty({ pink: 1 }), { type: "decrement", productId: "pink" });
    expect(s.quantities.pink).toBe(0);
  });

  it("caps quantity at the API limit", () => {
    const s = calculatorReducer(withQty({ pink: MAX_QUANTITY }), { type: "increment", productId: "pink" });
    expect(s.quantities.pink).toBe(MAX_QUANTITY);
  });

  it("stores the member card as typed", () => {
    const s = calculatorReducer(initialState, { type: "setMemberCard", value: " MEM0001" });
    expect(s.memberCard).toBe(" MEM0001");
  });

  it("moves idle -> loading -> result", () => {
    const loading = calculatorReducer(initialState, { type: "submit" });
    expect(loading.status).toBe("loading");
    const done = calculatorReducer(loading, { type: "succeeded", quote });
    expect(done).toMatchObject({ status: "result", quote });
  });

  it("keeps the cart and card when a request fails", () => {
    const start = withQty({ red: 1 }, { memberCard: "MEM0001", status: "loading" });
    const s = calculatorReducer(start, { type: "failed", message: "Red set is reserved" });
    expect(s).toMatchObject({ status: "error", errorMessage: "Red set is reserved", memberCard: "MEM0001" });
    expect(s.quantities).toEqual({ red: 1 });
  });

  it("clears the previous error when submitting again", () => {
    const s = calculatorReducer(withQty({ red: 1 }, { status: "error", errorMessage: "boom" }), { type: "submit" });
    expect(s).toMatchObject({ status: "loading", errorMessage: null });
  });

  it("recalculate returns to the form and keeps the inputs", () => {
    const s = calculatorReducer(withQty({ pink: 2 }, { status: "result", quote, memberCard: "MEM0001" }), {
      type: "recalculate",
    });
    expect(s).toMatchObject({ status: "idle", quote: null, memberCard: "MEM0001" });
    expect(s.quantities).toEqual({ pink: 2 });
  });
});

describe("buildRequest", () => {
  it("sends only products with quantity above zero", () => {
    const req = buildRequest(withQty({ pink: 2, blue: 0, red: 1 }));
    expect(req.items).toEqual([
      { productId: "pink", quantity: 2 },
      { productId: "red", quantity: 1 },
    ]);
  });

  it("trims the card and omits a blank one", () => {
    expect(buildRequest(withQty({ pink: 1 }, { memberCard: "  MEM0001 " })).memberCard).toBe("MEM0001");
    expect("memberCard" in buildRequest(withQty({ pink: 1 }, { memberCard: "   " }))).toBe(false);
  });
});

describe("hasItems", () => {
  it("is false for an empty or all-zero cart", () => {
    expect(hasItems(initialState)).toBe(false);
    expect(hasItems(withQty({ pink: 0 }))).toBe(false);
  });

  it("is true when any product has a quantity", () => {
    expect(hasItems(withQty({ pink: 1 }))).toBe(true);
  });
});
```

- [ ] **Step 2: Run and watch it fail**

Run: `cd frontend && npm test -- state`
Expected: FAIL, cannot resolve `./state`.

- [ ] **Step 3: Implement the reducer and the hook**

`frontend/src/features/calculator/state.ts`:
```ts
import type { CalculateRequest, Quote } from "./api";

/** Must match the API's MaxQuantityPerLine. */
export const MAX_QUANTITY = 1000;

export type State = {
  quantities: Record<string, number>;
  memberCard: string;
  status: "idle" | "loading" | "result" | "error";
  quote: Quote | null;
  errorMessage: string | null;
};

export type Action =
  | { type: "increment"; productId: string }
  | { type: "decrement"; productId: string }
  | { type: "setMemberCard"; value: string }
  | { type: "submit" }
  | { type: "succeeded"; quote: Quote }
  | { type: "failed"; message: string }
  | { type: "recalculate" };

export const initialState: State = {
  quantities: {},
  memberCard: "",
  status: "idle",
  quote: null,
  errorMessage: null,
};

export function calculatorReducer(state: State, action: Action): State {
  switch (action.type) {
    case "increment": {
      const next = Math.min((state.quantities[action.productId] ?? 0) + 1, MAX_QUANTITY);
      return { ...state, quantities: { ...state.quantities, [action.productId]: next } };
    }
    case "decrement": {
      const next = Math.max((state.quantities[action.productId] ?? 0) - 1, 0);
      return { ...state, quantities: { ...state.quantities, [action.productId]: next } };
    }
    case "setMemberCard":
      return { ...state, memberCard: action.value };
    case "submit":
      return { ...state, status: "loading", errorMessage: null };
    case "succeeded":
      return { ...state, status: "result", quote: action.quote, errorMessage: null };
    case "failed":
      return { ...state, status: "error", errorMessage: action.message };
    case "recalculate":
      return { ...state, status: "idle", quote: null, errorMessage: null };
  }
}

export function buildRequest(state: State): CalculateRequest {
  const items = Object.entries(state.quantities)
    .filter(([, quantity]) => quantity > 0)
    .map(([productId, quantity]) => ({ productId, quantity }));
  const memberCard = state.memberCard.trim();
  return memberCard ? { memberCard, items } : { items };
}

export function hasItems(state: State): boolean {
  return Object.values(state.quantities).some((q) => q > 0);
}
```

`frontend/src/features/calculator/useCalculator.ts`:
```ts
"use client";

import { useCallback, useReducer } from "react";
import type { CalculateFn } from "./api";
import { buildRequest, calculatorReducer, initialState } from "./state";

export function useCalculator(calculate: CalculateFn) {
  const [state, dispatch] = useReducer(calculatorReducer, initialState);

  const submit = useCallback(async () => {
    dispatch({ type: "submit" });
    const result = await calculate(buildRequest(state));
    dispatch(result.ok ? { type: "succeeded", quote: result.quote } : { type: "failed", message: result.message });
  }, [calculate, state]);

  return {
    state,
    increment: (productId: string) => dispatch({ type: "increment", productId }),
    decrement: (productId: string) => dispatch({ type: "decrement", productId }),
    setMemberCard: (value: string) => dispatch({ type: "setMemberCard", value }),
    submit,
    recalculate: () => dispatch({ type: "recalculate" }),
  };
}
```

- [ ] **Step 4: Run and watch it pass**

Run: `cd frontend && npm test && npm run typecheck`
Expected: PASS. The hook is exercised through the component tests in Task 16.

- [ ] **Step 5: Commit**

```bash
cd /Users/comnarin/Documents/Stelligence/Code-homework
git add frontend
git commit -m "feat(frontend): add calculator reducer and useCalculator hook" -m "Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>"
```

---

## Task 16: UI components, `Calculator` island and page

**Files:**
- Create: `frontend/src/features/calculator/components/ProductRow.tsx`, `ResultSummary.tsx`, `Calculator.tsx`, `frontend/src/app/page.tsx`, `frontend/src/app/error.tsx`
- Modify: `frontend/src/app/layout.tsx` (title), `frontend/next.config.ts`
- Test: `components/ProductRow.test.tsx`, `components/ResultSummary.test.tsx`, `components/Calculator.test.tsx`

**Interfaces:**
- Consumes: `formatTHB`, `Product`, `Quote`, `CalculateFn`, `calculateOrder`, `useCalculator`, `hasItems`, `API_URL`, `fetchProducts`.
- Produces: `ProductRow({product, quantity, onIncrement, onDecrement})`, `ResultSummary({quote, onRecalculate})`, `Calculator({products, calculate?})`.

- [ ] **Step 1: Write the failing component tests**

`frontend/src/features/calculator/components/ProductRow.test.tsx`:
```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ProductRow } from "./ProductRow";

const pink = { id: "pink", name: "Pink set", priceSatang: 8000 };

describe("ProductRow", () => {
  it("shows the name, price and quantity", () => {
    render(<ProductRow product={pink} quantity={3} onIncrement={vi.fn()} onDecrement={vi.fn()} />);
    expect(screen.getByText("Pink set")).toBeInTheDocument();
    expect(screen.getByText(/80\.00/)).toBeInTheDocument();
    expect(screen.getByLabelText("Pink set quantity")).toHaveTextContent("3");
  });

  it("calls the handlers from the labelled buttons", async () => {
    const onIncrement = vi.fn();
    const onDecrement = vi.fn();
    render(<ProductRow product={pink} quantity={1} onIncrement={onIncrement} onDecrement={onDecrement} />);

    await userEvent.click(screen.getByRole("button", { name: "Increase Pink set" }));
    await userEvent.click(screen.getByRole("button", { name: "Decrease Pink set" }));

    expect(onIncrement).toHaveBeenCalledTimes(1);
    expect(onDecrement).toHaveBeenCalledTimes(1);
  });

  it("disables decrease at zero", () => {
    render(<ProductRow product={pink} quantity={0} onIncrement={vi.fn()} onDecrement={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Decrease Pink set" })).toBeDisabled();
  });
});
```

`frontend/src/features/calculator/components/ResultSummary.test.tsx`:
```tsx
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Quote } from "../api";
import { ResultSummary } from "./ResultSummary";

const quote: Quote = {
  totalBeforeSatang: 24000,
  itemDiscounts: [{ ruleCode: "PAIR_5", productId: "orange", name: "Orange set", bundles: 1, amountSatang: 1200 }],
  memberDiscountSatang: 2280,
  finalTotalSatang: 20520,
};

describe("ResultSummary", () => {
  it("shows total, item discounts, member discount and final total", () => {
    render(<ResultSummary quote={quote} onRecalculate={vi.fn()} />);

    expect(screen.getByText("Total (before discount)").parentElement).toHaveTextContent(/240\.00/);
    const items = screen.getByRole("region", { name: "Discounted items" });
    expect(within(items).getByText("Orange set")).toBeInTheDocument();
    expect(within(items).getByText(/-.*12\.00/)).toBeInTheDocument();
    expect(screen.getByText("Discount from member card").parentElement).toHaveTextContent(/-.*22\.80/);
    expect(screen.getByText("Final total").parentElement).toHaveTextContent(/205\.20/);
  });

  it("says so when no item got a discount", () => {
    render(<ResultSummary quote={{ ...quote, itemDiscounts: [], memberDiscountSatang: 0 }} onRecalculate={vi.fn()} />);
    expect(screen.getByText(/no item discounts/i)).toBeInTheDocument();
  });

  it("recalculates on click", async () => {
    const onRecalculate = vi.fn();
    render(<ResultSummary quote={quote} onRecalculate={onRecalculate} />);
    await userEvent.click(screen.getByRole("button", { name: "Recalculate" }));
    expect(onRecalculate).toHaveBeenCalledTimes(1);
  });
});
```

`frontend/src/features/calculator/components/Calculator.test.tsx`:
```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { CalculateResult } from "../api";
import { Calculator } from "./Calculator";

const products = [
  { id: "red", name: "Red set", priceSatang: 5000 },
  { id: "pink", name: "Pink set", priceSatang: 8000 },
];

const okResult: CalculateResult = {
  ok: true,
  quote: { totalBeforeSatang: 16000, itemDiscounts: [], memberDiscountSatang: 1600, finalTotalSatang: 14400 },
};

describe("Calculator", () => {
  it("disables Calculate until something is in the cart", async () => {
    render(<Calculator products={products} calculate={vi.fn()} />);
    const button = screen.getByRole("button", { name: "Calculate" });
    expect(button).toBeDisabled();

    await userEvent.click(screen.getByRole("button", { name: "Increase Pink set" }));

    expect(button).toBeEnabled();
  });

  it("sends only chosen items and the trimmed card, then shows the result", async () => {
    const calculate = vi.fn().mockResolvedValue(okResult);
    render(<Calculator products={products} calculate={calculate} />);

    await userEvent.click(screen.getByRole("button", { name: "Increase Pink set" }));
    await userEvent.click(screen.getByRole("button", { name: "Increase Pink set" }));
    await userEvent.type(screen.getByLabelText("Card Number"), "  MEM0001 ");
    await userEvent.click(screen.getByRole("button", { name: "Calculate" }));

    expect(calculate).toHaveBeenCalledWith({ memberCard: "MEM0001", items: [{ productId: "pink", quantity: 2 }] });
    expect(await screen.findByText("Final total")).toBeInTheDocument();
    expect(screen.getByText("Final total").parentElement).toHaveTextContent(/144\.00/);
  });

  it("Recalculate goes back to the form with the cart intact", async () => {
    render(<Calculator products={products} calculate={vi.fn().mockResolvedValue(okResult)} />);
    await userEvent.click(screen.getByRole("button", { name: "Increase Pink set" }));
    await userEvent.click(screen.getByRole("button", { name: "Calculate" }));

    await userEvent.click(await screen.findByRole("button", { name: "Recalculate" }));

    expect(screen.getByLabelText("Pink set quantity")).toHaveTextContent("1");
  });

  it("shows the server message and keeps the cart when the Red set is held", async () => {
    const calculate = vi.fn().mockResolvedValue({
      ok: false,
      code: "LIMITED_PRODUCT_UNAVAILABLE",
      message: "Red set is reserved by another customer right now. Please try again later.",
    });
    render(<Calculator products={products} calculate={calculate} />);
    await userEvent.click(screen.getByRole("button", { name: "Increase Red set" }));

    await userEvent.click(screen.getByRole("button", { name: "Calculate" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Red set is reserved by another customer");
    expect(screen.getByLabelText("Red set quantity")).toHaveTextContent("1");
  });

  it("shows a retryable message when the network fails and lets the user retry", async () => {
    const calculate = vi
      .fn()
      .mockResolvedValueOnce({ ok: false, code: "NETWORK", message: "Cannot reach the server. Please try again." })
      .mockResolvedValueOnce(okResult);
    render(<Calculator products={products} calculate={calculate} />);
    await userEvent.click(screen.getByRole("button", { name: "Increase Pink set" }));

    await userEvent.click(screen.getByRole("button", { name: "Calculate" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/try again/i);

    await userEvent.click(screen.getByRole("button", { name: "Calculate" }));
    expect(await screen.findByText("Final total")).toBeInTheDocument();
  });

  it("prevents double submission while a request is in flight", async () => {
    let finish: (r: CalculateResult) => void = () => {};
    const calculate = vi.fn().mockReturnValue(new Promise<CalculateResult>((r) => (finish = r)));
    render(<Calculator products={products} calculate={calculate} />);
    await userEvent.click(screen.getByRole("button", { name: "Increase Pink set" }));

    await userEvent.click(screen.getByRole("button", { name: "Calculate" }));

    expect(screen.getByRole("button", { name: /calculating/i })).toBeDisabled();
    expect(calculate).toHaveBeenCalledTimes(1);
    finish(okResult);
    expect(await screen.findByText("Final total")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run and watch them fail**

Run: `cd frontend && npm test -- components`
Expected: FAIL, cannot resolve `./ProductRow`, `./ResultSummary`, `./Calculator`.

- [ ] **Step 3: Implement the components**

`frontend/src/features/calculator/components/ProductRow.tsx`:
```tsx
import { formatTHB } from "@/lib/money";
import type { Product } from "../api";

type Props = {
  product: Product;
  quantity: number;
  onIncrement: () => void;
  onDecrement: () => void;
};

const stepper =
  "flex h-8 w-8 items-center justify-center rounded-full bg-sky-500 text-white transition hover:bg-sky-600 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-sky-700 disabled:cursor-not-allowed disabled:bg-sky-200";

export function ProductRow({ product, quantity, onIncrement, onDecrement }: Props) {
  return (
    <li className="flex items-center justify-between rounded-2xl bg-white p-4 shadow-sm">
      <div>
        <p className="font-medium text-slate-900">{product.name}</p>
        <p className="text-sm text-slate-600">{formatTHB(product.priceSatang)}</p>
      </div>
      <div className="flex items-center gap-3">
        <button type="button" className={stepper} aria-label={`Decrease ${product.name}`} disabled={quantity === 0} onClick={onDecrement}>
          −
        </button>
        <output aria-label={`${product.name} quantity`} className="w-8 text-center tabular-nums">
          {quantity}
        </output>
        <button type="button" className={stepper} aria-label={`Increase ${product.name}`} onClick={onIncrement}>
          +
        </button>
      </div>
    </li>
  );
}
```

`frontend/src/features/calculator/components/ResultSummary.tsx`:
```tsx
import { formatTHB } from "@/lib/money";
import type { Quote } from "../api";

type Props = { quote: Quote; onRecalculate: () => void };

const card = "rounded-2xl bg-white p-4 shadow-sm";

export function ResultSummary({ quote, onRecalculate }: Props) {
  return (
    <div className="flex flex-col gap-4">
      <h2 className="text-center text-2xl font-bold">Calculate Total</h2>

      <section className={card}>
        <p className="text-lg">Total (before discount)</p>
        <p className="font-bold">{formatTHB(quote.totalBeforeSatang)}</p>
      </section>

      <section className={card} role="region" aria-label="Discounted items">
        <h3 className="text-lg">Discounted Items</h3>
        {quote.itemDiscounts.length === 0 ? (
          <p className="text-sm text-slate-600">No item discounts</p>
        ) : (
          <ul>
            {quote.itemDiscounts.map((d) => (
              <li key={`${d.ruleCode}-${d.productId}`} className="flex justify-between text-sm">
                <span>{d.name}</span>
                <span>-{formatTHB(d.amountSatang)}</span>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className={card}>
        <p className="text-lg">Discount from member card</p>
        <p className="font-bold">-{formatTHB(quote.memberDiscountSatang)}</p>
      </section>

      <section className={card}>
        <p className="text-lg font-bold">Final total</p>
        <p className="font-bold">{formatTHB(quote.finalTotalSatang)}</p>
      </section>

      <button
        type="button"
        onClick={onRecalculate}
        className="mx-auto rounded-full bg-sky-500 px-6 py-2 text-white transition hover:bg-sky-600 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-sky-700"
      >
        Recalculate
      </button>
    </div>
  );
}
```

`frontend/src/features/calculator/components/Calculator.tsx`:
```tsx
"use client";

import { calculateOrder, type CalculateFn, type Product } from "../api";
import { hasItems } from "../state";
import { useCalculator } from "../useCalculator";
import { ProductRow } from "./ProductRow";
import { ResultSummary } from "./ResultSummary";

type Props = { products: Product[]; calculate?: CalculateFn };

export function Calculator({ products, calculate = calculateOrder }: Props) {
  const { state, increment, decrement, setMemberCard, submit, recalculate } = useCalculator(calculate);

  if (state.status === "result" && state.quote) {
    return <ResultSummary quote={state.quote} onRecalculate={recalculate} />;
  }

  const loading = state.status === "loading";
  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault();
        void submit();
      }}
    >
      <ul className="flex flex-col gap-3">
        {products.map((p) => (
          <ProductRow
            key={p.id}
            product={p}
            quantity={state.quantities[p.id] ?? 0}
            onIncrement={() => increment(p.id)}
            onDecrement={() => decrement(p.id)}
          />
        ))}
      </ul>

      <div className="flex flex-col gap-1">
        <label htmlFor="member-card" className="text-sm font-medium">
          Card Number
        </label>
        <input
          id="member-card"
          value={state.memberCard}
          onChange={(e) => setMemberCard(e.target.value)}
          autoComplete="off"
          className="rounded-full bg-white px-4 py-3 shadow-sm focus-visible:outline focus-visible:outline-2 focus-visible:outline-sky-600"
        />
      </div>

      {state.errorMessage && (
        <p role="alert" className="rounded-xl bg-red-50 p-3 text-sm text-red-800">
          {state.errorMessage}
        </p>
      )}

      <button
        type="submit"
        disabled={!hasItems(state) || loading}
        className="rounded-full bg-sky-500 py-3 text-white transition hover:bg-sky-600 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-sky-700 disabled:cursor-not-allowed disabled:bg-sky-200"
      >
        {loading ? "Calculating…" : "Calculate"}
      </button>
    </form>
  );
}
```

- [ ] **Step 4: Run and watch them pass**

Run: `cd frontend && npm test && npm run typecheck && npm run lint`
Expected: PASS. If `getByText("Final total").parentElement` assertions fail because of markup, adjust the component markup, not the assertions.

- [ ] **Step 5: Add the page, error boundary, layout title and rewrite**

`frontend/src/app/page.tsx`:
```tsx
import { fetchProducts } from "@/features/calculator/api";
import { Calculator } from "@/features/calculator/components/Calculator";
import { API_URL } from "@/lib/env";

// The API is not reachable at build time (Docker), so render per request.
// The product fetch itself is cached for 60 seconds inside fetchProducts.
export const dynamic = "force-dynamic";

export default async function Page() {
  const products = await fetchProducts(API_URL);
  return (
    <main className="mx-auto flex min-h-screen max-w-md flex-col gap-6 bg-slate-50 p-4">
      <h1 className="text-xl font-bold">Food Store Calculator</h1>
      <Calculator products={products} />
    </main>
  );
}
```

`frontend/src/app/error.tsx`:
```tsx
"use client";

export default function Error({ reset }: { error: Error; reset: () => void }) {
  return (
    <main className="mx-auto flex min-h-screen max-w-md flex-col items-center justify-center gap-4 p-4 text-center">
      <h1 className="text-xl font-bold">We could not load the menu</h1>
      <p className="text-slate-600">The store service is not responding. Please try again.</p>
      <button type="button" onClick={reset} className="rounded-full bg-sky-500 px-6 py-2 text-white hover:bg-sky-600">
        Try again
      </button>
    </main>
  );
}
```

In `frontend/src/app/layout.tsx`, set `export const metadata = { title: "Food Store Calculator", description: "Price calculator for the food store" };`.

`frontend/next.config.ts`:
```ts
import type { NextConfig } from "next";

// Evaluated at build time and baked into the standalone output, so API_URL must
// be present as a build argument in Docker (see docker-compose.yml).
const apiUrl = process.env.API_URL ?? "http://localhost:8080";

const nextConfig: NextConfig = {
  output: "standalone",
  async rewrites() {
    return [{ source: "/api/:path*", destination: `${apiUrl}/api/:path*` }];
  },
};

export default nextConfig;
```

- [ ] **Step 6: Verify the build**

Run: `cd frontend && npm run build`
Expected: build succeeds (page is dynamic, so the API is not needed).

- [ ] **Step 7: Commit**

```bash
cd /Users/comnarin/Documents/Stelligence/Code-homework
git add frontend
git commit -m "feat(frontend): add calculator UI, page, error boundary and API rewrite" -m "Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>"
```

---

## Task 17: Frontend Docker image, Compose, README and end-to-end smoke test

**Files:**
- Create: `frontend/Dockerfile`, `frontend/.dockerignore`, `docker-compose.yml`, `README.md`
- Modify: root `.gitignore` if needed

**Interfaces:**
- Compose services: `postgres` (host port 55432), `redis`, `api` (8080), `web` (3000). `web` receives `API_URL=http://api:8080` as both a build arg and a runtime env.

- [ ] **Step 1: Frontend Dockerfile**

`frontend/Dockerfile`:
```dockerfile
FROM node:22-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci

FROM node:22-alpine AS build
WORKDIR /app
ARG API_URL=http://api:8080
ENV API_URL=$API_URL
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN npm run build

FROM node:22-alpine AS run
WORKDIR /app
ENV NODE_ENV=production PORT=3000 HOSTNAME=0.0.0.0
ARG API_URL=http://api:8080
ENV API_URL=$API_URL
RUN adduser -D -u 10001 app
COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static ./.next/static
COPY --from=build /app/public ./public
USER app
EXPOSE 3000
CMD ["node", "server.js"]
```

`frontend/.dockerignore`:
```
node_modules
.next
```

- [ ] **Step 2: Compose file**

`docker-compose.yml`:
```yaml
services:
  postgres:
    image: postgres:17-alpine
    environment:
      POSTGRES_PASSWORD: pw
      POSTGRES_DB: foodstore
    ports:
      - "55432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres -d foodstore"]
      interval: 3s
      timeout: 3s
      retries: 20

  redis:
    image: redis:7-alpine
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 3s
      timeout: 3s
      retries: 20

  api:
    build: ./backend
    environment:
      DATABASE_URL: postgres://postgres:pw@postgres:5432/foodstore?sslmode=disable
      REDIS_ADDR: redis:6379
      PORT: "8080"
    ports:
      - "8080:8080"
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/healthz"]
      interval: 3s
      timeout: 3s
      retries: 20

  web:
    build:
      context: ./frontend
      args:
        API_URL: http://api:8080
    environment:
      API_URL: http://api:8080
    ports:
      - "3000:3000"
    depends_on:
      api:
        condition: service_healthy
```

- [ ] **Step 3: README**

`README.md` must contain, in this order: (1) one-paragraph description; (2) prerequisites (Docker; Go 1.26 and Node 22+ only for local development); (3) **Run with Docker**: `docker compose up --build`, then open `http://localhost:3000`, API at `http://localhost:8080`; (4) demo member cards `MEM0001` (active) and `MEM0002` (suspended); (5) **How to try the Red set rule** (open the app in a normal window, order Red; open a private window, order Red -> blocked; the first window can recalculate); (6) **Run the tests**: backend `cd backend && go test -race -cover ./...` (set `DATABASE_URL=postgres://postgres:pw@localhost:55432/foodstore?sslmode=disable` with `docker compose up -d postgres` to include the Postgres integration tests), frontend `cd frontend && npm test`; (7) **API** table for `GET /healthz`, `GET /api/products`, `POST /api/calculate` with a sample request, sample response and the error codes; (8) **Project structure** (the tree from the spec, section 4) and the one-line dependency rule; (9) link to `docs/INTERVIEW.md` and the spec.

- [ ] **Step 4: Bring the whole stack up and smoke test**

```bash
docker rm -f foodstore-test-pg foodstore-test-redis 2>/dev/null
cd /Users/comnarin/Documents/Stelligence/Code-homework
docker compose up --build -d
docker compose ps
curl -s localhost:3000/api/products | head -c 150; echo
curl -s -c /tmp/a.jar -b /tmp/a.jar -X POST localhost:3000/api/calculate -H 'Content-Type: application/json' -d '{"memberCard":"MEM0001","items":[{"productId":"orange","quantity":2}]}'; echo
curl -s -c /tmp/a.jar -b /tmp/a.jar -X POST localhost:3000/api/calculate -H 'Content-Type: application/json' -d '{"items":[{"productId":"red","quantity":1}]}'; echo
curl -s -c /tmp/b.jar -X POST localhost:3000/api/calculate -H 'Content-Type: application/json' -d '{"items":[{"productId":"red","quantity":1}]}'; echo
curl -s -o /dev/null -w "page: %{http_code}\n" localhost:3000/
```
Expected: all four containers `healthy`/running; products JSON through the Next proxy; first calculate `finalTotalSatang":20520`; jar A's Red order 200; jar B's Red order 409 `LIMITED_PRODUCT_UNAVAILABLE`; page 200. Then open `http://localhost:3000` in a browser and repeat the Orange x2 + `MEM0001` flow by hand; confirm the result screen shows -฿12.00 and -฿22.80 and a final total of ฿205.20.

- [ ] **Step 5: Run every automated check**

```bash
cd backend && DATABASE_URL='postgres://postgres:pw@localhost:55432/foodstore?sslmode=disable' go test -race -cover ./... && go vet ./...
cd ../frontend && npm test && npm run lint && npm run typecheck
```
Expected: all green. Record the backend coverage numbers for `docs/INTERVIEW.md`.

- [ ] **Step 6: Commit**

```bash
cd /Users/comnarin/Documents/Stelligence/Code-homework
docker compose down
git add .
git commit -m "feat: add Docker images, Compose stack and README" -m "Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>"
```

---

## Task 18: Interview walkthrough (`docs/INTERVIEW.md`)

**Files:**
- Create: `docs/INTERVIEW.md`

This is a teaching document for the project owner, written after the code exists so every statement is checked against the real files. Use real file paths and short real code excerpts. Required sections:

1. **The 60-second pitch**: what was built, the stack, why these choices (Clean Architecture, Redis for the limit, Postgres for data-driven rules).
2. **Request walkthrough**: trace `POST /api/calculate` end to end: browser -> Next rewrite -> `cidMiddleware` -> handler -> `CalculateOrder.Execute` (each numbered step with the file and function) -> Postgres/Redis -> response -> UI. Include a diagram in ASCII.
3. **Clean Architecture explained on this codebase**: the four layers, the dependency rule, where ports live and why (consumer-side), what you would change if Postgres were replaced by DynamoDB (answer: one adapter).
4. **Key decisions and trade-offs** (one short paragraph each): integer satang money; basis points; rules as data plus a Go registry (what is data vs code); atomic Lua claim vs check-then-set; claim happens after validation; slot holder can re-claim; `cid` cookie vs client UUID vs auth; Server Component + client island; rewrite proxy and cookies; Zod at the boundary; why `calculate` is not a Server Action.
5. **TDD story**: the order tests were written, one example of red -> green -> refactor from the git log (`git log --oneline`), the mutation checks in Task 7, the concurrency test.
6. **Known limitations and how to fix them in production** (the list from the spec section 3, plus: rules loaded per request, order IDs, no rate limiting, claim at checkout, idempotency keys, observability).
7. **Likely interview questions with model answers** (each 3-6 sentences, answered in first person), at minimum:
   - Why Clean Architecture for a small app? When is it overkill?
   - How do you guarantee only one customer gets the Red set under concurrent requests?
   - Why Lua instead of `SETNX` plus `GET`? What if Redis restarts and loses the key?
   - Why integers for money? How do you round and why half-up?
   - How would you add a "buy 3 get 1 free" promotion? A new limited product?
   - How would you scale this to multiple API instances? Where is the state?
   - How do you test code that depends on time, randomness, Redis, Postgres?
   - What are the security issues with the anonymous cookie? How would you add real auth?
   - Why Server Components here, and what runs on the client?
   - What would you monitor in production? What breaks first under load?
   - What would you do with another week?
   - How do you mentor a junior on this codebase / what would you check in a code review of it?
8. **Cheat sheet**: ten bullets to remember before the interview, plus the commands to run the project and tests.

- [ ] **Step 1: Write the document** from the real code (open each referenced file first; quote only what exists).
- [ ] **Step 2: Verify every file path and command in the document exists or runs.** Run the commands listed in the cheat sheet.
- [ ] **Step 3: Commit**

```bash
cd /Users/comnarin/Documents/Stelligence/Code-homework
git add docs/INTERVIEW.md
git commit -m "docs: add interview walkthrough" -m "Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>"
```

---

## Self-Review

**Spec coverage**
- Spec 2 requirements: pair discount (Tasks 3-4), member discount (Task 4, 6), Red limit (Tasks 6-8), product list and calculation endpoints (Task 11), unit tests (every task), UI list with add/remove, card input, result breakdown (Task 16).
- Spec 3 decisions D1-D10: D1 Task 4 test "member 10% applies after pair discount"; D2 Tasks 7, 10, 11; D3 Tasks 6, 8; D4 Tasks 6, 8; D5 Task 11; D6 Task 1; D7 Tasks 9-10; D8 Tasks 6, 10; D9 Task 16; D10 Task 10 (per-request load).
- Spec 4 layout and dependency rule: Task layout matches; `platform/config`, `platform/db`, `cmd/server`, `migrations`.
- Spec 5-8: pricing, rules mapping, limiter, API, DB schema and seed are all tasks. Schema additions beyond the spec: `products.sort_order` (menu display order) and `order_discounts.id`; both are noted here and harmless.
- Spec 9 frontend: Server Component page, client island, rewrite, Zod, feature folders, reducer, accessibility labels, error boundary (Task 16). `aria-live` is implemented with `role="alert"`. `loading.tsx` from the spec's tree is not added: the page is a single dynamic fetch and `error.tsx` covers failure; this is an intentional YAGNI cut.
- Spec 10 / 10.1 TDD: every task is test-first; mutation checks in Task 7; `go test -race` and a concurrency test in Task 8; coverage reported in Task 17. The spec says the Red 409 UI text includes "for up to 1 hour"; the implemented message is "<Product> is reserved by another customer right now. Please try again later." because the limit window is data, not constant. The spec text should be updated to match when this plan is committed.
- Spec 11 delivery: Task 17 (README, Compose), Task 18 (INTERVIEW.md).

**Placeholder scan:** no TBD/TODO. Task 17 Step 3 and Task 18 give required content lists for prose documents rather than full text; all code is complete.

**Type consistency:** `domain.Line{Product, Quantity}`, `Quote.Discounts`/`ItemDiscounts()`/`MemberDiscount()`, `usecase.CalculateInput`/`ItemInput`, `SlotLimiter.Claim(ctx, productID, ownerID, ttl)`, `NewCalculateOrder(CalculateOrderDeps)` and the frontend `CalculateFn`/`CalculateResult` are used identically across tasks.
