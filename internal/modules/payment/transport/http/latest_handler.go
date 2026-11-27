package paymenthttp

import (
	"context"
	"errors"
	"fmt"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"strings"
	"time"

	paymentpresenter "github.com/dujiao-next/internal/modules/payment/transport/presenter"

	paymentdomain "github.com/dujiao-next/internal/modules/payment/domain"

	orderdomain "github.com/dujiao-next/internal/modules/order/domain"

	ginutil "github.com/dujiao-next/internal/platform/http/ginutil"

	"github.com/dujiao-next/internal/constants"
	reseller "github.com/dujiao-next/internal/modules/reseller/contract"
	"github.com/dujiao-next/internal/platform/http/response"

	"github.com/gin-gonic/gin"
)

var (
	ErrOrderNotFound      = errors.New("order not found")
	ErrGuestOrderNotFound = errors.New("guest order not found")
)

// GuestOrderLookup 游客订单查询端口。
type GuestOrderLookup interface {
	GetOrderByGuestOrderNoForTenant(tenant reseller.TenantContext, orderNo, email, password string) (*orderdomain.Order, error)
	GetOrderByGuestForTenant(tenant reseller.TenantContext, orderID uint, email, password string) (*orderdomain.Order, error)
}

// UserOrderLookup 用户订单查询端口。
type UserOrderLookup interface {
	GetOrderByUserOrderNoForTenant(tenant reseller.TenantContext, orderNo string, userID uint) (*orderdomain.Order, error)
	GetOrderByUserForTenant(tenant reseller.TenantContext, orderID, userID uint) (*orderdomain.Order, error)
}

// PendingPaymentLookup 待支付记录查询端口。
type PendingPaymentLookup interface {
	GetLatestPendingByOrder(orderID uint, now time.Time) (*paymentdomain.Payment, error)
}

// LatestGuestPaymentQuery 游客最新待支付查询参数。
type LatestGuestPaymentQuery struct {
	OrderNo string `form:"order_no" binding:"required"`
}

// LatestPaymentQuery 用户最新待支付查询参数。
type LatestPaymentQuery struct {
	OrderNo string `form:"order_no" binding:"required"`
}

// LatestHandler 处理前台最新待支付查询 HTTP。
type CashierClient interface {
	Cashier(context.Context, string, string, string) (jsonmap.JSON, error)
}

type LatestHandler struct {
	RecordQR    func(context.Context, uint, uint, string, string) (int, bool, string, error)
	AckQRAlert  func(uint, string) error
	QRAlert     func(string, string, string, jsonmap.JSON) error
	Cashier     CashierClient
	guestOrders GuestOrderLookup
	userOrders  UserOrderLookup
	payments    PendingPaymentLookup
}

func NewLatestHandler(guestOrders GuestOrderLookup, userOrders UserOrderLookup, payments PendingPaymentLookup) *LatestHandler {
	if guestOrders == nil || userOrders == nil || payments == nil {
		panic("payment latest handler: required dependency is nil")
	}
	return &LatestHandler{guestOrders: guestOrders, userOrders: userOrders, payments: payments}
}

func tenantFromRequest(c *gin.Context) reseller.TenantContext {
	return reseller.TenantOrMain(c.Request.Context())
}

// GetGuestLatestPayment 获取游客最新待支付记录
func (h *LatestHandler) GetGuestLatestPayment(c *gin.Context) {
	var query LatestGuestPaymentQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		ginutil.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	email, password, ok := ginutil.GetGuestCredentials(c)
	if !ok || email == "" {
		ginutil.RespondError(c, response.CodeBadRequest, "error.guest_email_required", nil)
		return
	}
	if password == "" {
		ginutil.RespondError(c, response.CodeBadRequest, "error.guest_password_required", nil)
		return
	}

	order, err := h.guestOrders.GetOrderByGuestOrderNoForTenant(tenantFromRequest(c), query.OrderNo, email, password)
	if err != nil {
		if errors.Is(err, ErrGuestOrderNotFound) {
			ginutil.RespondError(c, response.CodeNotFound, "error.guest_order_not_found", nil)
			return
		}
		ginutil.RespondError(c, response.CodeInternal, "error.order_fetch_failed", err)
		return
	}
	h.respondLatestPayment(c, order)
}

// GetLatestPayment 获取用户最新待支付记录
func (h *LatestHandler) GetLatestPayment(c *gin.Context) {
	uid, ok := ginutil.GetUserID(c)
	if !ok {
		return
	}

	var query LatestPaymentQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		ginutil.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}

	order, err := h.userOrders.GetOrderByUserOrderNoForTenant(tenantFromRequest(c), query.OrderNo, uid)
	if err != nil {
		if errors.Is(err, ErrOrderNotFound) {
			ginutil.RespondError(c, response.CodeNotFound, "error.order_not_found", nil)
			return
		}
		ginutil.RespondError(c, response.CodeInternal, "error.order_fetch_failed", err)
		return
	}
	h.respondLatestPayment(c, order)
}

func (h *LatestHandler) respondLatestPayment(c *gin.Context, order *orderdomain.Order) {
	if order.ParentID != nil || order.QRWaitPaymentID != 0 || order.QRAlipaySaveCount >= 3 {
		ginutil.RespondError(c, response.CodeBadRequest, "error.payment_invalid", nil)
		return
	}
	if order.Status != constants.OrderStatusPendingPayment {
		ginutil.RespondError(c, response.CodeBadRequest, "error.order_status_invalid", nil)
		return
	}
	if order.ExpiresAt != nil && !order.ExpiresAt.After(time.Now()) {
		ginutil.RespondError(c, response.CodeBadRequest, "error.order_status_invalid", nil)
		return
	}

	payment, err := h.payments.GetLatestPendingByOrder(order.ID, time.Now())
	if err != nil {
		ginutil.RespondError(c, response.CodeInternal, "error.payment_fetch_failed", err)
		return
	}
	if payment == nil {
		ginutil.RespondError(c, response.CodeNotFound, "error.payment_not_found", nil)
		return
	}

	if c.GetBool("gm_cashier") {
		var input struct {
			Network string `json:"network"`
			Token   string `json:"token"`
		}
		if c.Request.Method == "POST" {
			if err := c.ShouldBindJSON(&input); err != nil || input.Network == "" || input.Token == "" {
				ginutil.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
				return
			}
		}
		if h.Cashier == nil || (payment.ProviderType != "epusdt" && payment.ProviderType != "bepusdt") {
			ginutil.RespondError(c, response.CodeBadRequest, "error.payment_invalid", nil)
			return
		}
		data, err := h.Cashier.Cashier(c.Request.Context(), payment.PayURL, input.Network, input.Token)
		if err != nil {
			ginutil.RespondError(c, response.CodeBadRequest, "error.payment_fetch_failed", nil)
			return
		}
		c.Header("Cache-Control", "no-store")
		response.Success(c, data)
		return
	}
	response.Success(c, paymentpresenter.NewLatestPaymentResp(payment, order.OrderNo))
}

func (h *LatestHandler) GuestCashier(c *gin.Context) {
	c.Set("gm_cashier", true)
	h.GetGuestLatestPayment(c)
}
func (h *LatestHandler) UserCashier(c *gin.Context) { c.Set("gm_cashier", true); h.GetLatestPayment(c) }

func (h *LatestHandler) QRSave(guest bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			PaymentID uint   `json:"payment_id" binding:"required,gt=0"`
			OrderNo   string `json:"order_no" binding:"required,max=64"`
			EventID   string `json:"event_id" binding:"required,uuid4"`
			Kind      string `json:"kind" binding:"required,oneof=contextmenu dragstart"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			ginutil.RespondBindError(c, err)
			return
		}
		var o *orderdomain.Order
		var err error
		if guest {
			email, password, ok := ginutil.GetGuestCredentials(c)
			if !ok {
				return
			}
			o, err = h.guestOrders.GetOrderByGuestOrderNoForTenant(tenantFromRequest(c), req.OrderNo, email, password)
		} else {
			uid, ok := ginutil.GetUserID(c)
			if !ok {
				return
			}
			o, err = h.userOrders.GetOrderByUserOrderNoForTenant(tenantFromRequest(c), req.OrderNo, uid)
		}
		if err != nil || o == nil {
			ginutil.RespondError(c, response.CodeNotFound, "error.order_not_found", nil)
			return
		}
		if h.RecordQR == nil {
			ginutil.RespondError(c, response.CodeInternal, "error.internal_error", nil)
			return
		}
		count, alert, orderEmail, err := h.RecordQR(c.Request.Context(), o.ID, req.PaymentID, req.EventID, req.Kind)
		if err != nil {
			ginutil.RespondError(c, response.CodeBadRequest, "error.payment_invalid", nil)
			return
		}
		if alert {
			err = h.QRAlert("POST", "/payments/qr-event", c.ClientIP(), qrAlertDetails(o, orderEmail, count))
			if err == nil && h.AckQRAlert != nil {
				err = h.AckQRAlert(o.ID, req.EventID)
			}
			if err != nil {
				ginutil.RequestLog(c).Warnw("qr_alert_pending_retry", "order_id", o.ID)
			}
		}
		response.Success(c, gin.H{"count": count})
	}
}

func qrAlertDetails(order *orderdomain.Order, email string, count int) jsonmap.JSON {
	items := order.Items
	if len(items) == 0 {
		for _, child := range order.Children {
			items = append(items, child.Items...)
		}
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		name := fmt.Sprintf("#%d", item.ProductID)
		for _, locale := range []string{"zh-CN", "zh-TW", "en-US"} {
			if value, ok := item.TitleJSON[locale].(string); ok && strings.TrimSpace(value) != "" {
				name = value
				break
			}
		}
		names = append(names, fmt.Sprintf("%s × %d", name, item.Quantity))
	}
	products := strings.Join(names, "、")
	amount := order.TotalAmount.Decimal.StringFixed(2) + " " + order.Currency
	message := fmt.Sprintf("订单号：%s\n商品：%s\n订单金额：%s\n下单邮箱：%s\n疑似保存付款码：%d 次", order.OrderNo, products, amount, email, count)
	return jsonmap.JSON{"order_no": order.OrderNo, "reason": "qr_save_gesture", "count": count, "products": products, "amount": amount, "order_email": email, "message": message, "alert_type": "付款码疑似保存"}
}
