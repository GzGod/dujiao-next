package integrationtest

import (
	"testing"

	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	memberleveldomain "github.com/dujiao-next/internal/modules/memberlevel/domain"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func markRequiresApproval(t *testing.T, db *gorm.DB, levels ...memberleveldomain.MemberLevel) {
	t.Helper()
	for _, level := range levels {
		if err := db.Model(&memberleveldomain.MemberLevel{}).Where("id = ?", level.ID).Update("requires_approval", true).Error; err != nil {
			t.Fatalf("mark requires_approval failed: %v", err)
		}
	}
}

func memberLevelOf(t *testing.T, db *gorm.DB, userID uint) uint {
	t.Helper()
	var user userdomain.User
	if err := db.First(&user, userID).Error; err != nil {
		t.Fatalf("fetch user failed: %v", err)
	}
	return user.MemberLevelID
}

// 代理等级（需审核）布局：普通 0 → 初级代理 100(5000) → 中级代理 200(10000) → 高级代理 300(100000)
func dealerLevels(t *testing.T, db *gorm.DB) (normal, basic, pro, elite memberleveldomain.MemberLevel) {
	normal = createMemberLevelFixture(t, db, "normal", 0, "0", true)
	basic = createMemberLevelFixture(t, db, "dealer", 100, "5000", false)
	pro = createMemberLevelFixture(t, db, "dealer-pro", 200, "10000", false)
	elite = createMemberLevelFixture(t, db, "dealer-elite", 300, "100000", false)
	markRequiresApproval(t, db, basic, pro, elite)
	return
}

func TestApprovalLevelNotReachableBySpendingAlone(t *testing.T) {
	svc, db := newMemberLevelServiceForTest(t)
	normal, _, _, _ := dealerLevels(t, db)
	user := createUserFixture(t, db, "customer@example.com", normal.ID)

	if err := svc.OnOrderPaid(user.ID, decimal.RequireFromString("200000")); err != nil {
		t.Fatalf("OnOrderPaid failed: %v", err)
	}
	if got := memberLevelOf(t, db, user.ID); got != normal.ID {
		t.Fatalf("customer must stay on normal level without approval, got level %d", got)
	}
}

func TestApprovedDealerUpgradesWithinDealerTiers(t *testing.T) {
	svc, db := newMemberLevelServiceForTest(t)
	_, basic, pro, elite := dealerLevels(t, db)
	user := createUserFixture(t, db, "dealer@example.com", basic.ID)

	if err := svc.OnOrderPaid(user.ID, decimal.RequireFromString("12000")); err != nil {
		t.Fatalf("OnOrderPaid failed: %v", err)
	}
	if got := memberLevelOf(t, db, user.ID); got != pro.ID {
		t.Fatalf("approved dealer should reach pro at 12000, got level %d", got)
	}
	if err := svc.OnOrderPaid(user.ID, decimal.RequireFromString("90000")); err != nil {
		t.Fatalf("OnOrderPaid failed: %v", err)
	}
	if got := memberLevelOf(t, db, user.ID); got != elite.ID {
		t.Fatalf("approved dealer should reach elite at 102000, got level %d", got)
	}
}

func TestApprovedDealerJumpsStraightToHighestQualifiedTier(t *testing.T) {
	svc, db := newMemberLevelServiceForTest(t)
	_, basic, _, elite := dealerLevels(t, db)
	user := createUserFixture(t, db, "big@example.com", basic.ID)

	if err := svc.OnOrderPaid(user.ID, decimal.RequireFromString("150000")); err != nil {
		t.Fatalf("OnOrderPaid failed: %v", err)
	}
	if got := memberLevelOf(t, db, user.ID); got != elite.ID {
		t.Fatalf("expected elite, got level %d", got)
	}
}

func TestNormalLevelsStillAutoUpgrade(t *testing.T) {
	svc, db := newMemberLevelServiceForTest(t)
	normal, _, _, _ := dealerLevels(t, db)
	vip := createMemberLevelFixture(t, db, "vip", 50, "1000", false)
	user := createUserFixture(t, db, "vip@example.com", normal.ID)

	if err := svc.OnOrderPaid(user.ID, decimal.RequireFromString("200000")); err != nil {
		t.Fatalf("OnOrderPaid failed: %v", err)
	}
	if got := memberLevelOf(t, db, user.ID); got != vip.ID {
		t.Fatalf("non-approval level should still auto-upgrade, got level %d", got)
	}
}
