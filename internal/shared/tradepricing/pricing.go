package tradepricing

import "github.com/shopspring/decimal"

// PricingCost derives a fresh pricing basis from a raw upstream quote, never from a previous basis.
func PricingCost(upstream decimal.Decimal) decimal.Decimal {
	return upstream.Mul(decimal.RequireFromString("1.01")).RoundCeil(2)
}

type Override struct {
	ProductID, SKUID uint
	Amount, Rate     string
}
type Snapshot struct {
	CostMarkup   *string
	DiscountRate string
	Overrides    []Override
}

// Resolve mirrors membership precedence: SKU override, product override, cost markup, retail discount.
func Resolve(snapshot *Snapshot, productID, skuID uint, retail, cost decimal.Decimal) decimal.Decimal {
	if snapshot == nil {
		return retail
	}
	resolve := func(rule Override) (decimal.Decimal, bool) {
		if amount, e := decimal.NewFromString(rule.Amount); e == nil && amount.IsPositive() {
			return amount.Round(2), true
		}
		if rate, e := decimal.NewFromString(rule.Rate); e == nil && rate.IsPositive() {
			return retail.Mul(rate).Div(decimal.NewFromInt(100)).Round(2), true
		}
		return decimal.Zero, false
	}
	for _, id := range []uint{skuID, 0} {
		for _, rule := range snapshot.Overrides {
			if rule.ProductID == productID && rule.SKUID == id {
				if value, ok := resolve(rule); ok {
					return decimal.Min(retail, value)
				}
			}
		}
	}
	if snapshot.CostMarkup != nil && cost.IsPositive() {
		if pct, e := decimal.NewFromString(*snapshot.CostMarkup); e == nil && !pct.IsNegative() {
			return decimal.Min(retail, cost.Mul(decimal.NewFromInt(100).Add(pct)).Div(decimal.NewFromInt(100)).RoundCeil(2))
		}
	}
	if rate, e := decimal.NewFromString(snapshot.DiscountRate); e == nil && rate.IsPositive() && rate.LessThan(decimal.NewFromInt(100)) {
		return retail.Mul(rate).Div(decimal.NewFromInt(100)).Round(2)
	}
	return retail
}
