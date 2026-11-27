package gormstore

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	orderriskdomain "github.com/dujiao-next/internal/modules/orderrisk/domain"
	"testing"
	"time"

	ordercontract "github.com/dujiao-next/internal/modules/order/contract"

	fulfillmentdomain "github.com/dujiao-next/internal/modules/fulfillment/domain"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"

	usercontract "github.com/dujiao-next/internal/modules/identity/user/contract"
	userstore "github.com/dujiao-next/internal/modules/identity/user/infrastructure/gormstore"

	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"

	externalidentitydomain "github.com/dujiao-next/internal/modules/identity/externalidentity/domain"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupAdminSearchRepositoryTest(t *testing.T) (*userstore.Store, *Store, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:admin_search_repo_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&userdomain.User{}, &externalidentitydomain.Identity{}, &orderdomain.Order{}, &orderdomain.OrderItem{}, &fulfillmentdomain.Fulfillment{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	return userstore.New(db), New(db, "test-guest-credential-secret-with-32-bytes"), db
}

func TestUserRepositoryListSupportsOAuthKeyword(t *testing.T) {
	userRepo, _, db := setupAdminSearchRepositoryTest(t)

	user1 := userdomain.User{Email: "telegram_6059928735@login.local", PasswordHash: "hash", DisplayName: "TG Buyer", Status: "active"}
	user2 := userdomain.User{Email: "normal@example.com", PasswordHash: "hash", DisplayName: "Normal User", Status: "active"}
	if err := db.Create(&user1).Error; err != nil {
		t.Fatalf("create user1 failed: %v", err)
	}
	if err := db.Create(&user2).Error; err != nil {
		t.Fatalf("create user2 failed: %v", err)
	}
	if err := db.Create(&externalidentitydomain.Identity{
		UserID:         user1.ID,
		Provider:       "telegram",
		ProviderUserID: "6059928735",
		Username:       "tg_demo_user",
	}).Error; err != nil {
		t.Fatalf("create oauth identity failed: %v", err)
	}

	users, total, err := userRepo.List(usercontract.ListFilter{Page: 1, PageSize: 20, Keyword: "tg_demo_user"})
	if err != nil {
		t.Fatalf("list by username failed: %v", err)
	}
	if total != 1 || len(users) != 1 || users[0].ID != user1.ID {
		t.Fatalf("unexpected result by username: total=%d users=%+v", total, users)
	}

	users, total, err = userRepo.List(usercontract.ListFilter{Page: 1, PageSize: 20, Keyword: "6059928735"})
	if err != nil {
		t.Fatalf("list by provider user id failed: %v", err)
	}
	if total != 1 || len(users) != 1 || users[0].ID != user1.ID {
		t.Fatalf("unexpected result by provider user id: total=%d users=%+v", total, users)
	}

	users, total, err = userRepo.List(usercontract.ListFilter{Page: 1, PageSize: 20, UserID: user2.ID})
	if err != nil {
		t.Fatalf("list by user id failed: %v", err)
	}
	if total != 1 || len(users) != 1 || users[0].ID != user2.ID {
		t.Fatalf("unexpected result by user id: total=%d users=%+v", total, users)
	}
}

func TestOrderRepositoryListAdminSupportsUserKeyword(t *testing.T) {
	_, orderRepo, db := setupAdminSearchRepositoryTest(t)

	user := userdomain.User{Email: "telegram_6059928735@login.local", PasswordHash: "hash", DisplayName: "TG Buyer", Status: "active"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	if err := db.Create(&externalidentitydomain.Identity{
		UserID:         user.ID,
		Provider:       "telegram",
		ProviderUserID: "6059928735",
		Username:       "tg_demo_user",
	}).Error; err != nil {
		t.Fatalf("create oauth identity failed: %v", err)
	}

	order := orderdomain.Order{
		OrderNo:   "DJ-ORDER-TG-001",
		UserID:    user.ID,
		Status:    "paid",
		Currency:  "USD",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("create order failed: %v", err)
	}

	rows, total, err := orderRepo.ListAdmin(ordercontract.ListFilter{Page: 1, PageSize: 20, UserKeyword: "tg_demo_user"})
	if err != nil {
		t.Fatalf("list orders by oauth username failed: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].ID != order.ID {
		t.Fatalf("unexpected result by oauth username: total=%d rows=%+v", total, rows)
	}

	rows, total, err = orderRepo.ListAdmin(ordercontract.ListFilter{Page: 1, PageSize: 20, UserKeyword: "6059928735"})
	if err != nil {
		t.Fatalf("list orders by provider user id failed: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].ID != order.ID {
		t.Fatalf("unexpected result by provider user id: total=%d rows=%+v", total, rows)
	}
}

func TestListAdminIssuedOnlyIncludesDeliveredChildrenWithoutDuplicates(t *testing.T) {
	_, repo, db := setupAdminSearchRepositoryTest(t)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	create := func(no string, parent *uint) orderdomain.Order {
		o := orderdomain.Order{OrderNo: no, ParentID: parent, Status: "fulfilling", Currency: "CNY", GuestEmail: no + "@example.test"}
		if err := db.Create(&o).Error; err != nil {
			t.Fatal(err)
		}
		return o
	}
	direct := create("DIRECT", nil)
	split := create("SPLIT", nil)
	pending := create("PENDING", nil)
	deleted := create("DELETED", nil)
	deletedChildParent := create("DELETED-CHILD-PARENT", nil)
	_ = create("NO-DELIVERY", nil)
	child1 := create("SPLIT-1", &split.ID)
	child2 := create("SPLIT-2", &split.ID)
	deletedChild := create("DELETED-CHILD", &deletedChildParent.ID)
	now := time.Now()
	for _, f := range []fulfillmentdomain.Fulfillment{
		{OrderID: direct.ID, Status: "delivered", Type: "auto", Payload: "fixture-direct"},
		{OrderID: child1.ID, Status: "delivered", Type: "upstream", Payload: "fixture-child-1"},
		{OrderID: child2.ID, Status: "delivered", Type: "manual", Payload: "fixture-child-2"},
		{OrderID: pending.ID, Status: "pending", Type: "auto"},
		{OrderID: deleted.ID, Status: "delivered", Type: "auto", DeletedAt: &now},
		{OrderID: deletedChild.ID, Status: "delivered", Type: "auto"},
	} {
		if err := db.Create(&f).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&deletedChild).Update("deleted_at", now).Error; err != nil {
		t.Fatal(err)
	}
	rows, total, err := repo.ListAdmin(ordercontract.ListFilter{IssuedOnly: true, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(rows) != 2 {
		t.Fatalf("expected two parent orders; total=%d rows=%d", total, len(rows))
	}
	for _, row := range rows {
		if row.ID == direct.ID && (row.Fulfillment == nil || row.Fulfillment.Payload != "fixture-direct") {
			t.Fatal("missing direct delivery")
		}
		if row.ID == split.ID && (len(row.Children) != 2 || row.Children[0].Fulfillment == nil || row.Children[1].Fulfillment == nil) {
			t.Fatal("missing child deliveries")
		}
	}
	rows, total, err = repo.ListAdmin(ordercontract.ListFilter{IssuedOnly: true, Page: 2, PageSize: 1})
	if err != nil || total != 2 || len(rows) != 1 {
		t.Fatalf("pagination failed total=%d len=%d err=%v", total, len(rows), err)
	}
	rows, total, err = repo.ListAdmin(ordercontract.ListFilter{IssuedOnly: true, GuestEmail: split.GuestEmail, Page: 1, PageSize: 20})
	if err != nil || total != 1 || len(rows) != 1 || rows[0].ID != split.ID {
		t.Fatal("buyer filter was not retained")
	}
	_, total, err = repo.ListAdmin(ordercontract.ListFilter{Page: 1, PageSize: 20})
	if err != nil || total != 6 {
		t.Fatal("ordinary order listing changed")
	}
}

func TestAdminRiskShadowFilterExcludesHeldOrders(t *testing.T) {
	_, repo, db := setupAdminSearchRepositoryTest(t)
	now := time.Now()
	orders := []orderdomain.Order{
		{OrderNo: "SHADOW-HIT", Currency: "CNY", Status: "paid", PaymentRiskCandidate: true, PaymentRiskEvaluatedAt: &now},
		{OrderNo: "HELD-HIT", Currency: "CNY", Status: "paid", PaymentRiskCandidate: true, RiskHoldStatus: "pending_review", PaymentRiskEvaluatedAt: &now},
		{OrderNo: "EVALUATED-CLEAN", Currency: "CNY", Status: "paid", PaymentRiskEvaluatedAt: &now},
		{OrderNo: "SHADOW-CLEAN", Currency: "CNY", Status: "paid"},
	}
	if err := db.Create(&orders).Error; err != nil {
		t.Fatal(err)
	}
	rows, total, err := repo.ListAdmin(ordercontract.ListFilter{RiskShadowCandidate: true, SkipRelations: true})
	if err != nil || total != 1 || len(rows) != 1 || rows[0].OrderNo != "SHADOW-HIT" {
		t.Fatalf("expected only observation-mode candidate, got total=%d rows=%d err=%v", total, len(rows), err)
	}
	_, total, err = repo.ListAdmin(ordercontract.ListFilter{RiskEvaluatedOnly: true, SkipRelations: true})
	if err != nil || total != 3 {
		t.Fatalf("expected all three evaluated orders, got total=%d err=%v", total, err)
	}
}

func TestRiskSuccessfulHistoryExcludesUnfulfilledAndRejectedPayments(t *testing.T) {
	_, store, db := setupAdminSearchRepositoryTest(t)
	now := time.Now()
	for i, order := range []orderdomain.Order{
		{Status: "delivered"},
		{Status: "completed", RiskHoldStatus: "released"},
		{Status: "paid"},
		{Status: "fulfilled", RiskHoldStatus: "pending_review"},
		{Status: "delivered", RiskHoldStatus: "rejected"},
		{Status: "refunded"},
	} {
		order.OrderNo = fmt.Sprintf("risk-success-history-%d", i)
		order.UserID = 42
		order.PaidAt = &now
		if err := db.Create(&order).Error; err != nil {
			t.Fatal(err)
		}
	}
	_, count, err := store.ListAdmin(ordercontract.ListFilter{
		UserID: 42, Page: 1, PageSize: 1, RiskSuccessfulOnly: true, SkipRelations: true,
	})
	if err != nil || count != 2 {
		t.Fatalf("successful history count = %v, want 2: %v", count, err)
	}
}

func TestRedemptionRiskBlocksHeldParentHistoricalDelivery(t *testing.T) {
	_, store, db := setupAdminSearchRepositoryTest(t)
	assertRedemptionRiskBlocksHeldParent(t, store, db)
}

func TestPostgresRedemptionRiskBlocksHeldParentHistoricalDelivery(t *testing.T) {
	db := openGuestCredentialPostgresDB(t)
	assertRedemptionRiskBlocksHeldParent(t, New(db, "test-guest-credential-secret-with-32-bytes"), db)
}

func assertRedemptionRiskBlocksHeldParent(t *testing.T, store *Store, db *gorm.DB) {
	t.Helper()
	parent := orderdomain.Order{OrderNo: "risk-parent", RiskHoldStatus: "pending_review"}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatal(err)
	}
	child := orderdomain.Order{OrderNo: "risk-child", ParentID: &parent.ID}
	if err := db.Create(&child).Error; err != nil {
		t.Fatal(err)
	}
	const replaced = "REPLACED-0000-0000-0000-0000"
	archivedDigest := sha256.Sum256([]byte(replaced))
	const code = "TEST-0000-0000-0000-0000-0000"
	delivery := fulfillmentdomain.Fulfillment{
		OrderID: child.ID, Type: "upstream", Status: "delivered",
		RedemptionPreviousHashes: hex.EncodeToString(archivedDigest[:]),
		Payload:                  "OTHER-0000-0000-0000-0000-0000\r\n" + code,
	}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	assertBlocked := func(value string, want bool) {
		t.Helper()
		got, err := store.RedemptionRiskBlocked(t.Context(), sha256.Sum256([]byte(value)))
		if err != nil || got != want {
			t.Fatalf("blocked = %v, want %v, error = %v", got, want, err)
		}
	}
	assertBlocked(code, true)
	assertBlocked(replaced, true)
	assertBlocked("UNKNOWN-0000-0000-0000-0000", false)
	if err := db.Model(&parent).Update("risk_hold_status", "released").Error; err != nil {
		t.Fatal(err)
	}
	assertBlocked(code, false)
	assertBlocked(replaced, false)
	if err := db.Model(&child).Update("risk_hold_status", "rejected").Error; err != nil {
		t.Fatal(err)
	}
	assertBlocked(code, true)
	assertBlocked(replaced, true)
	// Legacy manual rows can have no payload. They must neither make all
	// lookups fail nor hide the archived code from the same held order.
	if err := db.Model(&delivery).UpdateColumn("payload", nil).Error; err != nil {
		t.Fatal(err)
	}
	assertBlocked(replaced, true)
	assertBlocked(code, false)
	assertBlocked("UNKNOWN-0000-0000-0000-0000", false)
}

func TestRedemptionSubmissionClearsOnlyMatchingCode(t *testing.T) {
	_, store, db := setupAdminSearchRepositoryTest(t)
	assertRedemptionSubmissionClearsOnlyMatchingCode(t, store, db)
}

func TestPostgresRedemptionSubmissionClearsOnlyMatchingCode(t *testing.T) {
	db := openGuestCredentialPostgresDB(t)
	assertRedemptionSubmissionClearsOnlyMatchingCode(t, New(db, "test-guest-credential-secret-with-32-bytes"), db)
}

func assertRedemptionSubmissionClearsOnlyMatchingCode(t *testing.T, store *Store, db *gorm.DB) {
	t.Helper()
	now := time.Now()
	const code = "TEST-0000-0000-0000-0000-0000"
	order := orderdomain.Order{OrderNo: "recent-redemption", PaidAt: &now}
	if err := db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	delivery := fulfillmentdomain.Fulfillment{
		OrderID: order.ID, Type: "upstream", Status: "delivered",
		Payload: code, RedemptionProgress: "unused",
	}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{"OTHER-0000-0000-0000-0000-0000", code, code} {
		if err := store.RecordRedemptionSubmission(t.Context(), sha256.Sum256([]byte(candidate))); err != nil {
			t.Fatal(err)
		}
		unused, err := store.HasUnusedRedemption([]uint{order.ID})
		if err != nil || unused != (candidate != code) {
			t.Fatalf("unexpected unused state %v: %v", unused, err)
		}
	}
}

func TestRedemptionEvidenceFiltersParentOrderDeliveries(t *testing.T) {
	_, store, db := setupAdminSearchRepositoryTest(t)
	if err := db.AutoMigrate(&orderriskdomain.RedemptionEvidence{}); err != nil {
		t.Fatal(err)
	}
	parent := orderdomain.Order{OrderNo: "evidence-parent"}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatal(err)
	}
	child := orderdomain.Order{OrderNo: "evidence-child", ParentID: &parent.ID}
	if err := db.Create(&child).Error; err != nil {
		t.Fatal(err)
	}
	code := "TEST-REDEMPTION-CODE-00001"
	if err := db.Create(&fulfillmentdomain.Fulfillment{OrderID: child.ID, Type: "upstream", Status: "delivered", Payload: code}).Error; err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{code, "OTHER-REDEMPTION-CODE-00001"} {
		digest := sha256.Sum256([]byte(value))
		if err := store.RecordRedemptionEvidence(t.Context(), orderriskdomain.RedemptionEvidence{CodeHash: hex.EncodeToString(digest[:]), Operation: "verify", Outcome: "attempt", ClientIP: "127.0.0.1"}); err != nil {
			t.Fatal(err)
		}
	}
	items, count, err := store.ListRedemptionEvidence(t.Context(), "", parent.OrderNo, 1, 20)
	if err != nil || count != 1 || len(items) != 1 {
		t.Fatalf("wanted only the child code record, got %d records and error %v", count, err)
	}
}
