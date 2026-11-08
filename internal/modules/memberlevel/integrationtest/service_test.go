package integrationtest

import (
	"fmt"
	"testing"
	"time"

	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	memberlevelapp "github.com/dujiao-next/internal/modules/memberlevel/application"
	memberlevelcontract "github.com/dujiao-next/internal/modules/memberlevel/contract"
	memberleveldomain "github.com/dujiao-next/internal/modules/memberlevel/domain"
	"github.com/dujiao-next/internal/modules/memberlevel/infrastructure/gormstore"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/money"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func newMemberLevelServiceForTest(t *testing.T) (*memberlevelapp.Service, *gorm.DB) {
	t.Helper()

	dsn := fmt.Sprintf("file:member_level_service_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&userdomain.User{}, &memberleveldomain.MemberLevel{}, &memberleveldomain.MemberLevelPrice{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	levelRepo := gormstore.NewLevelStore(db)
	priceRepo := gormstore.NewPriceStore(db)
	userRepo := gormstore.NewUserStore(db)
	return memberlevelapp.NewService(levelRepo, priceRepo, userRepo), db
}

func createMemberLevelFixture(
	t *testing.T,
	db *gorm.DB,
	slug string,
	sortOrder int,
	spendThreshold string,
	isDefault bool,
) memberleveldomain.MemberLevel {
	t.Helper()

	level := memberleveldomain.MemberLevel{
		NameJSON: jsonmap.JSON{
			"zh-CN": slug,
		},
		Slug:              slug,
		DiscountRate:      money.FromDecimal(decimal.NewFromInt(100)),
		RechargeThreshold: money.FromDecimal(decimal.Zero),
		SpendThreshold:    money.FromDecimal(decimal.RequireFromString(spendThreshold)),
		IsDefault:         isDefault,
		SortOrder:         sortOrder,
		IsActive:          true,
	}
	if err := db.Create(&level).Error; err != nil {
		t.Fatalf("create member level fixture failed: %v", err)
	}
	return level
}

func createUserFixture(t *testing.T, db *gorm.DB, email string, memberLevelID uint) userdomain.User {
	t.Helper()

	user := userdomain.User{
		Email:          email,
		PasswordHash:   "test-hash",
		Status:         "active",
		MemberLevelID:  memberLevelID,
		TotalRecharged: money.FromDecimal(decimal.Zero),
		TotalSpent:     money.FromDecimal(decimal.Zero),
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user fixture failed: %v", err)
	}
	return user
}

func TestMemberLevelServiceOnOrderPaidDoesNotMoveToEqualSortOrder(t *testing.T) {
	svc, db := newMemberLevelServiceForTest(t)
	defaultLevel := createMemberLevelFixture(t, db, "default", 0, "0", true)
	_ = createMemberLevelFixture(t, db, "vip", 0, "0.01", false)
	user := createUserFixture(t, db, "equal-sort@example.com", defaultLevel.ID)

	if err := svc.OnOrderPaid(user.ID, decimal.RequireFromString("0.01")); err != nil {
		t.Fatalf("OnOrderPaid failed: %v", err)
	}

	var updated userdomain.User
	if err := db.First(&updated, user.ID).Error; err != nil {
		t.Fatalf("fetch updated user failed: %v", err)
	}
	if updated.MemberLevelID != defaultLevel.ID {
		t.Fatalf("expected keep member_level_id=%d, got %d", defaultLevel.ID, updated.MemberLevelID)
	}
	if !updated.TotalSpent.Decimal.Equal(decimal.RequireFromString("0.01")) {
		t.Fatalf("expected total_spent=0.01, got %s", updated.TotalSpent.Decimal.String())
	}
}

func TestMemberLevelServiceOnOrderPaidKeepsHigherLevel(t *testing.T) {
	svc, db := newMemberLevelServiceForTest(t)
	highLevel := createMemberLevelFixture(t, db, "high", 100, "0", true)
	_ = createMemberLevelFixture(t, db, "low", 10, "0.01", false)
	user := createUserFixture(t, db, "no-downgrade@example.com", highLevel.ID)

	if err := svc.OnOrderPaid(user.ID, decimal.RequireFromString("50")); err != nil {
		t.Fatalf("OnOrderPaid failed: %v", err)
	}

	var updated userdomain.User
	if err := db.First(&updated, user.ID).Error; err != nil {
		t.Fatalf("fetch updated user failed: %v", err)
	}
	if updated.MemberLevelID != highLevel.ID {
		t.Fatalf("expected keep member_level_id=%d, got %d", highLevel.ID, updated.MemberLevelID)
	}
}

func TestMemberLevelServiceOnOrderPaidUpgradesToHigherSortOrder(t *testing.T) {
	svc, db := newMemberLevelServiceForTest(t)
	defaultLevel := createMemberLevelFixture(t, db, "default2", 0, "0", true)
	goldLevel := createMemberLevelFixture(t, db, "gold", 20, "0.01", false)
	user := createUserFixture(t, db, "higher-sort@example.com", defaultLevel.ID)

	if err := svc.OnOrderPaid(user.ID, decimal.RequireFromString("0.01")); err != nil {
		t.Fatalf("OnOrderPaid failed: %v", err)
	}

	var updated userdomain.User
	if err := db.First(&updated, user.ID).Error; err != nil {
		t.Fatalf("fetch updated user failed: %v", err)
	}
	if updated.MemberLevelID != goldLevel.ID {
		t.Fatalf("expected member_level_id=%d, got %d", goldLevel.ID, updated.MemberLevelID)
	}
}

type concurrentUserRepository struct {
	base                     memberlevelcontract.UserRepository
	afterFirstSpendIncrement func()
	spendIncrementCalled     bool
}

func (r *concurrentUserRepository) GetByID(id uint) (*userdomain.User, error) {
	return r.base.GetByID(id)
}

func (r *concurrentUserRepository) Update(user *userdomain.User) error {
	return r.base.Update(user)
}

func (r *concurrentUserRepository) IncrementTotalRecharged(userID uint, amount decimal.Decimal) error {
	return r.base.IncrementTotalRecharged(userID, amount)
}

func (r *concurrentUserRepository) IncrementTotalSpent(userID uint, amount decimal.Decimal) error {
	if err := r.base.IncrementTotalSpent(userID, amount); err != nil {
		return err
	}
	if !r.spendIncrementCalled && r.afterFirstSpendIncrement != nil {
		r.spendIncrementCalled = true
		r.afterFirstSpendIncrement()
	}
	return nil
}

func (r *concurrentUserRepository) UpdateMemberLevelIfCurrent(userID, currentLevelID, nextLevelID uint) (int64, error) {
	return r.base.UpdateMemberLevelIfCurrent(userID, currentLevelID, nextLevelID)
}

func (r *concurrentUserRepository) AssignDefaultMemberLevel(defaultLevelID uint) (int64, error) {
	return r.base.AssignDefaultMemberLevel(defaultLevelID)
}

func TestMemberLevelServiceOnOrderPaidDoesNotOverwriteConcurrentHigherLevel(t *testing.T) {
	_, db := newMemberLevelServiceForTest(t)
	defaultLevel := createMemberLevelFixture(t, db, "race-default", 0, "0", true)
	goldLevel := createMemberLevelFixture(t, db, "race-gold", 20, "1.00", false)
	highLevel := createMemberLevelFixture(t, db, "race-high", 100, "1000.00", false)
	user := createUserFixture(t, db, "race-no-downgrade@example.com", defaultLevel.ID)

	baseUserRepo := gormstore.NewUserStore(db)
	raceRepo := &concurrentUserRepository{
		base: baseUserRepo,
		afterFirstSpendIncrement: func() {
			if err := db.Model(&userdomain.User{}).Where("id = ?", user.ID).Update("member_level_id", highLevel.ID).Error; err != nil {
				t.Fatalf("simulate concurrent higher level update failed: %v", err)
			}
		},
	}
	svc := memberlevelapp.NewService(
		gormstore.NewLevelStore(db),
		gormstore.NewPriceStore(db),
		raceRepo,
	)

	if err := svc.OnOrderPaid(user.ID, decimal.RequireFromString("1.00")); err != nil {
		t.Fatalf("OnOrderPaid failed: %v", err)
	}

	var updated userdomain.User
	if err := db.First(&updated, user.ID).Error; err != nil {
		t.Fatalf("fetch updated user failed: %v", err)
	}
	if updated.MemberLevelID == goldLevel.ID {
		t.Fatalf("expected concurrent high level to be preserved, got auto-upgraded lower level %d", updated.MemberLevelID)
	}
	if updated.MemberLevelID != highLevel.ID {
		t.Fatalf("expected member_level_id=%d, got %d", highLevel.ID, updated.MemberLevelID)
	}
	if !updated.TotalSpent.Decimal.Equal(decimal.RequireFromString("1.00")) {
		t.Fatalf("expected total_spent=1.00, got %s", updated.TotalSpent.Decimal.String())
	}
}

func TestMemberLevelServiceCreateLevelRejectsActiveSortOrderConflict(t *testing.T) {
	svc, db := newMemberLevelServiceForTest(t)
	_ = createMemberLevelFixture(t, db, "sort-existing", 10, "0", true)

	err := svc.CreateLevel(&memberleveldomain.MemberLevel{
		NameJSON:          jsonmap.JSON{"zh-CN": "sort-conflict"},
		Slug:              "sort-conflict",
		DiscountRate:      money.FromDecimal(decimal.NewFromInt(100)),
		RechargeThreshold: money.FromDecimal(decimal.Zero),
		SpendThreshold:    money.FromDecimal(decimal.NewFromInt(1)),
		IsDefault:         false,
		SortOrder:         10,
		IsActive:          true,
	})
	if err == nil {
		t.Fatalf("expected active sort_order conflict to be rejected")
	}
}

func TestMemberLevelServiceUpdateLevelRejectsActiveSortOrderConflict(t *testing.T) {
	svc, db := newMemberLevelServiceForTest(t)
	_ = createMemberLevelFixture(t, db, "sort-update-existing", 10, "0", true)
	target := createMemberLevelFixture(t, db, "sort-update-target", 20, "1.00", false)

	target.SortOrder = 10
	err := svc.UpdateLevel(&target)
	if err == nil {
		t.Fatalf("expected active sort_order conflict to be rejected")
	}
}

func TestResolveMemberPriceFollowsRetailByRate(t *testing.T) {
	svc, db := newMemberLevelServiceForTest(t)
	level := createMemberLevelFixture(t, db, "dealer-rate", 1, "0", false)

	if err := gormstore.NewPriceStore(db).BatchUpsert([]memberleveldomain.MemberLevelPrice{{
		MemberLevelID: level.ID,
		ProductID:     7,
		PriceRate:     decimal.NewNullDecimal(decimal.RequireFromString("96")),
	}}); err != nil {
		t.Fatalf("upsert rate price: %v", err)
	}

	// 售价 100 → 96；上游涨价后售价 200 → 192，等级价跟着售价走
	for _, tc := range []struct{ base, want string }{{"100", "96"}, {"200", "192"}} {
		got, discount := svc.ResolveMemberPrice(level.ID, 7, 0, decimal.RequireFromString(tc.base), decimal.Zero)
		if !got.Equal(decimal.RequireFromString(tc.want)) {
			t.Fatalf("base %s: got %s want %s", tc.base, got, tc.want)
		}
		if !discount.Equal(decimal.RequireFromString(tc.base).Sub(got)) {
			t.Fatalf("base %s: unexpected discount %s", tc.base, discount)
		}
	}
}

func TestResolveMemberPriceFixedAmountWinsOverRate(t *testing.T) {
	svc, db := newMemberLevelServiceForTest(t)
	level := createMemberLevelFixture(t, db, "dealer-fixed", 1, "0", false)

	if err := gormstore.NewPriceStore(db).BatchUpsert([]memberleveldomain.MemberLevelPrice{{
		MemberLevelID: level.ID,
		ProductID:     8,
		PriceAmount:   money.FromDecimal(decimal.RequireFromString("90")),
		PriceRate:     decimal.NewNullDecimal(decimal.RequireFromString("50")),
	}}); err != nil {
		t.Fatalf("upsert price: %v", err)
	}

	got, _ := svc.ResolveMemberPrice(level.ID, 8, 0, decimal.RequireFromString("100"), decimal.Zero)
	if !got.Equal(decimal.RequireFromString("90")) {
		t.Fatalf("fixed amount should win, got %s", got)
	}
}

func TestResolveMemberPriceNeverExceedsRetail(t *testing.T) {
	svc, db := newMemberLevelServiceForTest(t)
	level := createMemberLevelFixture(t, db, "dealer-cap", 1, "0", false)

	if err := gormstore.NewPriceStore(db).BatchUpsert([]memberleveldomain.MemberLevelPrice{{
		MemberLevelID: level.ID,
		ProductID:     9,
		PriceRate:     decimal.NewNullDecimal(decimal.RequireFromString("100")),
	}}); err != nil {
		t.Fatalf("upsert price: %v", err)
	}

	got, discount := svc.ResolveMemberPrice(level.ID, 9, 0, decimal.RequireFromString("100"), decimal.Zero)
	if !got.Equal(decimal.RequireFromString("100")) || !discount.IsZero() {
		t.Fatalf("rate 100%% must not discount, got %s / %s", got, discount)
	}
}

func TestBatchUpsertUpdatesRateOnExistingPrice(t *testing.T) {
	_, db := newMemberLevelServiceForTest(t)
	store := gormstore.NewPriceStore(db)
	base := memberleveldomain.MemberLevelPrice{MemberLevelID: 1, ProductID: 10, PriceAmount: money.FromDecimal(decimal.RequireFromString("50"))}
	if err := store.BatchUpsert([]memberleveldomain.MemberLevelPrice{base}); err != nil {
		t.Fatal(err)
	}
	// 从固定金额切换到比例
	if err := store.BatchUpsert([]memberleveldomain.MemberLevelPrice{{
		MemberLevelID: 1, ProductID: 10, PriceRate: decimal.NewNullDecimal(decimal.RequireFromString("95")),
	}}); err != nil {
		t.Fatal(err)
	}
	saved, err := store.GetByLevelAndProductAndSKU(1, 10, 0)
	if err != nil || saved == nil {
		t.Fatalf("load: %v", err)
	}
	if !saved.PriceAmount.Decimal.IsZero() || !saved.PriceRate.Valid || !saved.PriceRate.Decimal.Equal(decimal.RequireFromString("95")) {
		t.Fatalf("unexpected saved price: amount=%s rate=%v", saved.PriceAmount.Decimal, saved.PriceRate)
	}
}

func TestResolveMemberPriceUsesLevelCostMarkup(t *testing.T) {
	svc, db := newMemberLevelServiceForTest(t)
	level := createMemberLevelFixture(t, db, "dealer-elite-cost", 1, "0", false)
	level.CostMarkupPercent = decimal.NewNullDecimal(decimal.RequireFromString("1"))
	level.DiscountRate = money.FromDecimal(decimal.RequireFromString("91.9"))
	if err := db.Save(&level).Error; err != nil {
		t.Fatal(err)
	}

	// 成本 621.20 → 627.41，与售价无关
	got, _ := svc.ResolveMemberPrice(level.ID, 20, 0, decimal.RequireFromString("660"), decimal.RequireFromString("621.20"))
	if !got.Equal(decimal.RequireFromString("627.41")) {
		t.Fatalf("expected cost × 1.01 = 627.41, got %s", got)
	}
	// 成本未知时回落到折扣率
	got, _ = svc.ResolveMemberPrice(level.ID, 20, 0, decimal.RequireFromString("100"), decimal.Zero)
	if !got.Equal(decimal.RequireFromString("91.9")) {
		t.Fatalf("expected discount-rate fallback 91.9, got %s", got)
	}
	// 成本加价后不得高于售价
	got, discount := svc.ResolveMemberPrice(level.ID, 20, 0, decimal.RequireFromString("100"), decimal.RequireFromString("99.50"))
	if !got.Equal(decimal.RequireFromString("100")) || !discount.IsZero() {
		t.Fatalf("expected retail cap 100, got %s / %s", got, discount)
	}
	// 商品级覆盖仍然优先
	if err := gormstore.NewPriceStore(db).BatchUpsert([]memberleveldomain.MemberLevelPrice{{
		MemberLevelID: level.ID, ProductID: 21, PriceAmount: money.FromDecimal(decimal.RequireFromString("500")),
	}}); err != nil {
		t.Fatal(err)
	}
	got, _ = svc.ResolveMemberPrice(level.ID, 21, 0, decimal.RequireFromString("600"), decimal.RequireFromString("100"))
	if !got.Equal(decimal.RequireFromString("500")) {
		t.Fatalf("product override should win, got %s", got)
	}
}
