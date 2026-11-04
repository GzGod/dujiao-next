package mappingreader

import (
	mappingdomain "github.com/dujiao-next/internal/modules/catalog/mapping/domain"
	procurementcontract "github.com/dujiao-next/internal/modules/procurement/contract"
)

type ProductSource interface {
	GetByLocalProductID(productID uint) (*mappingdomain.Mapping, error)
}

type SKUReaderSource interface {
	GetByLocalSKUID(skuID uint) (*mappingdomain.SKUMapping, error)
}

type ProductReader struct{ source ProductSource }
type SKUReader struct{ source SKUReaderSource }

var _ procurementcontract.ProductMappingReader = (*ProductReader)(nil)
var _ procurementcontract.SKUMappingReader = (*SKUReader)(nil)

func NewProducts(source ProductSource) *ProductReader { return &ProductReader{source: source} }
func NewSKUs(source SKUReaderSource) *SKUReader       { return &SKUReader{source: source} }

func (r *ProductReader) FindConnectionID(productID uint) (uint, bool, error) {
	mapping, err := r.source.GetByLocalProductID(productID)
	if err != nil || mapping == nil {
		return 0, false, err
	}
	return mapping.ConnectionID, true, nil
}

func (r *SKUReader) FindUpstreamSKUID(skuID uint) (uint, bool, error) {
	mapping, err := r.source.GetByLocalSKUID(skuID)
	if err != nil || mapping == nil {
		return 0, false, err
	}
	return mapping.UpstreamSKUID, true, nil
}

// FindUpstreamSKUInfo 同时返回上游 SKU ID、供货价字符串和币种。
// currency 从 sku_mappings 所属的 product_mapping → connection 的 currency 字段读取；
// 若未设置则默认 "CNY"（gptget.fun 当前仅支持人民币结算）。
func (r *SKUReader) FindUpstreamSKUInfo(skuID uint) (upstreamSKUID uint, upstreamPrice string, currency string, found bool, err error) {
	mapping, err := r.source.GetByLocalSKUID(skuID)
	if err != nil || mapping == nil {
		return 0, "", "", false, err
	}
	price := mapping.UpstreamPrice.StringFixed(2)
	// 默认 CNY；后续如需多币种可扩展 connection.currency 字段
	cur := "CNY"
	return mapping.UpstreamSKUID, price, cur, true, nil
}
