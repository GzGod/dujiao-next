package application

import (
	productcontract "github.com/dujiao-next/internal/modules/catalog/product/contract"
	"strings"

	resellercontract "github.com/dujiao-next/internal/modules/reseller/contract"

	resellerdomain "github.com/dujiao-next/internal/modules/reseller/domain"

	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"

	"github.com/shopspring/decimal"
)

const (
	RuleSourceSKU     = "sku"
	RuleSourceProduct = "product"
	RuleSourceProfile = "profile"
	RuleSourceInherit = "inherit"
)

// PricingRule 描述分销改价规则解析结果。
type PricingRule struct {
	Mode              string
	Source            string
	SettingID         *uint
	MarkupPercent     decimal.Decimal
	FixedMarkupAmount decimal.Decimal
	FixedPriceAmount  decimal.Decimal
}

// ResolveUnitAmount 按 SKU → 商品 → 资料默认加价 → 继承底价的优先级解析成交单价。
func ResolveUnitAmount(profile *resellerdomain.Profile, productSetting *resellerdomain.ProductSetting, skuSetting *resellerdomain.ProductSetting, baseUnit decimal.Decimal) (decimal.Decimal, PricingRule, error) {
	if skuSetting != nil && strings.TrimSpace(skuSetting.PricingMode) != resellerdomain.PricingModeInherit {
		return ApplyPricingRule(*skuSetting, RuleSourceSKU, baseUnit)
	}
	if productSetting != nil && strings.TrimSpace(productSetting.PricingMode) != resellerdomain.PricingModeInherit {
		return ApplyPricingRule(*productSetting, RuleSourceProduct, baseUnit)
	}
	if profile != nil && profile.DefaultMarkupPercent.Decimal.GreaterThan(decimal.Zero) {
		rule := PricingRule{
			Mode:          resellerdomain.PricingModeMarkupPercent,
			Source:        RuleSourceProfile,
			MarkupPercent: profile.DefaultMarkupPercent.Decimal.Round(2),
		}
		return ApplyMarkupPercent(baseUnit, rule.MarkupPercent), rule, nil
	}
	return baseUnit.Round(2), PricingRule{Mode: resellerdomain.PricingModeInherit, Source: RuleSourceInherit}, nil
}

// ApplyPricingRule 按单条分销配置计算单价。
func ApplyPricingRule(setting resellerdomain.ProductSetting, source string, baseUnit decimal.Decimal) (decimal.Decimal, PricingRule, error) {
	settingID := setting.ID
	rule := PricingRule{
		Mode:              strings.TrimSpace(setting.PricingMode),
		Source:            source,
		SettingID:         &settingID,
		MarkupPercent:     setting.MarkupPercent.Decimal.Round(2),
		FixedMarkupAmount: setting.FixedMarkupAmount.Decimal.Round(2),
		FixedPriceAmount:  setting.FixedPriceAmount.Decimal.Round(2),
	}
	switch rule.Mode {
	case resellerdomain.PricingModeMarkupPercent:
		return ApplyMarkupPercent(baseUnit, rule.MarkupPercent), rule, nil
	case resellerdomain.PricingModeFixedMarkup:
		return baseUnit.Add(rule.FixedMarkupAmount).Round(2), rule, nil
	case resellerdomain.PricingModeFixedPrice:
		return rule.FixedPriceAmount.Round(2), rule, nil
	case resellerdomain.PricingModeInherit:
		return baseUnit.Round(2), rule, nil
	default:
		return decimal.Zero, rule, resellercontract.ErrPricingModeInvalid
	}
}

// ApplyMarkupPercent 按百分比加价。
func ApplyMarkupPercent(baseUnit decimal.Decimal, percent decimal.Decimal) decimal.Decimal {
	return baseUnit.Mul(decimal.NewFromInt(100).Add(percent)).Div(decimal.NewFromInt(100)).Round(2)
}

// ValidateUnitAmount 校验分销单价不低于底价/成本价，且不超过最大加价比例。
func ValidateUnitAmount(profile *resellerdomain.Profile, sku *productdomain.ProductSKU, baseUnit decimal.Decimal, resellerUnit decimal.Decimal) error {
	baseUnit = baseUnit.Round(2)
	resellerUnit = resellerUnit.Round(2)
	if resellerUnit.LessThanOrEqual(decimal.Zero) || resellerUnit.LessThan(baseUnit) {
		return resellercontract.ErrPriceBelowBase
	}
	if sku != nil && sku.CostPriceAmount.Decimal.GreaterThan(decimal.Zero) && resellerUnit.LessThan(sku.CostPriceAmount.Decimal.Round(2)) {
		return resellercontract.ErrPriceBelowBase
	}
	if profile != nil && profile.MaxMarkupPercent.Decimal.GreaterThan(decimal.Zero) && baseUnit.GreaterThan(decimal.Zero) {
		implicit := resellerUnit.Sub(baseUnit).Div(baseUnit).Mul(decimal.NewFromInt(100)).Round(4)
		if implicit.GreaterThan(profile.MaxMarkupPercent.Decimal.Round(4)) {
			return resellercontract.ErrMarkupExceeded
		}
	}
	return nil
}

// ResolveSupplyUnitAmount is shared by storefront display, reseller previews and
// order snapshots. A configured price that falls below current cost fails closed.
func ResolveSupplyUnitAmount(profile *resellerdomain.Profile, productID, skuID uint, retail, cost decimal.Decimal) (decimal.Decimal, error) {
	base := retail.Round(2)
	if profile == nil || profile.SupplyPricing == nil {
		return base, nil
	}
	config := profile.SupplyPricing
	var selected *resellerdomain.SupplyPriceRule
	for i := range config.Overrides {
		rule := &config.Overrides[i]
		if rule.ProductID == productID && rule.SKUID == 0 {
			selected = rule
		}
	}
	for i := range config.Overrides {
		rule := &config.Overrides[i]
		if rule.ProductID == productID && rule.SKUID == skuID && skuID > 0 {
			selected = rule
		}
	}
	if selected != nil {
		value, err := decimal.NewFromString(selected.Value)
		if err != nil || value.IsNegative() {
			return decimal.Zero, resellercontract.ErrPriceBelowBase
		}
		switch selected.Mode {
		case "fixed":
			base = value.Round(2)
		case "cost_markup":
			if !cost.GreaterThan(decimal.Zero) {
				return decimal.Zero, resellercontract.ErrPriceBelowBase
			}
			base = ApplyMarkupPercent(cost, value)
		default:
			return decimal.Zero, resellercontract.ErrPricingModeInvalid
		}
	} else if config.DefaultMarkupPercent != nil && cost.GreaterThan(decimal.Zero) {
		value, err := decimal.NewFromString(*config.DefaultMarkupPercent)
		if err != nil || value.IsNegative() {
			return decimal.Zero, resellercontract.ErrPriceBelowBase
		}
		base = ApplyMarkupPercent(cost, value)
	}
	if base.LessThanOrEqual(decimal.Zero) || (cost.GreaterThan(decimal.Zero) && base.LessThan(cost.Round(2))) {
		return decimal.Zero, resellercontract.ErrPriceBelowBase
	}
	return base, nil
}

func validateSupplyPricing(config *resellerdomain.SupplyPricing) error {
	if config == nil {
		return nil
	}
	if len(config.BlockedProductIDs) > 1000 {
		return resellercontract.ErrPricingModeInvalid
	}
	blocked := map[uint]bool{}
	for _, id := range config.BlockedProductIDs {
		if id == 0 || blocked[id] {
			return resellercontract.ErrPricingModeInvalid
		}
		blocked[id] = true
	}
	if config.DefaultMarkupPercent != nil {
		d, err := decimal.NewFromString(*config.DefaultMarkupPercent)
		if err != nil || d.IsNegative() || d.GreaterThan(decimal.NewFromInt(10000)) {
			return resellercontract.ErrPricingModeInvalid
		}
	}
	if len(config.Overrides) > 1000 {
		return resellercontract.ErrPricingModeInvalid
	}
	seen := map[[2]uint]bool{}
	for _, rule := range config.Overrides {
		key := [2]uint{rule.ProductID, rule.SKUID}
		value, err := decimal.NewFromString(rule.Value)
		if rule.ProductID == 0 || seen[key] || err != nil || value.IsNegative() || value.GreaterThan(decimal.NewFromInt(100000000)) {
			return resellercontract.ErrPricingModeInvalid
		}
		if rule.Mode != "fixed" && rule.Mode != "cost_markup" {
			return resellercontract.ErrPricingModeInvalid
		}
		if rule.Mode == "fixed" && value.Round(2).LessThanOrEqual(decimal.Zero) {
			return resellercontract.ErrPriceBelowBase
		}
		seen[key] = true
	}
	return nil
}

// ProductWithSupplyPrices copies catalogue data; it never changes the main-site
// product or exposes the upstream cost to a reseller response.
func ProductWithSupplyPrices(profile *resellerdomain.Profile, product productdomain.Product) (productdomain.Product, error) {
	result := product
	if !profile.CanSupplyProduct(product.ID) {
		return result, productcontract.ErrNotFound
	}
	result.SKUs = append([]productdomain.ProductSKU(nil), product.SKUs...)
	base, err := ResolveSupplyUnitAmount(profile, product.ID, 0, product.PriceAmount.Decimal, product.CostPriceAmount.Decimal)
	// SKU-only overrides are evaluated below; a product aggregate may lack a cost.
	if err != nil && len(product.SKUs) == 0 {
		return result, err
	}
	if err == nil {
		result.PriceAmount.Decimal = base
	}
	hasSupplySKU := false
	for i := range result.SKUs {
		sku := &result.SKUs[i]
		if !sku.IsActive {
			continue
		}
		price, err := ResolveSupplyUnitAmount(profile, product.ID, sku.ID, sku.PriceAmount.Decimal, sku.CostPriceAmount.Decimal)
		if err != nil {
			return result, err
		}
		sku.PriceAmount.Decimal = price
		if profile != nil && profile.SupplyPricing != nil {
			if !hasSupplySKU || price.LessThan(result.PriceAmount.Decimal) {
				result.PriceAmount.Decimal = price
			}
			hasSupplySKU = true
		}
	}
	return result, nil
}
