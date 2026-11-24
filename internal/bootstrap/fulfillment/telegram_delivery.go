package fulfillmentwiring

import (
	"github.com/dujiao-next/internal/app/container"
	app "github.com/dujiao-next/internal/modules/fulfillment/application"
	contract "github.com/dujiao-next/internal/modules/fulfillment/contract"
	api "github.com/dujiao-next/internal/modules/fulfillment/infrastructure/telegramapi"
	transport "github.com/dujiao-next/internal/modules/fulfillment/transport/http"
	"os"
	"strings"
)

func NewTelegramDeliveryHandler(c *container.Container) *transport.TelegramDeliveryHandler {
	store, _ := c.FulfillmentStore.(contract.TelegramDeliveryStore)
	token := strings.TrimSpace(os.Getenv("DELIVERY_TELEGRAM_BOT_TOKEN"))
	service := &app.TelegramDeliveryService{Store: store, Fulfillment: c.FulfillmentService, BotUsername: strings.TrimPrefix(strings.TrimSpace(os.Getenv("DELIVERY_TELEGRAM_BOT_USERNAME")), "@"), TokenSecret: os.Getenv("DELIVERY_TELEGRAM_LINK_SECRET"), SiteURL: "https://xuegaogx.cc", SupportURL: "https://t.me/xuegaozhanshen"}
	if token != "" && len(os.Getenv("DELIVERY_TELEGRAM_WEBHOOK_SECRET")) >= 32 {
		service.Sender = &api.Client{Token: token}
	}
	return &transport.TelegramDeliveryHandler{Service: service, Orders: c.OrderService, WebhookSecret: os.Getenv("DELIVERY_TELEGRAM_WEBHOOK_SECRET")}
}
