package application_test

import (
	"context"
	"errors"
	"github.com/dujiao-next/internal/constants"
	app "github.com/dujiao-next/internal/modules/fulfillment/application"
	contract "github.com/dujiao-next/internal/modules/fulfillment/contract"
	domain "github.com/dujiao-next/internal/modules/fulfillment/domain"
	store "github.com/dujiao-next/internal/modules/fulfillment/infrastructure/gormstore"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	orderstore "github.com/dujiao-next/internal/modules/order/infrastructure/gormstore"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"net/url"
	"strings"
	"testing"
	"time"
)

type deliverySender struct {
	calls   int
	fail    bool
	message contract.TelegramMessage
}

func (s *deliverySender) SendDelivery(_ context.Context, m contract.TelegramMessage) (int64, error) {
	s.calls++
	s.message = m
	if s.fail {
		return 0, errors.New("uncertain")
	}
	return 123, nil
}
func TestTelegramDeliveryBindingSendingAndDuplicateProtection(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "uncertain"}[fail], func(t *testing.T) {
			db := setupFulfillmentServiceTestDB(t)
			if err := db.AutoMigrate(&domain.TelegramDelivery{}); err != nil {
				t.Fatal(err)
			}
			sql, _ := db.DB()
			sql.SetMaxOpenConns(1)
			order := &orderdomain.Order{OrderNo: "TG-ORDER", Status: constants.OrderStatusPaid, Currency: "CNY", Items: []orderdomain.OrderItem{{ProductID: 1, Quantity: 1, FulfillmentType: constants.FulfillmentTypeManual, TitleJSON: jsonmap.JSON{"zh-CN": "Claude Pro"}}}}
			if err := db.Create(order).Error; err != nil {
				t.Fatal(err)
			}
			fs := store.New(db)
			os := orderstore.New(db, "test-guest-credential-secret-with-32-bytes")
			sender := &deliverySender{}
			svc := &app.TelegramDeliveryService{Store: fs, Sender: sender, Fulfillment: app.New(app.Options{OrderStore: os, FulfillmentStore: fs}), BotUsername: "fixture_bot", TokenSecret: strings.Repeat("x", 32), SiteURL: "https://example.test", SupportURL: "https://t.me/support"}
			link, err := svc.Link(order)
			if err != nil {
				t.Fatal(err)
			}
			u, _ := url.Parse(link)
			token := u.Query().Get("start")
			if strings.Contains(token, "TG-ORDER") || len(token) > 64 {
				t.Fatal("unsafe token")
			}
			if err := svc.Bind(context.Background(), token, "42", "buyer"); err != nil {
				t.Fatal(err)
			}
			if err := svc.Bind(context.Background(), token, "43", "thief"); err == nil {
				t.Fatal("stolen binding accepted")
			}
			sender.calls = 0
			sender.fail = fail
			f, err := svc.Send(context.Background(), order.ID, 1, "account@example.test\npassword: fixture")
			if fail {
				if err == nil {
					t.Fatal("uncertain send marked successful")
				}
			} else {
				if err != nil || f == nil {
					t.Fatal(err)
				}
			}
			_, _ = svc.Send(context.Background(), order.ID, 1, "account@example.test\npassword: fixture")
			if sender.calls != 1 {
				t.Fatalf("duplicate send: %d", sender.calls)
			}
			loaded, _ := os.GetByID(order.ID)
			if fail && loaded.Status != constants.OrderStatusPaid {
				t.Fatal("failed send delivered order")
			}
			if !fail && (loaded.Status != constants.OrderStatusDelivered || loaded.Fulfillment.Payload != "account@example.test\npassword: fixture") {
				t.Fatal("missing website delivery")
			}
			row, _ := fs.GetTelegramDelivery(order.ID)
			want := "delivered"
			if fail {
				want = "review"
			}
			if row.State != want {
				t.Fatalf("state %s", row.State)
			}
			if !strings.HasPrefix(sender.message.SupportURL, "https://t.me/") {
				t.Fatal("missing support button")
			}
		})
	}
}
func TestTelegramExpiredTokenAndConcurrentSendClaim(t *testing.T) {
	db := setupFulfillmentServiceTestDB(t)
	_ = db.AutoMigrate(&domain.TelegramDelivery{})
	s := store.New(db)
	if err := s.IssueTelegramToken(1, "expired", time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BindTelegramToken("expired", "42", "user", time.Now()); err == nil {
		t.Fatal("expired token accepted")
	}
	_ = s.IssueTelegramToken(2, "valid", time.Now().Add(time.Hour))
	_, err := s.BindTelegramToken("valid", "42", "user", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.ClaimTelegramDelivery(2, "hash")
	if err != nil || !first {
		t.Fatal(err)
	}
	second, err := s.ClaimTelegramDelivery(2, "hash")
	if err != nil || second {
		t.Fatal("duplicate claim accepted")
	}
}
