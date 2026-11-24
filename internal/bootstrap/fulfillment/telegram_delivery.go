package fulfillmentwiring

import (
	"context"
	"errors"
	"github.com/dujiao-next/internal/app/container"
	app "github.com/dujiao-next/internal/modules/fulfillment/application"
	contract "github.com/dujiao-next/internal/modules/fulfillment/contract"
	api "github.com/dujiao-next/internal/modules/fulfillment/infrastructure/telegramapi"
	transport "github.com/dujiao-next/internal/modules/fulfillment/transport/http"
	"os"
	"strings"
	"sync"
	"time"
)

func NewTelegramDeliveryHandler(c *container.Container) *transport.TelegramDeliveryHandler {
	store, _ := c.FulfillmentStore.(contract.TelegramDeliveryStore)
	token := strings.TrimSpace(os.Getenv("DELIVERY_TELEGRAM_BOT_TOKEN"))
	service := &app.TelegramDeliveryService{Store: store, Fulfillment: c.FulfillmentService, BotUsername: strings.TrimPrefix(strings.TrimSpace(os.Getenv("DELIVERY_TELEGRAM_BOT_USERNAME")), "@"), TokenSecret: os.Getenv("DELIVERY_TELEGRAM_LINK_SECRET"), SiteURL: "https://xuegaogx.cc", SupportURL: "https://t.me/xuegaozhanshen"}
	if token != "" && len(os.Getenv("DELIVERY_TELEGRAM_WEBHOOK_SECRET")) >= 32 {
		service.Sender = &api.Client{Token: token}
	}
	service.AdminChatID = strings.TrimSpace(os.Getenv("DELIVERY_TELEGRAM_ADMIN_ID"))
	service.AuthorizeAdmin = func() (uint, error) {
		username := strings.TrimSpace(os.Getenv("DELIVERY_TELEGRAM_ADMIN_USERNAME"))
		if username == "" || c.AdminStore == nil {
			return 0, errors.New("delivery admin not configured")
		}
		admin, err := c.AdminStore.GetByUsername(username)
		if err != nil || admin == nil || !admin.IsSuper {
			return 0, errors.New("delivery admin unavailable")
		}
		return admin.ID, nil
	}
	return &transport.TelegramDeliveryHandler{Service: service, Orders: c.OrderService, WebhookSecret: os.Getenv("DELIVERY_TELEGRAM_WEBHOOK_SECRET")}
}

var deliveryPollOnce sync.Once

func StartTelegramDeliveryNotifications(c *container.Container) {
	service := NewTelegramDeliveryHandler(c).Service
	if !service.AdminEnabled() {
		return
	}
	deliveryPollOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
				_ = service.NotifyAdminOrders(ctx, false)
				cancel()
			}
		}()
	})
}
