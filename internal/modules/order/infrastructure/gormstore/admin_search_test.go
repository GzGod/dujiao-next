package gormstore

import (
	"fmt"
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
