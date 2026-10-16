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
