package upstreamhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	walletcontract "github.com/dujiao-next/internal/modules/wallet/contract"
	walletdomain "github.com/dujiao-next/internal/modules/wallet/domain"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dujiao-next/internal/constants"
	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"
	downstreamcallbackdomain "github.com/dujiao-next/internal/modules/downstreamcallback/domain"
	fulfillmentdomain "github.com/dujiao-next/internal/modules/fulfillment/domain"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	procurementcontract "github.com/dujiao-next/internal/modules/procurement/contract"
	procurementdomain "github.com/dujiao-next/internal/modules/procurement/domain"
	siteconnectiondomain "github.com/dujiao-next/internal/modules/siteconnection/domain"
	"github.com/dujiao-next/internal/shared/jsonmap"
	upstreamadapter "github.com/dujiao-next/internal/upstream"

	"github.com/gin-gonic/gin"
)

func TestMapCallbackStatus(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{name: "delivered keep delivered", input: "delivered", expect: "delivered"},
		{name: "completed map delivered", input: "completed", expect: "delivered"},
		{name: "fulfilled map delivered", input: "fulfilled", expect: "delivered"},
		{name: "canceled keep canceled", input: "canceled", expect: "canceled"},
		{name: "cancelled map canceled", input: "cancelled", expect: "canceled"},
		{name: "refunded keep refunded", input: "refunded", expect: "refunded"},
		{name: "partially refunded keep value", input: "partially_refunded", expect: "partially_refunded"},
		{name: "trim and lower", input: "  ReFuNdEd  ", expect: "refunded"},
		{name: "fallback normalized raw", input: "PROCESSING", expect: "processing"},
	}

	for _, tc := range tests {
		got := mapCallbackStatus(tc.input)
		if got != tc.expect {
			t.Fatalf("%s: want %q got %q", tc.name, tc.expect, got)
		}
	}
}

type stubConnections struct {
	conn *siteconnectiondomain.Connection
}

func (s stubConnections) GetByApiKey(string) (*siteconnectiondomain.Connection, error) {
	return s.conn, nil
}

type stubSecrets struct{}

func (stubSecrets) DecryptSecret(encrypted string) (string, error) { return encrypted, nil }

type stubProcurements struct {
	order   *procurementdomain.Order
	handled bool
}

func (s *stubProcurements) GetByLocalOrderNo(string) (*procurementdomain.Order, error) {
	return s.order, nil
}

func (s *stubProcurements) HandleUpstreamCallback(uint, string, *procurementcontract.Fulfillment) error {
	s.handled = true
	return nil
}

// TestHandleCallbackOwnership 保证回调只能由采购单所属连接、且上游订单号一致时才被受理。
func TestHandleCallbackOwnership(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name         string
		connectionID uint
		upstreamID   uint
		payloadID    uint
		wantHandled  bool
	}{
		{name: "同连接同上游单号放行", connectionID: 1, upstreamID: 88, payloadID: 88, wantHandled: true},
		{name: "上游单号未落库时只校验连接", connectionID: 1, upstreamID: 0, payloadID: 88, wantHandled: true},
		{name: "其它连接冒用本地订单号", connectionID: 2, upstreamID: 88, payloadID: 88, wantHandled: false},
		{name: "上游订单号不匹配", connectionID: 1, upstreamID: 88, payloadID: 99, wantHandled: false},
	}

	for _, tc := range cases {
		procurements := &stubProcurements{order: &procurementdomain.Order{
			ID:              7,
			ConnectionID:    tc.connectionID,
			LocalOrderNo:    "LOCAL-1",
			UpstreamOrderID: tc.upstreamID,
		}}
		handler := &Handler{Dependencies: Dependencies{
			Connections: stubConnections{conn: &siteconnectiondomain.Connection{
				ID: 1, ApiKey: "key-1", ApiSecret: "secret-1", Status: "active",
			}},
			ConnectionSecrets: stubSecrets{},
			Procurements:      procurements,
		}}

		timestamp := time.Now().Unix()
		body, _ := json.Marshal(callbackPayload{
			Event:             "order.fulfilled",
			OrderID:           tc.payloadID,
			DownstreamOrderNo: "LOCAL-1",
			Status:            "delivered",
			Timestamp:         timestamp,
		})

		request := httptest.NewRequest(http.MethodPost, "/api/v1/upstream/callback", bytes.NewReader(body))
		request.Header.Set(upstreamadapter.HeaderApiKey, "key-1")
		request.Header.Set(upstreamadapter.HeaderTimestamp, fmt.Sprintf("%d", timestamp))
		request.Header.Set(upstreamadapter.HeaderSignature,
			upstreamadapter.Sign("secret-1", http.MethodPost, "/api/v1/upstream/callback", timestamp, body))

		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = request
		handler.HandleCallback(c)

		if procurements.handled != tc.wantHandled {
			t.Fatalf("%s: handled=%v want %v (body %s)", tc.name, procurements.handled, tc.wantHandled, recorder.Body.String())
		}
	}
}

type formTestProducts struct{ product productdomain.Product }

func (s formTestProducts) GetByID(string) (*productdomain.Product, error) {
	p := s.product
	return &p, nil
}

type formTestSKUs struct{}

func (formTestSKUs) GetByID(uint) (*productdomain.ProductSKU, error) {
	return &productdomain.ProductSKU{ID: 19, ProductID: 18, IsActive: true}, nil
}

type formTestOrders struct {
	Orders
	received CreateOrderInput
}

func (s *formTestOrders) CreateOrder(input CreateOrderInput) (*orderdomain.Order, error) {
	s.received = input
	return &orderdomain.Order{ID: 100, OrderNo: "FORM-TEST", Status: constants.OrderStatusPendingPayment}, nil
}

type formTestPayments struct{}

func (formTestPayments) CreatePayment(CreatePaymentInput) (*CreatePaymentResult, error) {
	return &CreatePaymentResult{OrderPaid: true}, nil
}

type formTestRefs struct{}

func (formTestRefs) Create(*downstreamcallbackdomain.OrderRef) error { return nil }
func (formTestRefs) GetByCredentialAndDownstreamNo(uint, string) (*downstreamcallbackdomain.OrderRef, error) {
	return nil, nil
}

type formTestSettings struct{ Settings }

func (formTestSettings) GetSiteCurrency(string) (string, error) { return "CNY", nil }

// 接收端不得按交付类型丢弃 manual_form_data：对接商品（upstream）同样可能带必填表单。
func TestCreateOrderPassesManualFormDataForAllFulfillmentTypes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, kind := range []string{constants.FulfillmentTypeManual, constants.FulfillmentTypeAuto, constants.FulfillmentTypeUpstream} {
		t.Run(kind, func(t *testing.T) {
			orders := &formTestOrders{}
			handler := &Handler{Dependencies: Dependencies{
				ProductRepository: formTestProducts{productdomain.Product{ID: 18, IsActive: true, FulfillmentType: kind}},
				SKUs:              formTestSKUs{}, Orders: orders, Payments: formTestPayments{},
				DownstreamRefs: formTestRefs{}, Settings: formTestSettings{},
			}}
			body, _ := json.Marshal(map[string]interface{}{
				"sku_id": 19, "quantity": 1, "manual_form_data": map[string]interface{}{"x_handle": "fixture_user"},
			})
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/upstream/orders", bytes.NewReader(body))
			ctx.Request.Header.Set("Content-Type", "application/json")
			ctx.Set(upstreamUserIDKey, uint(1))
			ctx.Set(upstreamCredentialIDKey, uint(1))
			handler.CreateOrder(ctx)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if got := orders.received.ManualFormData["18"]["x_handle"]; got != "fixture_user" {
				t.Fatalf("manual form data dropped for %s: %#v", kind, orders.received.ManualFormData)
			}
		})
	}
}

type canceledRefOrders struct{ formTestOrders }

func (s *canceledRefOrders) GetOrderByUser(uint, uint) (*orderdomain.Order, error) {
	return &orderdomain.Order{ID: 50, OrderNo: "OLD", Status: constants.OrderStatusCanceled}, nil
}

type existingRefs struct{ formTestRefs }

func (existingRefs) GetByCredentialAndDownstreamNo(uint, string) (*downstreamcallbackdomain.OrderRef, error) {
	return &downstreamcallbackdomain.OrderRef{OrderID: 50}, nil
}

// 余额不足导致旧单被取消后，下游用同一单号重试必须真正重新下单，而不是拿回已取消的旧单。
func TestCreateOrderRecreatesWhenSameDownstreamNoWasCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	orders := &canceledRefOrders{}
	handler := &Handler{Dependencies: Dependencies{
		ProductRepository: formTestProducts{productdomain.Product{ID: 18, IsActive: true, FulfillmentType: constants.FulfillmentTypeAuto}},
		SKUs:              formTestSKUs{}, Orders: orders, Payments: formTestPayments{},
		DownstreamRefs: existingRefs{}, Settings: formTestSettings{},
	}}
	body, _ := json.Marshal(map[string]interface{}{"sku_id": 19, "quantity": 1, "downstream_order_no": "LOCAL-1"})
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/upstream/orders", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set(upstreamUserIDKey, uint(1))
	ctx.Set(upstreamCredentialIDKey, uint(1))
	handler.CreateOrder(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if orders.received.Items == nil {
		t.Fatalf("expected a new order to be created, body=%s", recorder.Body.String())
	}
}

type scopedWallet struct {
	Wallet
	filter walletcontract.TransactionListFilter
	calls  int
}

func (s *scopedWallet) ListTransactions(filter walletcontract.TransactionListFilter) ([]walletdomain.Transaction, int64, error) {
	s.filter = filter
	s.calls++
	return []walletdomain.Transaction{{ID: 8, UserID: filter.UserID, Reference: "private-internal-reference", Remark: "private-admin-note"}}, 1, nil
}

type scopedRefs struct {
	DownstreamOrderReferences
	credential uint
	external   string
	ref        *downstreamcallbackdomain.OrderRef
}

func (s *scopedRefs) GetByCredentialAndDownstreamNo(id uint, no string) (*downstreamcallbackdomain.OrderRef, error) {
	s.credential = id
	s.external = no
	return s.ref, nil
}
func (s *scopedRefs) GetByOrderID(uint) (*downstreamcallbackdomain.OrderRef, error) {
	return s.ref, nil
}
func TestAccountTransactionsForceAuthenticatedUserAndLimit(t *testing.T) {
	wallet := &scopedWallet{}
	handler := &Handler{Dependencies: Dependencies{Wallet: wallet}}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/account/transactions?user_id=999&page_size=10000", nil)
	c.Set(upstreamUserIDKey, uint(42))
	handler.AccountTransactions(c)
	if w.Code != 200 || wallet.filter.UserID != 42 || wallet.filter.PageSize != 100 {
		t.Fatalf("wrong account scope: %v %v", wallet.filter, w.Code)
	}
	if strings.Contains(w.Body.String(), "private-") {
		t.Fatal("private ledger metadata exposed")
	}
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/account/transactions", nil)
	handler.AccountTransactions(c)
	if w.Code != 401 || wallet.calls != 1 {
		t.Fatal("unauthenticated request reached ledger")
	}
}
func TestExternalLookupAndAfterSaleRespectCredential(t *testing.T) {
	refs := &scopedRefs{}
	handler := &Handler{Dependencies: Dependencies{DownstreamRefs: refs}}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/orders/by-external/EXTERNAL", nil)
	c.Params = gin.Params{{Key: "external", Value: "EXTERNAL"}}
	c.Set(upstreamUserIDKey, uint(42))
	c.Set(upstreamCredentialIDKey, uint(7))
	handler.GetOrderByExternal(c)
	if w.Code != 404 || refs.credential != 7 || refs.external != "EXTERNAL" {
		t.Fatal("lookup did not scope external number to credential")
	}
	refs.ref = &downstreamcallbackdomain.OrderRef{OrderID: 12, ApiCredentialID: 8}
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/orders/12/replace", nil)
	c.Params = gin.Params{{Key: "id", Value: "12"}}
	c.Set(upstreamUserIDKey, uint(42))
	c.Set(upstreamCredentialIDKey, uint(7))
	handler.ReplaceCode(c)
	if w.Code != 404 {
		t.Fatal("foreign credential order reached after-sale operation")
	}
}

type adminRedemptionProbe struct{ inspected uint }

func (*adminRedemptionProbe) Inspect(context.Context, uint) (*procurementcontract.RedemptionState, error) {
	panic("unexpected customer inspection")
}

func (p *adminRedemptionProbe) InspectForAudit(_ context.Context, id uint) (*procurementcontract.RedemptionState, error) {
	p.inspected = id
	return &procurementcontract.RedemptionState{Code: "private-test-code", Progress: "unused", Revision: 2}, nil
}
func (*adminRedemptionProbe) ReviewRefund(context.Context, uint, bool, uint, string) error {
	panic("unexpected mutation")
}
func (*adminRedemptionProbe) Replace(context.Context, uint, string, int) (*procurementcontract.RedemptionState, error) {
	panic("unexpected mutation")
}
func (*adminRedemptionProbe) RequestRefund(context.Context, uint, string, string) error {
	panic("unexpected mutation")
}
func TestAdminRedemptionReadOnlyAndNoCodeLeak(t *testing.T) {
	probe := &adminRedemptionProbe{}
	handler := &Handler{Dependencies: Dependencies{AfterSales: probe}}
	for _, authenticated := range []bool{false, true} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/admin/procurement-orders/3/redemption", nil)
		c.Params = gin.Params{{Key: "id", Value: "3"}}
		if authenticated {
			c.Set("admin_id", uint(1))
		}
		handler.InspectAdminRedemption(c)
		if !authenticated && probe.inspected != 0 {
			t.Fatal("unauthenticated inspection reached upstream")
		}
		if authenticated && (w.Code != 200 || probe.inspected != 3 || !strings.Contains(w.Body.String(), `"progress_status":"unused"`)) {
			t.Fatalf("missing status: %s", w.Body.String())
		}
		if strings.Contains(w.Body.String(), "private-test-code") {
			t.Fatal("redemption code leaked")
		}
	}
}

type riskHeldDownstreamOrders struct{ formTestOrders }

func (*riskHeldDownstreamOrders) GetOrderByUser(uint, uint) (*orderdomain.Order, error) {
	return &orderdomain.Order{ID: 12, OrderNo: "HELD", RiskHoldStatus: "pending_review"}, nil
}

func TestHeldOrderBlocksDownstreamCodeAccess(t *testing.T) {
	handler := &Handler{Dependencies: Dependencies{
		Orders:         &riskHeldDownstreamOrders{},
		DownstreamRefs: &scopedRefs{ref: &downstreamcallbackdomain.OrderRef{OrderID: 12, ApiCredentialID: 7}},
	}}
	for _, operation := range []func(*gin.Context){handler.RedemptionState, handler.ReplaceCode} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/orders/12/redemption", nil)
		c.Params = gin.Params{{Key: "id", Value: "12"}}
		c.Set(upstreamUserIDKey, uint(42))
		c.Set(upstreamCredentialIDKey, uint(7))
		operation(c)
		if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "state_unavailable") {
			t.Fatalf("held order reached the code operation: %d %s", w.Code, w.Body.String())
		}
	}
}

type deliveryLookupOrders struct {
	formTestOrders
	order *orderdomain.Order
}

func (s *deliveryLookupOrders) GetOrderByUser(uint, uint) (*orderdomain.Order, error) {
	return s.order, nil
}

func (*deliveryLookupOrders) BuildLocalRefundRecordsForOrder(*orderdomain.Order) ([]jsonmap.JSON, error) {
	return nil, nil
}

func TestDownstreamOrderLookupOnlyReturnsDeliveredUnheldChild(t *testing.T) {
	order := &orderdomain.Order{ID: 12, OrderNo: "SPLIT", Status: constants.OrderStatusPartiallyDelivered}
	order.Children = []orderdomain.Order{
		{RiskHoldStatus: "pending_review", Fulfillment: &fulfillmentdomain.Fulfillment{Status: "delivered", Payload: "held-code"}},
		{Fulfillment: &fulfillmentdomain.Fulfillment{Status: "pending", Payload: "pending-code"}},
		{Fulfillment: &fulfillmentdomain.Fulfillment{Status: "delivered", Payload: "released-code"}},
	}
	handler := &Handler{Dependencies: Dependencies{Orders: &deliveryLookupOrders{order: order}}}
	for _, hold := range []string{"", "pending_review", "rejected"} {
		order.RiskHoldStatus = hold
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/orders/12", nil)
		c.Params = gin.Params{{Key: "id", Value: "12"}}
		c.Set(upstreamUserIDKey, uint(42))
		handler.GetOrder(c)
		body := w.Body.String()
		if w.Code != http.StatusOK || strings.Contains(body, "held-code") || strings.Contains(body, "pending-code") ||
			strings.Contains(body, "released-code") != (hold == "") {
			t.Fatalf("unexpected fulfillment visibility for hold %q: HTTP %d %s", hold, w.Code, body)
		}
	}
}
