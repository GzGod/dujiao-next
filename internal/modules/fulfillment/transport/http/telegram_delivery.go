package fulfillmenthttp

import (
	"context"
	"crypto/subtle"
	app "github.com/dujiao-next/internal/modules/fulfillment/application"
	contract "github.com/dujiao-next/internal/modules/fulfillment/contract"
	domain "github.com/dujiao-next/internal/modules/order/domain"
	reseller "github.com/dujiao-next/internal/modules/reseller/contract"
	"github.com/dujiao-next/internal/platform/http/ginutil"
	"github.com/dujiao-next/internal/platform/http/response"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type DeliveryOrderReader interface {
	GetOrderByUserOrderNoForTenant(reseller.TenantContext, string, uint) (*domain.Order, error)
	GetOrderByGuestOrderNoForTenant(reseller.TenantContext, string, string, string) (*domain.Order, error)
	GetOrderForAdmin(uint) (*domain.Order, error)
}
type TelegramDeliveryHandler struct {
	Overview      func(context.Context) map[string]interface{}
	Service       *app.TelegramDeliveryService
	Orders        DeliveryOrderReader
	WebhookSecret string
}

func (h *TelegramDeliveryHandler) RegisterGuest(routes gin.IRoutes) {
	routes.POST("/orders/:order_no/telegram-delivery", h.GuestLink)
}
func (h *TelegramDeliveryHandler) RegisterUser(routes gin.IRoutes) {
	routes.POST("/orders/:order_no/telegram-delivery", h.UserLink)
}
func (h *TelegramDeliveryHandler) RegisterAdmin(routes gin.IRoutes) {
	routes.GET("/telegram-delivery/status", h.BotOverview)
	routes.GET("/orders/:id/telegram-delivery", h.Status)
	routes.POST("/orders/:id/telegram-delivery", h.Send)
}
func (h *TelegramDeliveryHandler) RegisterWebhook(routes gin.IRoutes) {
	routes.POST("/telegram-delivery/webhook", h.Webhook)
}
func (h *TelegramDeliveryHandler) fail(c *gin.Context) {
	ginutil.RespondError(c, response.CodeBadRequest, "error.telegram_delivery_unavailable", nil)
}
func (h *TelegramDeliveryHandler) link(c *gin.Context, order *domain.Order, err error) {
	if err != nil || order == nil {
		h.fail(c)
		return
	}
	url, err := h.Service.Link(order)
	if err != nil {
		h.fail(c)
		return
	}
	response.Success(c, gin.H{"url": url})
}
func (h *TelegramDeliveryHandler) GuestLink(c *gin.Context) {
	email, password, ok := ginutil.GetGuestCredentials(c)
	if !ok {
		h.fail(c)
		return
	}
	order, err := h.Orders.GetOrderByGuestOrderNoForTenant(reseller.TenantOrMain(c.Request.Context()), c.Param("order_no"), email, password)
	h.link(c, order, err)
}
func (h *TelegramDeliveryHandler) UserLink(c *gin.Context) {
	id, ok := ginutil.GetUserID(c)
	if !ok {
		return
	}
	order, err := h.Orders.GetOrderByUserOrderNoForTenant(reseller.TenantOrMain(c.Request.Context()), c.Param("order_no"), id)
	h.link(c, order, err)
}
func (h *TelegramDeliveryHandler) Status(c *gin.Context) {
	id, err := ginutil.ParseParamUint(c, "id")
	if err != nil {
		h.fail(c)
		return
	}
	order, err := h.Orders.GetOrderForAdmin(id)
	if err != nil {
		h.fail(c)
		return
	}
	row, err := h.Service.Status(order)
	if err != nil {
		h.fail(c)
		return
	}
	recipient := ""
	if row != nil {
		recipient = row.ChatID
	}
	response.Success(c, gin.H{"enabled": h.Service.Enabled(), "binding": row, "recipient": recipient})
}
func (h *TelegramDeliveryHandler) Send(c *gin.Context) {
	admin, ok := ginutil.GetAdminID(c)
	if !ok {
		return
	}
	id, err := ginutil.ParseParamUint(c, "id")
	if err != nil {
		h.fail(c)
		return
	}
	var req struct {
		Payload string `json:"payload"`
	}
	if c.ShouldBindJSON(&req) != nil {
		h.fail(c)
		return
	}
	result, err := h.Service.Send(c.Request.Context(), id, admin, req.Payload)
	if err != nil {
		ginutil.RespondError(c, response.CodeBadRequest, "error.telegram_delivery_review", nil)
		return
	}
	response.Success(c, result)
}
func (h *TelegramDeliveryHandler) Webhook(c *gin.Context) {
	if len(h.WebhookSecret) < 32 || subtle.ConstantTimeCompare([]byte(c.GetHeader("X-Telegram-Bot-Api-Secret-Token")), []byte(h.WebhookSecret)) != 1 {
		c.Status(http.StatusForbidden)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32768)

	type actor struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
		IsBot    bool   `json:"is_bot"`
	}
	type message struct {
		Text string `json:"text"`
		Chat struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		} `json:"chat"`
		From actor `json:"from"`
	}
	var update struct {
		ID       int64    `json:"update_id"`
		Message  *message `json:"message"`
		Callback *struct {
			ID      string   `json:"id"`
			Data    string   `json:"data"`
			From    actor    `json:"from"`
			Message *message `json:"message"`
		} `json:"callback_query"`
	}
	if c.ShouldBindJSON(&update) != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	var m *message
	data := ""
	from := actor{}
	if update.Message != nil {
		m = update.Message
		from = m.From
	}
	if update.Callback != nil {
		m = update.Callback.Message
		from = update.Callback.From
		data = update.Callback.Data
	}
	if m == nil || m.Chat.Type != "private" || from.IsBot || from.ID <= 0 || m.Chat.ID != from.ID {
		c.Status(http.StatusOK)
		return
	}
	chat := strconv.FormatInt(from.ID, 10)
	// Authenticate before any administrative order lookup or callback handling.
	if data != "" && chat != h.Service.AdminChatID {
		c.Status(http.StatusOK)
		return
	}
	limitChat := chat
	if chat == h.Service.AdminChatID {
		limitChat = ""
	}
	accepted, err := h.Service.Store.AcceptTelegramUpdate(update.ID, limitChat, time.Now())
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	if !accepted {
		c.Status(http.StatusOK)
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 25*time.Second)
	defer cancel()
	if chat == h.Service.AdminChatID && h.Service.AdminEnabled() {
		if update.Callback != nil {
			if ack, ok := h.Service.Sender.(contract.TelegramCallbackAcknowledger); ok {
				_ = ack.AnswerCallback(ctx, update.Callback.ID)
			}
		}
		_ = h.Service.AdminAction(ctx, chat, data, m.Text)
		c.Status(http.StatusOK)
		return
	}
	parts := strings.Fields(m.Text)
	if len(parts) == 1 && parts[0] == "/start" {
		h.Service.StartHelp(ctx, chat)
	}
	if len(parts) == 2 && parts[0] == "/start" {
		if err := h.Service.Bind(ctx, parts[1], chat, from.Username); err != nil {
			h.Service.StartHelp(ctx, chat)
		}
	}
	c.Status(http.StatusOK)
}

func (h *TelegramDeliveryHandler) BotOverview(c *gin.Context) {
	if _, ok := ginutil.GetAdminID(c); !ok {
		return
	}
	if h.Overview == nil {
		response.Success(c, gin.H{"enabled": false, "connected": false})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 12*time.Second)
	defer cancel()
	response.Success(c, h.Overview(ctx))
}
