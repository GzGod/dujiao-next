package fulfillmenthttp

import (
	"context"
	"crypto/subtle"
	app "github.com/dujiao-next/internal/modules/fulfillment/application"
	domain "github.com/dujiao-next/internal/modules/order/domain"
	reseller "github.com/dujiao-next/internal/modules/reseller/contract"
	"github.com/dujiao-next/internal/platform/http/ginutil"
	"github.com/dujiao-next/internal/platform/http/response"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"strings"
)

type DeliveryOrderReader interface {
	GetOrderByUserOrderNoForTenant(reseller.TenantContext, string, uint) (*domain.Order, error)
	GetOrderByGuestOrderNoForTenant(reseller.TenantContext, string, string, string) (*domain.Order, error)
	GetOrderForAdmin(uint) (*domain.Order, error)
}
type TelegramDeliveryHandler struct {
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
	var update struct {
		Message *struct {
			Text string `json:"text"`
			Chat struct {
				ID   int64  `json:"id"`
				Type string `json:"type"`
			} `json:"chat"`
			From struct {
				ID       int64  `json:"id"`
				Username string `json:"username"`
				IsBot    bool   `json:"is_bot"`
			} `json:"from"`
		} `json:"message"`
	}
	if c.ShouldBindJSON(&update) != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	m := update.Message
	if m != nil && m.Chat.Type == "private" && !m.From.IsBot && m.Chat.ID == m.From.ID {
		parts := strings.Fields(m.Text)
		if len(parts) == 1 && parts[0] == "/start" {
			h.Service.StartHelp(c.Request.Context(), strconv.FormatInt(m.Chat.ID, 10))
		}
		if len(parts) == 2 && parts[0] == "/start" {
			if err := h.Service.Bind(context.WithoutCancel(c.Request.Context()), parts[1], strconv.FormatInt(m.Chat.ID, 10), m.From.Username); err != nil {
				h.Service.StartHelp(c.Request.Context(), strconv.FormatInt(m.Chat.ID, 10))
			}
		}
	}
	// Do not expose token/binding information or retry a delivered update.
	c.Status(http.StatusOK)
}
