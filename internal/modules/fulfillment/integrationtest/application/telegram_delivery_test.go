package application_test

import (
	"context"
	"errors"
	"fmt"
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
	calls    int
	fail     bool
	message  contract.TelegramMessage
	messages []contract.TelegramMessage
}

func (s *deliverySender) SendDelivery(_ context.Context, m contract.TelegramMessage) (int64, error) {
	s.calls++
	s.message = m
	s.messages = append(s.messages, m)
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
	for _, id := range []uint{1, 2} {
		if err := db.Create(&orderdomain.Order{ID: id, OrderNo: fmt.Sprintf("TOKEN-%d", id), Status: constants.OrderStatusPaid}).Error; err != nil {
			t.Fatal(err)
		}
	}
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

func TestAdminBotOnlyOwnerCanReceiveAndConfirmDelivery(t *testing.T) {
	db := setupFulfillmentServiceTestDB(t)
	if err := db.AutoMigrate(&domain.TelegramDelivery{}, &domain.TelegramAdminNotice{}, &domain.TelegramAdminSession{}, &domain.TelegramUpdateReceipt{}); err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	order := &orderdomain.Order{OrderNo: "ADMIN-BOT-ORDER", Status: constants.OrderStatusPaid, Currency: "CNY", Items: []orderdomain.OrderItem{{ProductID: 1, Quantity: 1, FulfillmentType: "manual", TitleJSON: jsonmap.JSON{"zh-CN": "Claude Pro"}}}}
	if err := db.Create(order).Error; err != nil {
		t.Fatal(err)
	}
	fs := store.New(db)
	os := orderstore.New(db, "test-guest-credential-secret-with-32-bytes")
	sender := &deliverySender{}
	svc := &app.TelegramDeliveryService{Store: fs, Sender: sender, Fulfillment: app.New(app.Options{OrderStore: os, FulfillmentStore: fs}), BotUsername: "fixture_bot", TokenSecret: strings.Repeat("s", 32), SiteURL: "https://example.test", SupportURL: "https://t.me/support", AdminChatID: "5760638358", AuthorizeAdmin: func() (uint, error) { return 7, nil }}
	ctx := context.Background()
	if err := svc.AdminAction(ctx, "42", "", "/orders"); err == nil || sender.calls != 0 {
		t.Fatal("customer obtained admin notifications")
	}
	for _, text := range []string{"/start", "/pending", "📦 未发货订单"} {
		if err := svc.AdminAction(ctx, "42", "", text); err == nil || sender.calls != 0 {
			t.Fatal("customer accessed administrator menu")
		}
	}
	if err := svc.AdminAction(ctx, "5760638358", "", "/start"); err != nil || !sender.message.AdminMenu || sender.message.ChatID != "5760638358" {
		t.Fatal("administrator menu not private")
	}
	if err := svc.NotifyAdminOrders(ctx, false); err != nil {
		t.Fatal(err)
	}
	if sender.message.ChatID != "5760638358" || !strings.Contains(sender.message.Text, order.OrderNo) || len(sender.message.Buttons) != 2 {
		t.Fatal("notification not exclusively to admin")
	}
	calls := sender.calls
	if err := svc.NotifyAdminOrders(ctx, false); err != nil || sender.calls != calls {
		t.Fatal("duplicate paid notification")
	}
	link, err := svc.Link(order)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(link)
	if err := svc.Bind(ctx, u.Query().Get("start"), "42", "customer"); err != nil {
		t.Fatal(err)
	}
	if sender.message.ChatID != "5760638358" || !strings.Contains(sender.message.Text, "42") || !strings.Contains(sender.message.Text, "customer") || !strings.Contains(sender.message.Text, order.OrderNo) || len(sender.message.Buttons) != 2 {
		t.Fatal("binding did not notify exclusive administrator with identity and order actions")
	}
	if len(sender.message.Entities) != 1 || sender.message.Entities[0].URL != "tg://user?id=42" {
		t.Fatal("bound customer is not a clickable private-chat link")
	}
	calls = sender.calls
	if err := svc.Bind(ctx, u.Query().Get("start"), "42", "customer"); err != nil || sender.calls != calls {
		t.Fatal("repeated start generated notification")
	}
	action := fmt.Sprintf("deliver:%d", order.ID)
	if err := svc.AdminAction(ctx, "43", action, ""); err == nil {
		t.Fatal("forwarded admin button accepted")
	}
	if err := svc.AdminAction(ctx, "5760638358", action, ""); err != nil {
		t.Fatal(err)
	}
	preparation, _ := fs.GetAdminSession("5760638358")
	if len(sender.message.Buttons) != 1 || sender.message.Buttons[0].Callback != "cancel:"+preparation.Nonce {
		t.Fatal("preparation missing cancellation button")
	}
	if err := svc.AdminAction(ctx, "5760638358", "cancel:"+preparation.Nonce, ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.AdminAction(ctx, "5760638358", "", "do not deliver cancelled content"); err != nil {
		t.Fatal(err)
	}
	cancelled, _ := fs.GetAdminSession("5760638358")
	if cancelled.Stage != "consumed" {
		t.Fatal("cancelled session accepted draft")
	}
	if err := svc.AdminAction(ctx, "5760638358", action, ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.AdminAction(ctx, "5760638358", "", "account: fixture-secret"); err != nil {
		t.Fatal(err)
	}
	session, err := fs.GetAdminSession("5760638358")
	if err != nil {
		t.Fatal(err)
	}
	if session.Stage != "confirm" || strings.Contains(session.Ciphertext, "fixture-secret") {
		t.Fatal("draft not encrypted or missing confirmation")
	}
	confirm := "confirm:" + session.Nonce
	if err := svc.AdminAction(ctx, "42", confirm, ""); err == nil {
		t.Fatal("customer confirmed admin draft")
	}
	if err := svc.AdminAction(ctx, "5760638358", confirm, ""); err != nil {
		t.Fatal(err)
	}
	loaded, _ := os.GetByID(order.ID)
	foundCustomerDelivery := false
	for _, m := range sender.messages {
		if strings.Contains(m.Text, "account: fixture-secret") && m.ChatID == "42" {
			foundCustomerDelivery = true
		}
		if strings.Contains(m.Text, "人工订单已付款") && m.ChatID != "5760638358" {
			t.Fatal("admin notification leaked")
		}
	}
	if !foundCustomerDelivery {
		t.Fatal("delivery not addressed to bound customer")
	}
	if loaded.Status != constants.OrderStatusDelivered || loaded.Fulfillment.Payload != "account: fixture-secret" {
		t.Fatal("bot delivery not persisted")
	}
	calls = sender.calls
	_ = svc.AdminAction(ctx, "5760638358", confirm, "")
	if sender.calls != calls {
		t.Fatal("confirm replay sent another message")
	}
}

func TestDeliveryReservationBlocksRefundAndManualDeliveryAndHidesDraft(t *testing.T) {
	db := setupFulfillmentServiceTestDB(t)
	_ = db.AutoMigrate(&domain.TelegramDelivery{})
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	order := &orderdomain.Order{OrderNo: "RESERVED-ORDER", Status: constants.OrderStatusPaid, Items: []orderdomain.OrderItem{{ProductID: 1, Quantity: 1, FulfillmentType: "manual", TitleJSON: jsonmap.JSON{"zh-CN": "fixture"}}}}
	if err := db.Create(order).Error; err != nil {
		t.Fatal(err)
	}
	fs := store.New(db)
	os := orderstore.New(db, "test-guest-credential-secret-with-32-bytes")
	_ = fs.IssueTelegramToken(order.ID, "reservation", time.Now().Add(time.Hour))
	if _, err := fs.BindTelegramToken("reservation", "42", "user", time.Now()); err != nil {
		t.Fatal(err)
	}
	ok, err := fs.ReserveTelegramDelivery(order.ID, 7, "hash")
	if err != nil || !ok {
		t.Fatal(err)
	}
	record, _ := fs.GetByOrderID(order.ID)
	if record.Payload != "" || record.Status != "pending" {
		t.Fatal("unconfirmed delivery content exposed")
	}
	if err := os.UpdateFields(order.ID, map[string]interface{}{"status": "refunded"}); err == nil {
		t.Fatal("refund status bypassed hold")
	}
	if err := os.UpdateFields(order.ID, map[string]interface{}{"refunded_amount": 1}); err == nil {
		t.Fatal("refund money bypassed hold")
	}
	svc := app.New(app.Options{OrderStore: os, FulfillmentStore: fs})
	if _, err := svc.CreateManual(app.CreateManualInput{OrderID: order.ID, AdminID: 8, Payload: "other"}); err == nil {
		t.Fatal("manual delivery bypassed hold")
	}
	if err := fs.ReleaseTelegramDelivery(order.ID); err != nil {
		t.Fatal(err)
	}
	if err := os.UpdateFields(order.ID, map[string]interface{}{"status": "refunded"}); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.BindTelegramToken("reservation", "43", "other", time.Now()); err == nil {
		t.Fatal("refunded order token accepted")
	}
}
func TestWebhookDedupAndCustomerRateLimit(t *testing.T) {
	db := setupFulfillmentServiceTestDB(t)
	_ = db.AutoMigrate(&domain.TelegramUpdateReceipt{})
	s := store.New(db)
	now := time.Now()
	a, err := s.AcceptTelegramUpdate(100, "42", now)
	if err != nil || !a {
		t.Fatal(err)
	}
	a, err = s.AcceptTelegramUpdate(100, "42", now.Add(time.Second))
	if err != nil || a {
		t.Fatal("replay accepted")
	}
	a, err = s.AcceptTelegramUpdate(101, "42", now.Add(100*time.Millisecond))
	if err != nil || a {
		t.Fatal("rate limit bypass")
	}
	a, err = s.AcceptTelegramUpdate(102, "42", now.Add(2*time.Second))
	if err != nil || !a {
		t.Fatal("rate limit did not recover")
	}
}

func TestAdministratorMustResolveUncertainSendWithoutAutomaticRetry(t *testing.T) {
	for _, resolution := range []string{"received:", "unsent:"} {
		t.Run(resolution, func(t *testing.T) {
			db := setupFulfillmentServiceTestDB(t)
			_ = db.AutoMigrate(&domain.TelegramDelivery{}, &domain.TelegramAdminNotice{}, &domain.TelegramAdminSession{}, &domain.TelegramUpdateReceipt{})
			order := &orderdomain.Order{OrderNo: "REVIEW-ORDER", Status: constants.OrderStatusPaid, Items: []orderdomain.OrderItem{{ProductID: 1, Quantity: 1, FulfillmentType: "manual", TitleJSON: jsonmap.JSON{"zh-CN": "fixture"}}}}
			if err := db.Create(order).Error; err != nil {
				t.Fatal(err)
			}
			fs := store.New(db)
			os := orderstore.New(db, "test-guest-credential-secret-with-32-bytes")
			sender := &deliverySender{}
			svc := &app.TelegramDeliveryService{Store: fs, Sender: sender, Fulfillment: app.New(app.Options{OrderStore: os, FulfillmentStore: fs}), BotUsername: "fixture_bot", TokenSecret: strings.Repeat("s", 32), AdminChatID: "5760638358", AuthorizeAdmin: func() (uint, error) { return 7, nil }}
			ctx := context.Background()
			link, _ := svc.Link(order)
			u, _ := url.Parse(link)
			if err := svc.Bind(ctx, u.Query().Get("start"), "42", "customer"); err != nil {
				t.Fatal(err)
			}
			_ = svc.AdminAction(ctx, "5760638358", fmt.Sprintf("deliver:%d", order.ID), "")
			_ = svc.AdminAction(ctx, "5760638358", "", "sensitive fixture")
			session, _ := fs.GetAdminSession("5760638358")
			sender.fail = true
			_ = svc.AdminAction(ctx, "5760638358", "confirm:"+session.Nonce, "")
			session, _ = fs.GetAdminSession("5760638358")
			if session.Stage != "review" {
				t.Fatal("uncertain delivery not awaiting review")
			}
			sender.fail = false
			calls := sender.calls
			if err := svc.AdminAction(ctx, "5760638358", resolution+session.Nonce, ""); err != nil {
				t.Fatal(err)
			}
			for _, m := range sender.messages[calls:] {
				if m.ChatID == "42" {
					t.Fatal("recovery automatically resent sensitive content")
				}
			}
			loaded, _ := os.GetByID(order.ID)
			if resolution == "received:" && loaded.Status != constants.OrderStatusDelivered {
				t.Fatal("confirmed receipt not finalized")
			}
			if resolution == "unsent:" {
				session, _ = fs.GetAdminSession("5760638358")
				if loaded.Status != constants.OrderStatusPaid || session.Stage != "awaiting" {
					t.Fatal("unsent recovery did not require fresh confirmation")
				}
			}
		})
	}
}

type blockingDeliverySender struct{ entered, release chan struct{} }

func (s *blockingDeliverySender) SendDelivery(_ context.Context, m contract.TelegramMessage) (int64, error) {
	if strings.Contains(m.Text, "race-payload") {
		close(s.entered)
		<-s.release
	}
	return 321, nil
}
func TestInFlightTelegramSendBlocksConcurrentRefundAndSecondDelivery(t *testing.T) {
	db := setupFulfillmentServiceTestDB(t)
	_ = db.AutoMigrate(&domain.TelegramDelivery{})
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	parent := &orderdomain.Order{OrderNo: "RACE-PARENT", Status: constants.OrderStatusPaid}
	if err := db.Create(parent).Error; err != nil {
		t.Fatal(err)
	}
	leaf := &orderdomain.Order{ParentID: &parent.ID, OrderNo: "RACE-CHILD", Status: constants.OrderStatusPaid, Items: []orderdomain.OrderItem{{ProductID: 1, Quantity: 1, FulfillmentType: "manual", TitleJSON: jsonmap.JSON{"zh-CN": "fixture"}}}}
	if err := db.Create(leaf).Error; err != nil {
		t.Fatal(err)
	}
	fs := store.New(db)
	os := orderstore.New(db, "test-guest-credential-secret-with-32-bytes")
	sender := &blockingDeliverySender{entered: make(chan struct{}), release: make(chan struct{})}
	svc := &app.TelegramDeliveryService{Store: fs, Sender: sender, Fulfillment: app.New(app.Options{OrderStore: os, FulfillmentStore: fs}), BotUsername: "fixture_bot", TokenSecret: strings.Repeat("s", 32)}
	fullParent, _ := os.GetByID(parent.ID)
	link, err := svc.Link(fullParent)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(link)
	if err := svc.Bind(context.Background(), u.Query().Get("start"), "42", "customer"); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := svc.Send(context.Background(), leaf.ID, 7, "race-payload"); result <- err }()
	select {
	case <-sender.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("send never reached Telegram")
	}
	if err := os.UpdateFields(parent.ID, map[string]interface{}{"refunded_amount": 1}); err == nil {
		t.Error("parent refund bypassed in-flight hold")
	}
	if err := os.UpdateStatus(leaf.ID, constants.OrderStatusRefunded, nil); err == nil {
		t.Error("child refund bypassed in-flight hold")
	}
	if _, err := svc.Fulfillment.CreateManual(app.CreateManualInput{OrderID: leaf.ID, AdminID: 8, Payload: "second"}); err == nil {
		t.Error("parallel manual delivery accepted")
	}
	if _, err := svc.Send(context.Background(), leaf.ID, 7, "race-payload"); err == nil {
		t.Error("parallel bot delivery accepted")
	}
	close(sender.release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	loaded, _ := os.GetByID(leaf.ID)
	if loaded.Status != constants.OrderStatusDelivered {
		t.Fatal("original delivery not finalized")
	}
}

func TestAdminMarkDeliveredAndCancelPreparation(t *testing.T) {
	for _, scenario := range []string{"mark", "cancel", "refunded", "reserved"} {
		t.Run(scenario, func(t *testing.T) {
			db := setupFulfillmentServiceTestDB(t)
			if err := db.AutoMigrate(&domain.TelegramDelivery{}, &domain.TelegramAdminNotice{}, &domain.TelegramAdminSession{}); err != nil {
				t.Fatal(err)
			}
			sql, _ := db.DB()
			sql.SetMaxOpenConns(1)
			order := &orderdomain.Order{OrderNo: "MARK-" + scenario, Status: constants.OrderStatusPaid, Items: []orderdomain.OrderItem{{ProductID: 1, Quantity: 1, FulfillmentType: "manual", TitleJSON: jsonmap.JSON{"zh-CN": "Claude"}}}}
			if err := db.Create(order).Error; err != nil {
				t.Fatal(err)
			}
			fs := store.New(db)
			os := orderstore.New(db, "test-guest-credential-secret-with-32-bytes")
			sender := &deliverySender{}
			svc := &app.TelegramDeliveryService{Store: fs, Sender: sender, Fulfillment: app.New(app.Options{OrderStore: os, FulfillmentStore: fs}), BotUsername: "fixture_bot", TokenSecret: strings.Repeat("s", 32), AdminChatID: "5760638358", AuthorizeAdmin: func() (uint, error) { return 7, nil }}
			ctx := context.Background()
			action := fmt.Sprintf("markdelivered:%d", order.ID)
			if err := svc.AdminAction(ctx, "42", action, ""); err == nil || sender.calls != 0 {
				t.Fatal("non-admin accepted")
			}
			if err := svc.AdminAction(ctx, "5760638358", action, ""); err != nil {
				t.Fatal(err)
			}
			session, _ := fs.GetAdminSession("5760638358")
			if session.Stage != "mark_delivered" {
				t.Fatal("missing confirmation")
			}
			loaded, _ := os.GetByID(order.ID)
			if loaded.Status != constants.OrderStatusPaid {
				t.Fatal("marked before confirmation")
			}
			if scenario == "cancel" {
				_ = svc.AdminAction(ctx, "5760638358", "cancel:"+session.Nonce, "")
			} else if scenario == "refunded" {
				if err := os.UpdateStatus(order.ID, constants.OrderStatusRefunded, nil); err != nil {
					t.Fatal(err)
				}
			} else if scenario == "reserved" {
				if err := db.Create(&domain.TelegramDelivery{OrderID: order.ID, State: "bound", ChatID: "42", TokenHash: "fixture"}).Error; err != nil {
					t.Fatal(err)
				}
				if ok, err := fs.ReserveTelegramDelivery(order.ID, 7, "pending-hash"); err != nil || !ok {
					t.Fatalf("reserve: %v", err)
				}
			}
			_ = svc.AdminAction(ctx, "5760638358", "markconfirm:"+session.Nonce, "")
			_ = svc.AdminAction(ctx, "5760638358", "markconfirm:"+session.Nonce, "")
			loaded, _ = os.GetByID(order.ID)
			if (loaded.Status == constants.OrderStatusDelivered) != (scenario == "mark") {
				t.Fatalf("unexpected status: %s", loaded.Status)
			}
			for _, message := range sender.messages {
				if message.ChatID != "5760638358" {
					t.Fatal("sent delivery content to customer")
				}
			}
			if scenario == "mark" {
				var count int64
				db.Model(&domain.Fulfillment{}).Where("order_id = ?", order.ID).Count(&count)
				if count != 1 {
					t.Fatalf("duplicate fulfillment: %d", count)
				}
			}
		})
	}
}

type editableAdminSender struct {
	deliverySender
	edits []contract.TelegramMessage
}

func (s *editableAdminSender) EditDelivery(_ context.Context, _ int64, m contract.TelegramMessage) error {
	s.edits = append(s.edits, m)
	return nil
}
func TestAdminOrderNoticesReconcileDeliveryAndRefund(t *testing.T) {
	for _, status := range []string{"delivered", "refunded"} {
		t.Run(status, func(t *testing.T) {
			db := setupFulfillmentServiceTestDB(t)
			if err := db.AutoMigrate(&domain.TelegramAdminNotice{}, &domain.TelegramDelivery{}); err != nil {
				t.Fatal(err)
			}
			order := &orderdomain.Order{OrderNo: "NOTICE-" + status, Status: status}
			if err := db.Create(order).Error; err != nil {
				t.Fatal(err)
			}
			fs := store.New(db)
			os := orderstore.New(db, "test-guest-credential-secret-with-32-bytes")
			sender := &editableAdminSender{}
			svc := &app.TelegramDeliveryService{Store: fs, Sender: sender, Fulfillment: app.New(app.Options{OrderStore: os, FulfillmentStore: fs}), BotUsername: "fixture_bot", TokenSecret: strings.Repeat("s", 32), AdminChatID: "5760638358", AuthorizeAdmin: func() (uint, error) { return 7, nil }}
			for _, id := range []int64{101, 102} {
				if err := fs.SaveAdminNotice(&domain.TelegramAdminNotice{OrderID: order.ID, ChatID: "5760638358", MessageID: id}); err != nil {
					t.Fatal(err)
				}
			}
			if err := svc.SyncAdminNotices(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(sender.edits) != 2 {
				t.Fatalf("not all prior messages updated: %d", len(sender.edits))
			}
			for _, m := range sender.edits {
				if m.ChatID != "5760638358" || len(m.Buttons) != 0 || !strings.Contains(m.Text, order.OrderNo) {
					t.Fatal("unsafe or incomplete notice edit")
				}
			}
			if err := svc.SyncAdminNotices(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(sender.edits) != 2 {
				t.Fatal("unchanged terminal notice edited again")
			}
		})
	}
}

func TestPendingOrdersCursorDoesNotSkipAfterDelivery(t *testing.T) {
	db := setupFulfillmentServiceTestDB(t)
	for i := 0; i < 13; i++ {
		status := "paid"
		if i == 12 {
			status = "refunded"
		}
		order := &orderdomain.Order{OrderNo: fmt.Sprintf("PAGE-%d", i), Status: status, Items: []orderdomain.OrderItem{{ProductID: 1, Quantity: 1, FulfillmentType: "manual", TitleJSON: jsonmap.JSON{"zh-CN": "fixture"}}}}
		if err := db.Create(order).Error; err != nil {
			t.Fatal(err)
		}
	}
	fs := store.New(db)
	ids, err := fs.ListTelegramPendingPage(0)
	if err != nil || len(ids) != 11 {
		t.Fatalf("first page: %v %v", ids, err)
	}
	previous, err := fs.PreviousTelegramPendingCursor(ids[9])
	if err != nil || previous != 0 {
		t.Fatalf("previous first page: %d %v", previous, err)
	}
	cursor := ids[9]
	if err := db.Model(&orderdomain.Order{}).Where("id <= ?", cursor).Update("status", "delivered").Error; err != nil {
		t.Fatal(err)
	}
	next, err := fs.ListTelegramPendingPage(cursor)
	if err != nil || len(next) != 2 || next[0] != ids[10] {
		t.Fatalf("cursor skipped orders: %v %v", next, err)
	}
}
