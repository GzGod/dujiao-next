package domain

import (
	"time"

	"github.com/dujiao-next/internal/shared/money"
	"github.com/shopspring/decimal"
)

// MemberLevelPrice 商品/SKU 等级定价覆盖
type MemberLevelPrice struct {
	ID            uint         `gorm:"primarykey" json:"id"`
	MemberLevelID uint         `gorm:"uniqueIndex:idx_member_level_price;not null" json:"member_level_id"`                // 关联等级
	ProductID     uint         `gorm:"uniqueIndex:idx_member_level_price;not null" json:"product_id"`                     // 关联商品
	SKUID         uint         `gorm:"column:sku_id;uniqueIndex:idx_member_level_price;not null;default:0" json:"sku_id"` // 0=商品级覆盖，>0=SKU级覆盖
	PriceAmount   money.Amount `gorm:"type:decimal(20,2);not null;default:0" json:"price_amount"`                         // 等级特价（固定金额，优先）
	// PriceRate 按售价比例定价（百分比，如 96 = 售价 × 96%）；PriceAmount 为 0 时生效，售价随上游变动时等级价同步跟随。
	PriceRate decimal.NullDecimal `gorm:"type:decimal(10,4)" json:"price_rate"`
	CreatedAt time.Time           `gorm:"index" json:"created_at"`
	UpdatedAt time.Time           `gorm:"index" json:"updated_at"`
	DeletedAt *time.Time          `gorm:"index" json:"-"`
}

func (MemberLevelPrice) TableName() string {
	return "member_level_prices"
}

// Resolve 按覆盖规则计算等级价：固定金额优先，其次按售价比例；都没有返回 false。
func (p *MemberLevelPrice) Resolve(basePrice decimal.Decimal) (decimal.Decimal, bool) {
	if p == nil {
		return decimal.Zero, false
	}
	if p.PriceAmount.Decimal.GreaterThan(decimal.Zero) {
		return p.PriceAmount.Decimal.Round(2), true
	}
	if p.PriceRate.Valid && p.PriceRate.Decimal.GreaterThan(decimal.Zero) {
		return basePrice.Mul(p.PriceRate.Decimal).Div(decimal.NewFromInt(100)).Round(2), true
	}
	return decimal.Zero, false
}
