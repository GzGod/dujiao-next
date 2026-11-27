package identitypricing

import (
	"fmt"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	memberapp "github.com/dujiao-next/internal/modules/memberlevel/application"
	memberdomain "github.com/dujiao-next/internal/modules/memberlevel/domain"
	memberstore "github.com/dujiao-next/internal/modules/memberlevel/infrastructure/gormstore"
	resellerapp "github.com/dujiao-next/internal/modules/reseller/application"
	resellercontract "github.com/dujiao-next/internal/modules/reseller/contract"
	resellerdomain "github.com/dujiao-next/internal/modules/reseller/domain"
	resellerstore "github.com/dujiao-next/internal/modules/reseller/infrastructure/gormstore"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/money"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestIndependentGradesLowestAndSQLiteTransaction(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:identity-pricing-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	defer sqlDB.Close()
	if err = db.AutoMigrate(&userdomain.User{}, &memberdomain.MemberLevel{}, &memberdomain.MemberLevelPrice{}, &resellerdomain.Profile{}); err != nil {
		t.Fatal(err)
	}
	level := memberdomain.MemberLevel{NameJSON: jsonmap.JSON{"en-US": "agent"}, Slug: "agent", IsActive: true, DiscountRate: money.FromDecimal(decimal.NewFromInt(100)), CostMarkupPercent: decimal.NewNullDecimal(decimal.NewFromInt(5))}
	if err = db.Create(&level).Error; err != nil {
		t.Fatal(err)
	}
	user := userdomain.User{Email: "tiers@example.test", PasswordHash: "hash", Status: "active", MemberLevelID: level.ID}
	if err = db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	profile := resellerdomain.Profile{UserID: user.ID, Status: "active", SettlementStatus: "normal", SupplyPricing: &resellerdomain.SupplyPricing{Grade: "advanced"}}
	if err = db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	members := memberapp.NewService(memberstore.NewLevelStore(db), memberstore.NewPriceStore(db), memberstore.NewUserStore(db))
	store := resellerstore.New(db)
	Configure(members, store)
	got, _ := members.ResolveUserPrice(user.ID, level.ID, 19, 1, decimal.NewFromInt(130), decimal.NewFromInt(101))
	if got.String() != "102.01" {
		t.Fatalf("lowest reseller identity: %s", got)
	}
	profile.SupplyPricing = &resellerdomain.SupplyPricing{Grade: "basic", BlockedProductIDs: []uint{19}}
	if err = db.Model(&profile).Select("SupplyPricing").Updates(&profile).Error; err != nil {
		t.Fatal(err)
	}
	got, _ = members.ResolveUserPrice(user.ID, level.ID, 19, 1, decimal.NewFromInt(130), decimal.NewFromInt(101))
	if got.String() != "106.05" {
		t.Fatalf("unauthorized reseller quote used: %s", got)
	}
	// A lower agent override still wins over an assigned storefront grade.
	override := memberdomain.MemberLevelPrice{MemberLevelID: level.ID, ProductID: 20, SKUID: 1, PriceAmount: money.FromDecimal(decimal.NewFromInt(102))}
	if err = db.Create(&override).Error; err != nil {
		t.Fatal(err)
	}
	got, _ = members.ResolveUserPrice(user.ID, level.ID, 20, 1, decimal.NewFromInt(130), decimal.NewFromInt(101))
	if got.String() != "102" {
		t.Fatalf("lower agent quote ignored: %s", got)
	}
	var unchanged userdomain.User
	if err = db.First(&unchanged, user.ID).Error; err != nil || unchanged.MemberLevelID != level.ID {
		t.Fatal("storefront identity changed agent membership")
	}
	// The snapshot loader must use the transaction handle, never the global pool.
	done := make(chan error, 1)
	go func() {
		done <- store.WithinManagementTransaction(func(tx resellercontract.ManagementStore) error {
			p, e := tx.GetProfileByID(profile.ID)
			if e != nil {
				return e
			}
			_, e = resellerapp.ResolveSupplyUnitAmount(p, 20, 1, decimal.NewFromInt(130), decimal.NewFromInt(101))
			return e
		})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SQLite pricing deadlock")
	}
}
