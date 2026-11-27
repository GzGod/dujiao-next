package identitypricing

import (
	memberapp "github.com/dujiao-next/internal/modules/memberlevel/application"
	memberstore "github.com/dujiao-next/internal/modules/memberlevel/infrastructure/gormstore"
	resellerapp "github.com/dujiao-next/internal/modules/reseller/application"
	resellerdomain "github.com/dujiao-next/internal/modules/reseller/domain"
	resellerstore "github.com/dujiao-next/internal/modules/reseller/infrastructure/gormstore"
	"github.com/dujiao-next/internal/shared/tradepricing"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Configure keeps all snapshot reads on the same handle, including SQLite transactions.
func Configure(members *memberapp.Service, resellers *resellerstore.Store) {
	resellers.ProfilePricingEnricher = func(db *gorm.DB, p *resellerdomain.Profile) error {
		if p.User == nil || p.User.Status != "active" || p.User.MemberLevelID == 0 {
			return nil
		}
		level, err := memberstore.NewLevelStore(db).GetByID(p.User.MemberLevelID)
		if err != nil {
			return err
		}
		if level == nil || !level.IsActive {
			return nil
		}
		rules, err := memberstore.NewPriceStore(db).ListByLevel(level.ID)
		if err != nil {
			return err
		}
		snapshot := &tradepricing.Snapshot{DiscountRate: level.DiscountRate.String()}
		if level.CostMarkupPercent.Valid {
			pct := level.CostMarkupPercent.Decimal.String()
			snapshot.CostMarkup = &pct
		}
		for _, rule := range rules {
			rate := ""
			if rule.PriceRate.Valid {
				rate = rule.PriceRate.Decimal.String()
			}
			snapshot.Overrides = append(snapshot.Overrides, tradepricing.Override{ProductID: rule.ProductID, SKUID: rule.SKUID, Amount: rule.PriceAmount.String(), Rate: rate})
		}
		p.AgentPricing = snapshot
		return nil
	}
	members.IdentitySupplyPrice = func(userID, productID, skuID uint, retail, cost decimal.Decimal) (decimal.Decimal, bool) {
		profile, err := resellers.GetProfileByUserID(userID)
		if err != nil || profile == nil || profile.Status != resellerdomain.ProfileStatusActive || !profile.CanSupplyProduct(productID) {
			return decimal.Zero, false
		}
		if profile.User == nil || profile.User.Status != "active" {
			return decimal.Zero, false
		}
		value, err := resellerapp.ResolveSupplyUnitAmount(profile, productID, skuID, retail, cost)
		return value, err == nil
	}
}
