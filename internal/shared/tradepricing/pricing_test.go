package tradepricing

import (
	"github.com/shopspring/decimal"
	"testing"
)

func TestPricingBasisAndLowestIdentity(t *testing.T) {
	raw := decimal.RequireFromString("109.02")
	basis := PricingCost(raw)
	if basis.String() != "110.12" {
		t.Fatal(basis)
	}
	// Repeated refreshes use the same raw quote, not the previous pricing cost.
	if !PricingCost(raw).Equal(basis) {
		t.Fatal("pricing cost compounded")
	}
	markup := "5"
	snapshot := &Snapshot{CostMarkup: &markup, DiscountRate: "97"}
	price := Resolve(snapshot, 19, 1, decimal.NewFromInt(130), basis)
	if price.String() != "115.63" {
		t.Fatal(price)
	}
	snapshot.Overrides = []Override{{ProductID: 19, Amount: "114"}, {ProductID: 19, SKUID: 1, Amount: "113"}}
	if !Resolve(snapshot, 19, 1, decimal.NewFromInt(130), basis).Equal(decimal.NewFromInt(113)) {
		t.Fatal("SKU precedence lost")
	}
	if !Resolve(snapshot, 19, 0, decimal.NewFromInt(100), basis).Equal(decimal.NewFromInt(100)) {
		t.Fatal("retail cap lost")
	}
}
