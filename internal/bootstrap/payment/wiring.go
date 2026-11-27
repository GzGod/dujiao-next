package paymentbootstrap

import (
	"context"
	"github.com/dujiao-next/internal/app/container"
	"github.com/dujiao-next/internal/modules/fulfillment/contract"
	"github.com/dujiao-next/internal/modules/fulfillment/infrastructure/telegramapi"
	"github.com/dujiao-next/internal/modules/payment/infrastructure/gateway/gmcashier"
	paymenttransport "github.com/dujiao-next/internal/modules/payment/transport/http"
	paymentcallbacktransport "github.com/dujiao-next/internal/modules/payment/transport/http/callback"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"os"
	"strings"
)

// Handlers is the complete payment HTTP entrypoint set assembled at the
// application boundary.
type Handlers struct {
	Latest       *paymenttransport.LatestHandler
	Write        *paymenttransport.WriteHandler
	Admin        *paymenttransport.AdminHandler
	AdminChannel *paymenttransport.AdminChannelHandler
	Webhook      *paymenttransport.WebhookHandler
	Callback     *paymentcallbacktransport.Handler
	EpayRedirect *paymenttransport.EpayRedirectHandler
}

// New assembles payment transports from application services and HTTP handlers.
func New(c *container.Container) Handlers {
	guestOrders := guestOrderLookupAdapter{orders: c.OrderService}
	userOrders := userOrderLookupAdapter{orders: c.OrderService}
	alerter := exceptionAlerterAdapter{notifications: c.NotificationService}

	latest := paymenttransport.NewLatestHandler(guestOrders, userOrders, pendingLookupAdapter{payments: c.PaymentStore})
	if recorder, ok := c.PaymentStore.(interface {
		RecordQRSave(context.Context, uint, uint, string, string) (int, bool, string, error)
	}); ok {
		latest.RecordQR = recorder.RecordQRSave
	}
	latest.QRAlert = alerter.EnqueuePaymentExceptionAlert
	if token, chat := strings.TrimSpace(os.Getenv("DELIVERY_TELEGRAM_BOT_TOKEN")), strings.TrimSpace(os.Getenv("DELIVERY_TELEGRAM_ADMIN_ID")); token != "" && chat != "" {
		bot := &telegramapi.Client{Token: token}
		latest.QRAlert = func(_, _, _ string, data jsonmap.JSON) error {
			message, _ := data["message"].(string)
			_, err := bot.SendDelivery(context.Background(), contract.TelegramMessage{ChatID: chat, Text: "付款码风控提醒\n" + message})
			return err
		}
	}
	latest.Cashier = gmcashier.Client{}
	return Handlers{
		Latest: latest,
		Write: paymenttransport.NewWriteHandler(
			guestOrders,
			userOrders,
			writerAdapter{payments: c.PaymentService},
		),
		Admin: paymenttransport.NewAdminHandler(
			adminQueryAdapter{payments: c.PaymentService},
			adminChannelLookupAdapter{channels: c.PaymentChannelStore},
			adminOrderLookupAdapter{orders: c.OrderStore},
			adminRechargeLookupAdapter{wallets: c.WalletRepo},
			adminPaymentConfirmerAdapter{payments: c.PaymentService},
		),
		AdminChannel: paymenttransport.NewAdminChannelHandler(
			adminChannelCatalogAdapter{payments: c.PaymentService, channels: c.PaymentChannelStore},
		),
		Webhook: paymenttransport.NewWebhookHandler(
			webhookServiceAdapter{payments: c.PaymentService},
			alerter,
		),
		Callback: paymentcallbacktransport.NewHandler(
			callbackServiceAdapter{payments: c.PaymentService},
			c.PaymentStore,
			c.PaymentChannelStore,
			alerter,
		),
		EpayRedirect: paymenttransport.NewEpayRedirectHandler(c.PaymentStore),
	}
}
