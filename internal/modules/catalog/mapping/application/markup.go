package application

import (
	"strconv"

	"github.com/shopspring/decimal"

	mappingcontract "github.com/dujiao-next/internal/modules/catalog/mapping/contract"
	siteconnectioncontract "github.com/dujiao-next/internal/modules/siteconnection/contract"

	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"

	"github.com/dujiao-next/internal/shared/money"
)

// ReapplyMarkup 对指定连接的所有映射商品重新应用加价规则
func (s *Service) ReapplyMarkup(connectionID uint) (int, error) {
	conn, err := s.connections.GetByID(connectionID)
	if err != nil {
		return 0, err
	}
	if conn == nil {
		return 0, siteconnectioncontract.ErrNotFound
	}

	mappings, err := s.mappings.ListActiveByConnection(connectionID)
	if err != nil {
		return 0, err
	}

	updated := 0
	for _, mapping := range mappings {
		skuMappings, err := s.skuMappings.ListByProductMapping(mapping.ID)
		if err != nil {
			continue
		}

		for _, sm := range skuMappings {
			newLocalPrice := CalculateLocalPrice(sm.UpstreamPrice.Decimal, conn.ExchangeRate, mapping.EffectiveMarkupPercent(conn.PriceMarkupPercent), conn.PriceRoundingMode)
			localSKU, err := s.skus.GetByID(sm.LocalSKUID)
			if err != nil || localSKU == nil {
				continue
			}
			localSKU.PriceAmount = money.FromDecimal(newLocalPrice.Round(2))
			localSKU.CostPriceAmount = money.FromDecimal(convertCurrency(sm.UpstreamPrice.Decimal, conn.ExchangeRate).Round(2)) // 成本价 = 上游价格 × 汇率（本地币种）
			_ = s.skus.Update(localSKU)
		}

		// 更新 Product.PriceAmount
		localProduct, err := s.products.GetByID(strconv.FormatUint(uint64(mapping.LocalProductID), 10))
		if err == nil && localProduct != nil {
			s.recalcProductPrice(localProduct)
			updated++
		}
	}

	return updated, nil
}

// recalcProductPrice 重新计算商品基准价格和成本价为最低活跃 SKU 价格
func (s *Service) recalcProductPrice(product *productdomain.Product) {
	allSKUs, err := s.skus.ListByProduct(product.ID, true)
	if err != nil || len(allSKUs) == 0 {
		return
	}
	minPrice := allSKUs[0].PriceAmount.Decimal
	minCostPrice := allSKUs[0].CostPriceAmount.Decimal
	for _, sku := range allSKUs[1:] {
		if sku.PriceAmount.Decimal.LessThan(minPrice) {
			minPrice = sku.PriceAmount.Decimal
		}
		if sku.CostPriceAmount.Decimal.LessThan(minCostPrice) {
			minCostPrice = sku.CostPriceAmount.Decimal
		}
	}
	product.PriceAmount = money.FromDecimal(minPrice.Round(2))
	product.CostPriceAmount = money.FromDecimal(minCostPrice.Round(2))
	_ = s.products.Update(product)
}

// SetMarkup 设置单个映射商品的加价百分比（nil 表示恢复使用连接统一加价），并立即按新比例重算本地售价。
// 价格计算基于最近一次同步记录的上游价格，不发起上游请求。
func (s *Service) SetMarkup(id uint, markupPercent *decimal.Decimal) error {
	mapping, err := s.mappings.GetByID(id)
	if err != nil {
		return err
	}
	if mapping == nil {
		return mappingcontract.ErrMappingNotFound
	}
	if markupPercent != nil && markupPercent.LessThan(minMarkupPercent) {
		return mappingcontract.ErrMarkupInvalid
	}
	if markupPercent != nil {
		mapping.PriceMarkupPercent = decimal.NewNullDecimal(*markupPercent)
	} else {
		mapping.PriceMarkupPercent = decimal.NullDecimal{}
	}
	if err := s.mappings.Update(mapping); err != nil {
		return err
	}

	conn, err := s.connections.GetByID(mapping.ConnectionID)
	if err != nil {
		return err
	}
	if conn == nil {
		return siteconnectioncontract.ErrNotFound
	}
	skuMappings, err := s.skuMappings.ListByProductMapping(mapping.ID)
	if err != nil {
		return err
	}
	markup := mapping.EffectiveMarkupPercent(conn.PriceMarkupPercent)
	for _, sm := range skuMappings {
		localSKU, err := s.skus.GetByID(sm.LocalSKUID)
		if err != nil || localSKU == nil {
			continue
		}
		localSKU.PriceAmount = money.FromDecimal(CalculateLocalPrice(sm.UpstreamPrice.Decimal, conn.ExchangeRate, markup, conn.PriceRoundingMode).Round(2))
		localSKU.CostPriceAmount = money.FromDecimal(convertCurrency(sm.UpstreamPrice.Decimal, conn.ExchangeRate).Round(2))
		if err := s.skus.Update(localSKU); err != nil {
			return err
		}
	}
	localProduct, err := s.products.GetByID(strconv.FormatUint(uint64(mapping.LocalProductID), 10))
	if err == nil && localProduct != nil {
		s.recalcProductPrice(localProduct)
	}
	return nil
}

// minMarkupPercent 单品加价下限：-100% 即售价为 0，再低没有意义。
var minMarkupPercent = decimal.NewFromInt(-100)
