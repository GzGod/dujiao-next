package upstreamhttp

import (
	"github.com/dujiao-next/internal/i18n"
	"github.com/dujiao-next/internal/platform/http/ginutil"
	"github.com/dujiao-next/internal/platform/http/response"
	"net/http"
	"strconv"
	"strings"

	downstreamcontract "github.com/dujiao-next/internal/modules/downstreamcallback/contract"
	downstreamdomain "github.com/dujiao-next/internal/modules/downstreamcallback/domain"
	procurementdomain "github.com/dujiao-next/internal/modules/procurement/domain"
	walletcontract "github.com/dujiao-next/internal/modules/wallet/contract"
	walletdomain "github.com/dujiao-next/internal/modules/wallet/domain"
	"github.com/gin-gonic/gin"
)

func RegisterAuthenticatedRoutes(upstream gin.IRoutes, handler *Handler) {
	if upstream == nil || handler == nil {
		panic("upstream authenticated routes: required dependency is nil")
	}
	upstream.POST("/ping", handler.Ping)
	upstream.GET("/capabilities", handler.Capabilities)
	upstream.GET("/account", handler.Account)
	upstream.GET("/account/transactions", handler.AccountTransactions)
	upstream.GET("/orders", handler.ListOrders)
	upstream.GET("/orders/by-external/:external", handler.GetOrderByExternal)
	upstream.GET("/categories", handler.ListCategories)
	upstream.GET("/products", handler.ListProducts)
	upstream.GET("/products/:id", handler.GetProduct)
	upstream.POST("/orders", handler.CreateOrder)
	upstream.GET("/orders/:id", handler.GetOrder)
	upstream.POST("/orders/:id/cancel", handler.CancelOrder)
	upstream.GET("/orders/:id/redemption", handler.RedemptionState)
	upstream.POST("/orders/:id/replace", handler.ReplaceCode)
	upstream.POST("/orders/:id/refund-requests", handler.RequestRefund)
}

type walletTransactionReader interface {
	ListTransactions(walletcontract.TransactionListFilter) ([]walletdomain.Transaction, int64, error)
}
type downstreamReferenceLister interface {
	ListByCredentialID(uint, downstreamcontract.RefListFilter) ([]downstreamdomain.OrderRef, int64, error)
}

func (h *Handler) Capabilities(c *gin.Context) {
	if getUpstreamUserID(c) == 0 || getUpstreamCredentialID(c) == 0 {
		errorResponse(c, http.StatusUnauthorized, "unauthorized", i18n.T(i18n.ResolveLocale(c), "error.downstream.unauthorized"))
		return
	}
	successResponse(c, gin.H{"ok": true, "api_version": "1.1", "available_operations": []string{"ping", "account", "account_transactions", "categories", "products", "create_order", "get_order", "get_order_by_external", "list_orders", "cancel_unpaid_order", "transaction_callback", "redemption_state", "replace_code", "request_refund"}, "customer_redemption": gin.H{"base_url": "https://redeem.xuegaogx.cc/api/v1/redemptions", "operations": []string{"verify", "submit", "progress"}, "requires_registered_origin": true}, "unavailable_operations": []string{"automatic_refund", "upgrade_quote", "purchase_upgrade"}})
}

func (h *Handler) AccountTransactions(c *gin.Context) {
	userID := getUpstreamUserID(c)
	if userID == 0 {
		errorResponse(c, http.StatusUnauthorized, "unauthorized", i18n.T(i18n.ResolveLocale(c), "error.downstream.unauthorized"))
		return
	}
	reader, ok := h.Wallet.(walletTransactionReader)
	if !ok {
		errorResponse(c, http.StatusServiceUnavailable, "unavailable", i18n.T(i18n.ResolveLocale(c), "error.downstream.unavailable"))
		return
	}
	page, size := resourcePagination(c)
	rows, total, err := reader.ListTransactions(walletcontract.TransactionListFilter{UserID: userID, Page: page, PageSize: size})
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "internal_error", i18n.T(i18n.ResolveLocale(c), "error.downstream.internal_error"))
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		items = append(items, gin.H{"id": row.ID, "order_id": row.OrderID, "type": row.Type, "direction": row.Direction, "amount": row.Amount.StringFixed(2), "balance_before": row.BalanceBefore.StringFixed(2), "balance_after": row.BalanceAfter.StringFixed(2), "currency": row.Currency, "created_at": row.CreatedAt})
	}
	successResponse(c, gin.H{"ok": true, "items": items, "total": total, "page": page, "page_size": size})
}

func (h *Handler) GetOrderByExternal(c *gin.Context) {
	credentialID, userID := getUpstreamCredentialID(c), getUpstreamUserID(c)
	if credentialID == 0 || userID == 0 {
		errorResponse(c, http.StatusUnauthorized, "unauthorized", i18n.T(i18n.ResolveLocale(c), "error.downstream.unauthorized"))
		return
	}
	external := strings.TrimSpace(c.Param("external"))
	if external == "" || len(external) > 128 {
		errorResponse(c, http.StatusBadRequest, "bad_request", i18n.T(i18n.ResolveLocale(c), "error.downstream.bad_request"))
		return
	}
	ref, err := h.DownstreamRefs.GetByCredentialAndDownstreamNo(credentialID, external)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "internal_error", i18n.T(i18n.ResolveLocale(c), "error.downstream.internal_error"))
		return
	}
	if ref == nil {
		errorResponse(c, http.StatusNotFound, "order_not_found", i18n.T(i18n.ResolveLocale(c), "error.downstream.order_not_found"))
		return
	}
	c.Params = append(c.Params, gin.Param{Key: "id", Value: strconv.FormatUint(uint64(ref.OrderID), 10)})
	h.GetOrder(c)
}

func (h *Handler) ListOrders(c *gin.Context) {
	credentialID, userID := getUpstreamCredentialID(c), getUpstreamUserID(c)
	if credentialID == 0 || userID == 0 {
		errorResponse(c, http.StatusUnauthorized, "unauthorized", i18n.T(i18n.ResolveLocale(c), "error.downstream.unauthorized"))
		return
	}
	reader, ok := h.DownstreamRefs.(downstreamReferenceLister)
	if !ok {
		errorResponse(c, http.StatusServiceUnavailable, "unavailable", i18n.T(i18n.ResolveLocale(c), "error.downstream.unavailable"))
		return
	}
	page, size := resourcePagination(c)
	refs, total, err := reader.ListByCredentialID(credentialID, downstreamcontract.RefListFilter{Page: page, PageSize: size})
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "internal_error", i18n.T(i18n.ResolveLocale(c), "error.downstream.internal_error"))
		return
	}
	items := make([]gin.H, 0, len(refs))
	for _, ref := range refs {
		order, err := h.Orders.GetOrderByUser(ref.OrderID, userID)
		if err != nil || order == nil {
			errorResponse(c, http.StatusInternalServerError, "internal_error", i18n.T(i18n.ResolveLocale(c), "error.downstream.internal_error"))
			return
		}
		items = append(items, gin.H{"order_id": order.ID, "order_no": order.OrderNo, "downstream_order_no": ref.DownstreamOrderNo, "status": order.Status, "amount": order.TotalAmount.StringFixed(2), "refunded_amount": order.RefundedAmount.StringFixed(2), "currency": order.Currency, "created_at": order.CreatedAt})
	}
	successResponse(c, gin.H{"ok": true, "items": items, "total": total, "page": page, "page_size": size})
}

func resourcePagination(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size
}

type orderReferenceReader interface {
	GetByOrderID(uint) (*downstreamdomain.OrderRef, error)
}

// ownedProcurement resolves the credential-owned parent order to its single
// delivered procurement. Multi-item/manual deliveries are not silently changed.
func (h *Handler) ownedProcurement(c *gin.Context) *procurementdomain.Order {
	userID, credentialID := getUpstreamUserID(c), getUpstreamCredentialID(c)
	if userID == 0 || credentialID == 0 {
		errorResponse(c, 401, "unauthorized", i18n.T(i18n.ResolveLocale(c), "error.downstream.unauthorized"))
		return nil
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		errorResponse(c, 400, "bad_request", i18n.T(i18n.ResolveLocale(c), "error.downstream.bad_request"))
		return nil
	}
	reader, ok := h.DownstreamRefs.(orderReferenceReader)
	if !ok {
		errorResponse(c, 503, "unavailable", i18n.T(i18n.ResolveLocale(c), "error.downstream.unavailable"))
		return nil
	}
	ref, err := reader.GetByOrderID(uint(id))
	if err != nil {
		errorResponse(c, 500, "internal_error", i18n.T(i18n.ResolveLocale(c), "error.downstream.internal_error"))
		return nil
	}
	if ref == nil || ref.ApiCredentialID != credentialID {
		errorResponse(c, 404, "order_not_found", i18n.T(i18n.ResolveLocale(c), "error.downstream.order_not_found"))
		return nil
	}
	order, err := h.Orders.GetOrderByUser(uint(id), userID)
	if err != nil || order == nil {
		errorResponse(c, 404, "order_not_found", i18n.T(i18n.ResolveLocale(c), "error.downstream.order_not_found"))
		return nil
	}
	localNo := order.OrderNo
	if len(order.Children) == 1 {
		localNo = order.Children[0].OrderNo
	} else if len(order.Children) > 1 {
		errorResponse(c, 409, "unsupported_delivery", i18n.T(i18n.ResolveLocale(c), "error.downstream.unsupported_delivery"))
		return nil
	}
	proc, err := h.Procurements.GetByLocalOrderNo(localNo)
	if err != nil || proc == nil {
		errorResponse(c, 409, "unsupported_delivery", i18n.T(i18n.ResolveLocale(c), "error.downstream.unsupported_delivery"))
		return nil
	}
	return proc
}
func (h *Handler) RedemptionState(c *gin.Context) {
	proc := h.ownedProcurement(c)
	if proc == nil {
		return
	}
	if h.AfterSales == nil {
		errorResponse(c, 503, "unavailable", i18n.T(i18n.ResolveLocale(c), "error.downstream.unavailable"))
		return
	}
	result, err := h.AfterSales.Inspect(c.Request.Context(), proc.ID)
	if err != nil {
		errorResponse(c, 409, "state_unavailable", i18n.T(i18n.ResolveLocale(c), "error.downstream.state_unavailable"))
		return
	}
	successResponse(c, gin.H{"ok": true, "redemption": result, "after_sale_kind": proc.AfterSaleKind, "after_sale_state": proc.AfterSaleState, "after_sale_request_id": proc.AfterSaleRequestID})
}
func (h *Handler) ReplaceCode(c *gin.Context) {
	proc := h.ownedProcurement(c)
	if proc == nil {
		return
	}
	if h.AfterSales == nil {
		errorResponse(c, 503, "unavailable", i18n.T(i18n.ResolveLocale(c), "error.downstream.unavailable"))
		return
	}
	var body struct {
		RequestID string `json:"request_id" binding:"required,max=64"`
		Revision  int    `json:"expected_code_revision" binding:"required,min=1"`
	}
	if c.ShouldBindJSON(&body) != nil {
		errorResponse(c, 400, "bad_request", i18n.T(i18n.ResolveLocale(c), "error.downstream.bad_request"))
		return
	}
	result, err := h.AfterSales.Replace(c.Request.Context(), proc.ID, body.RequestID, body.Revision)
	if err != nil {
		errorResponse(c, 409, "replacement_unconfirmed", i18n.T(i18n.ResolveLocale(c), "error.downstream.replacement_unconfirmed"))
		return
	}
	successResponse(c, gin.H{"ok": true, "order_id": c.Param("id"), "code": result.Code, "code_revision": result.Revision, "financial_status": result.Financial, "progress_status": result.Progress})
}
func (h *Handler) RequestRefund(c *gin.Context) {
	proc := h.ownedProcurement(c)
	if proc == nil {
		return
	}
	if h.AfterSales == nil {
		errorResponse(c, 503, "unavailable", i18n.T(i18n.ResolveLocale(c), "error.downstream.unavailable"))
		return
	}
	var body struct {
		RequestID string `json:"request_id" binding:"required,max=64"`
		Reason    string `json:"reason" binding:"required,max=1000"`
	}
	if c.ShouldBindJSON(&body) != nil {
		errorResponse(c, 400, "bad_request", i18n.T(i18n.ResolveLocale(c), "error.downstream.bad_request"))
		return
	}
	if err := h.AfterSales.RequestRefund(c.Request.Context(), proc.ID, body.RequestID, body.Reason); err != nil {
		errorResponse(c, 409, "refund_request_rejected", i18n.T(i18n.ResolveLocale(c), "error.downstream.refund_request_rejected"))
		return
	}
	updated, err := h.Procurements.GetByLocalOrderNo(proc.LocalOrderNo)
	if err != nil || updated == nil {
		errorResponse(c, 503, "state_unavailable", i18n.T(i18n.ResolveLocale(c), "error.downstream.state_unavailable"))
		return
	}
	successResponse(c, gin.H{"ok": true, "status": updated.AfterSaleState, "request_id": body.RequestID, "refunded": updated.AfterSaleState == "refunded"})
}

// ReviewAfterSale is mounted exclusively behind the admin finance permissions.
func (h *Handler) ReviewAfterSale(c *gin.Context) {
	adminID, ok := ginutil.GetAdminID(c)
	if !ok {
		return
	}
	id, err := ginutil.ParseParamUint(c, "id")
	if err != nil {
		ginutil.RespondError(c, response.CodeBadRequest, "error.bad_request", err)
		return
	}
	var body struct {
		Approve *bool  `json:"approve" binding:"required"`
		Remark  string `json:"remark" binding:"max=1000"`
	}
	if c.ShouldBindJSON(&body) != nil {
		ginutil.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}
	if h.AfterSales == nil {
		ginutil.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}
	if err := h.AfterSales.ReviewRefund(c.Request.Context(), id, *body.Approve, adminID, body.Remark); err != nil {
		ginutil.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}
	response.Success(c, gin.H{"reviewed": true})
}

func (h *Handler) Account(c *gin.Context) {
	userID := getUpstreamUserID(c)
	if userID == 0 {
		errorResponse(c, 401, "unauthorized", i18n.T(i18n.ResolveLocale(c), "error.downstream.unauthorized"))
		return
	}
	account, err := h.Wallet.GetAccount(userID)
	if err != nil || account == nil {
		errorResponse(c, 503, "account_unavailable", i18n.T(i18n.ResolveLocale(c), "error.downstream.account_unavailable"))
		return
	}
	currency, err := h.Settings.GetSiteCurrency("CNY")
	if err != nil {
		errorResponse(c, 503, "account_unavailable", i18n.T(i18n.ResolveLocale(c), "error.downstream.account_unavailable"))
		return
	}
	successResponse(c, gin.H{"ok": true, "balance": account.Balance.StringFixed(2), "currency": currency})
}

// InspectAdminRedemption is read-only and never returns the credential itself.
func (h *Handler) InspectAdminRedemption(c *gin.Context) {
	if _, ok := ginutil.GetAdminID(c); !ok {
		return
	}
	id, err := ginutil.ParseParamUint(c, "id")
	if err != nil {
		ginutil.RespondError(c, response.CodeBadRequest, "error.bad_request", nil)
		return
	}
	if h.AfterSales == nil {
		ginutil.RespondError(c, response.CodeInternal, "error.downstream.state_unavailable", nil)
		return
	}
	result, err := h.AfterSales.Inspect(c.Request.Context(), id)
	if err != nil || result == nil {
		ginutil.RespondError(c, response.CodeBadRequest, "error.downstream.state_unavailable", nil)
		return
	}
	response.Success(c, result)
}
