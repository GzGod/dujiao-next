package domain

import (
	"github.com/dujiao-next/internal/shared/tradepricing"
	"time"

	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	"github.com/dujiao-next/internal/shared/money"
)

// Profile 分销商资料。
// SupplyPricing is controlled exclusively by the main-site administrator.
// Nil DefaultMarkupPercent preserves the legacy main-site retail base.
type SupplyPricing struct {
	Grade                string            `json:"grade,omitempty"`
	AllowedProductIDs    *[]uint           `json:"allowed_product_ids,omitempty"`
	BlockedProductIDs    []uint            `json:"blocked_product_ids,omitempty"`
	DefaultMarkupPercent *string           `json:"default_markup_percent"`
	Overrides            []SupplyPriceRule `json:"overrides"`
}

type SupplyPriceRule struct {
	ProductID uint   `json:"product_id"`
	SKUID     uint   `json:"sku_id"`
	Mode      string `json:"mode"`
	Value     string `json:"value"`
}

type Profile struct {
	AgentPricing         *tradepricing.Snapshot `gorm:"-" json:"-"`
	SupplyPricing        *SupplyPricing         `gorm:"serializer:json;type:json" json:"-"`
	ID                   uint                   `gorm:"primarykey" json:"id"`
	UserID               uint                   `gorm:"not null;uniqueIndex" json:"user_id"`
	Status               string                 `gorm:"type:varchar(32);index;not null;default:'pending_review'" json:"status"`
	ApplyReason          string                 `gorm:"type:text" json:"apply_reason,omitempty"`
	RejectReason         string                 `gorm:"type:text" json:"reject_reason,omitempty"`
	DefaultMarkupPercent money.Amount           `gorm:"type:decimal(10,2);not null;default:0" json:"default_markup_percent"`
	MaxMarkupPercent     money.Amount           `gorm:"type:decimal(10,2);not null;default:0" json:"max_markup_percent"`
	SettlementStatus     string                 `gorm:"type:varchar(32);index;not null;default:'normal'" json:"settlement_status"`
	ReviewedBy           *uint                  `gorm:"index" json:"reviewed_by,omitempty"`
	ReviewedAt           *time.Time             `gorm:"index" json:"reviewed_at,omitempty"`
	CreatedAt            time.Time              `gorm:"index" json:"created_at"`
	UpdatedAt            time.Time              `gorm:"index" json:"updated_at"`
	DeletedAt            *time.Time             `gorm:"index" json:"-"`

	User *userdomain.User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

func (Profile) TableName() string { return "reseller_profiles" }

// CanSupplyProduct is a main-site permission, independent of reseller-managed
// listing preferences. Missing configuration preserves existing catalogue access.
func (p *Profile) CanSupplyProduct(productID uint) bool {
	if p == nil || p.SupplyPricing == nil {
		return true
	}
	if allowed := p.SupplyPricing.AllowedProductIDs; allowed != nil {
		found := false
		for _, id := range *allowed {
			if id == productID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for _, id := range p.SupplyPricing.BlockedProductIDs {
		if id == productID {
			return false
		}
	}
	return true
}
