package analytics

import (
	"strings"
	"testing"
	"time"
)

func TestBuildExpenseWhereUsesBaseAmountExpression(t *testing.T) {
	from := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)

	where, args, amountExpr := buildExpenseWhere("fam-1", from, to, "USD", true, []string{"cat-1"})

	if amountExpr != "COALESCE(e.amount_in_base, e.amount)" {
		t.Fatalf("expected base amount expression, got %q", amountExpr)
	}
	if !strings.Contains(where, "e.base_currency = ?") {
		t.Fatalf("expected base_currency condition, got %q", where)
	}
	if !strings.Contains(where, "e.amount_in_base IS NOT NULL") {
		t.Fatalf("expected amount_in_base condition, got %q", where)
	}
	if len(args) != 6 {
		t.Fatalf("expected 6 args, got %d", len(args))
	}
}

func TestBuildExpenseWhereUsesOriginalAmountWithExplicitCurrency(t *testing.T) {
	from := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)

	where, args, amountExpr := buildExpenseWhere("fam-1", from, to, "BYN", false, nil)

	if amountExpr != "e.amount" {
		t.Fatalf("expected e.amount expression, got %q", amountExpr)
	}
	if !strings.Contains(where, "e.currency = ?") {
		t.Fatalf("expected currency condition, got %q", where)
	}
	if strings.Contains(where, "e.base_currency") {
		t.Fatalf("did not expect base_currency condition, got %q", where)
	}
	if len(args) != 4 {
		t.Fatalf("expected 4 args, got %d", len(args))
	}
}

func TestBuildExpenseWhereRangeUsesBaseAmountExpression(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	where, args, amountExpr := buildExpenseWhereRange("fam-1", from, to, "USD", true, nil)

	if amountExpr != "COALESCE(e.amount_in_base, e.amount)" {
		t.Fatalf("expected base amount expression, got %q", amountExpr)
	}
	if !strings.Contains(where, "e.date < ?") {
		t.Fatalf("expected exclusive upper bound, got %q", where)
	}
	if len(args) != 5 {
		t.Fatalf("expected 5 args, got %d", len(args))
	}
}
