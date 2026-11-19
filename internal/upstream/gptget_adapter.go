package upstream

// gptget_adapter.go — gptget.fun 代理采购 API 适配器
//
// 协议与 dujiao-next 上游协议使用相同的签名算法（HMAC-SHA256），
// 但 API 路径前缀为 /api/v1/open/redemptions，字段命名遵循代理采购协议。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	procurementcontract "github.com/dujiao-next/internal/modules/procurement/contract"
	siteconnectiondomain "github.com/dujiao-next/internal/modules/siteconnection/domain"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/shopspring/decimal"
)

const (
	gptgetBasePath = "/api/v1/open/redemptions"
)

// GptgetAdapter gptget.fun 代理采购 API 适配器
type GptgetAdapter struct {
	baseURL   string
	apiKey    string
	apiSecret string
	client    *http.Client
}

// NewGptgetAdapter 创建 gptget.fun 适配器
func NewGptgetAdapter(conn *siteconnectiondomain.Connection) *GptgetAdapter {
	return &GptgetAdapter{
		baseURL:   strings.TrimRight(conn.BaseURL, "/"),
		apiKey:    conn.ApiKey,
		apiSecret: conn.ApiSecret,
		client:    &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
}

// — gptget API 请求/响应结构 —————————————————————————————————————

type gptgetPingResp struct {
	OK   bool `json:"ok"`
	Data struct {
		BusinessOperationsEnabled bool     `json:"business_operations_enabled"`
		AvailableOperations       []string `json:"available_operations"`
	} `json:"data"`
}
type gptgetAccountResp struct {
	OK   bool `json:"ok"`
	Data struct {
		UserID       uint   `json:"user_id"`
		Balance      string `json:"balance"`
		Currency     string `json:"currency"`
		FundingState string `json:"funding_state"`
	} `json:"data"`
}

type gptgetProduct struct {
	SKUID       uint         `json:"sku_id"`
	SKUCode     string       `json:"sku_code"`
	SpecValues  jsonmap.JSON `json:"spec_values"`
	SKUName     string       `json:"sku_name"`
	ProductName string       `json:"product_name"`
	SupplyPrice string       `json:"supply_price"`
	Currency    string       `json:"currency"`
	Available   bool         `json:"available"`
	ProductID   uint         `json:"product_id"`
	ProductSlug string       `json:"product_slug"`
	CategoryID  uint         `json:"category_id"`
	Images      []string     `json:"images"`
	Description string       `json:"description"`
}

type gptgetProductsResp struct {
	OK   bool   `json:"ok"`
	Msg  string `json:"msg"`
	Data struct {
		Total   int             `json:"total"`
		HasMore bool            `json:"has_more"`
		Items   []gptgetProduct `json:"items"`
	} `json:"data"`
}

type gptgetOrderReq struct {
	RequestID         string `json:"request_id"`
	ExternalOrderNo   string `json:"external_order_no"`
	SKUID             uint   `json:"sku_id"`
	ExpectedUnitPrice string `json:"expected_unit_price"`
	Currency          string `json:"currency"`
	CallbackURL       string `json:"callback_url,omitempty"`
}

type gptgetOrderData struct {
	OrderNo         string `json:"order_no"`
	ExternalOrderNo string `json:"external_order_no"`
	SKUID           uint   `json:"sku_id"`
	ProductName     string `json:"product_name"`
	Code            string `json:"code"`
	CodeRevision    int    `json:"code_revision"`
	FinancialStatus string `json:"financial_status"`
	ProgressStatus  string `json:"progress_status"`
	ChargedAmount   string `json:"charged_amount"`
	Currency        string `json:"currency"`
	BalanceAfter    string `json:"balance_after"`
	CanReplace      bool   `json:"can_replace"`
	CanRefund       bool   `json:"can_refund"`
}

type gptgetOrderResp struct {
	OK   bool            `json:"ok"`
	Msg  string          `json:"msg"`
	Data gptgetOrderData `json:"data"`
}

type gptgetGetOrderResp struct {
	OK   bool            `json:"ok"`
	Msg  string          `json:"msg"`
	Data gptgetOrderData `json:"data"`
}

// — Adapter 接口实现 ——————————————————————————————————————————————

// Ping 检测采购权限和余额
func (a *GptgetAdapter) Ping(ctx context.Context) (*PingResult, error) {
	var resp gptgetPingResp
	if err := a.doRequest(ctx, http.MethodGet, "/capabilities", nil, &resp); err != nil {
		return nil, err
	}
	var account gptgetAccountResp
	if err := a.doRequest(ctx, http.MethodGet, "/account", nil, &account); err != nil {
		return nil, err
	}
	return &PingResult{
		SiteName: "gptget.fun", ProtocolVersion: "gptget-v1", UserID: account.Data.UserID,
		Balance: account.Data.Balance, Currency: account.Data.Currency,
		FundingState:              account.Data.FundingState,
		BusinessOperationsEnabled: &resp.Data.BusinessOperationsEnabled,
		AvailableOperations:       resp.Data.AvailableOperations,
	}, nil
}

// ListCategories gptget 不提供分类 API，返回空列表
func (a *GptgetAdapter) ListCategories(_ context.Context) (*CategoryListResult, error) {
	return &CategoryListResult{Supported: false, Categories: []UpstreamCategory{}}, nil
}

// ListProducts 拉取 gptget 商品列表（供货价目录）
func (a *GptgetAdapter) ListProducts(ctx context.Context, opts ListProductsOpts) (*ProductListResult, error) {
	page, size := opts.Page, opts.PageSize
	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = 50
	}
	var rows []gptgetProduct
	for remotePage := 1; ; remotePage++ {
		if remotePage > 10000 {
			return nil, fmt.Errorf("gptget catalog pagination exceeds limit")
		}
		var resp gptgetProductsResp
		if err := a.doRequest(ctx, http.MethodGet, fmt.Sprintf("/products?page=%d", remotePage), nil, &resp); err != nil {
			return nil, err
		}
		rows = append(rows, resp.Data.Items...)
		if !resp.Data.HasMore {
			break
		}
		if len(resp.Data.Items) == 0 {
			return nil, fmt.Errorf("gptget catalog pagination returned empty page with has_more")
		}
	}

	items := make([]UpstreamProduct, 0, len(rows))
	positions := make(map[uint]int)
	for _, p := range rows {
		cur := p.Currency
		if cur == "" {
			cur = "CNY"
		}

		// 商品名包装为多语言 JSON（import.go 要求 title_json 非空）
		name := p.ProductName
		if name == "" {
			name = fmt.Sprintf("Product #%d", p.ProductID)
		}
		titleJSON := jsonmap.JSON{
			"zh-CN": name,
			"zh-TW": name,
			"en-US": name,
		}

		// available=true 视为在售；gptget 是按需采购，不维护本地库存
		active := p.Available
		stockQty := -1 // 无限库存
		if !active {
			stockQty = 0
		}
		stockStatus := "in_stock"
		if !active {
			stockStatus = "out_of_stock"
		}

		sku := UpstreamSKU{
			ID:            p.SKUID,
			SpecValues:    p.SpecValues,
			SKUCode:       p.SKUCode,
			PriceAmount:   p.SupplyPrice,
			StockStatus:   stockStatus,
			StockQuantity: stockQty,
			IsActive:      active,
		}
		if sku.SKUCode == "" {
			sku.SKUCode = fmt.Sprintf("gptget-sku-%d", p.SKUID)
		}

		product := UpstreamProduct{
			ID:          p.ProductID,
			Slug:        p.ProductSlug,
			Title:       titleJSON,
			PriceAmount: p.SupplyPrice,
			Currency:    cur,
			IsActive:    active,
			CategoryID:  p.CategoryID,
			Images:      p.Images,
			SKUs:        []UpstreamSKU{sku},
			Description: jsonmap.JSON{"zh-CN": p.Description, "zh-TW": p.Description, "en-US": p.Description},
		}
		if index, exists := positions[p.ProductID]; exists {
			existing := &items[index]
			existing.SKUs = append(existing.SKUs, sku)
			existing.IsActive = existing.IsActive || active
			price, err := decimal.NewFromString(p.SupplyPrice)
			previous, previousErr := decimal.NewFromString(existing.PriceAmount)
			if err != nil || previousErr != nil {
				return nil, fmt.Errorf("gptget invalid catalog price")
			}
			if price.LessThan(previous) {
				existing.PriceAmount = p.SupplyPrice
			}
		} else {
			positions[p.ProductID] = len(items)
			items = append(items, product)
		}
	}

	total := len(items)
	start := (page - 1) * size
	if start >= total {
		items = []UpstreamProduct{}
	} else {
		end := min(start+size, total)
		items = items[start:end]
	}
	return &ProductListResult{Total: total, Items: items}, nil
}

// GetProduct gptget 没有单商品详情接口，从列表里匹配
func (a *GptgetAdapter) GetProduct(ctx context.Context, productID uint) (*UpstreamProduct, error) {
	page := 1
	for {
		result, err := a.ListProducts(ctx, ListProductsOpts{Page: page, PageSize: 100})
		if err != nil {
			return nil, err
		}
		for _, p := range result.Items {
			if p.ID == productID {
				return &p, nil
			}
		}
		if len(result.Items) == 0 || page*100 >= result.Total {
			break
		}
		page++
	}
	return nil, ErrUpstreamProductDeleted
}

// CreateOrder 采购一个核销码
func (a *GptgetAdapter) CreateOrder(ctx context.Context, req CreateUpstreamOrderReq) (*CreateUpstreamOrderResp, error) {
	if req.Quantity != 1 || req.TraceID == "" || req.DownstreamOrderNo == "" || req.SKUID == 0 {
		return &CreateUpstreamOrderResp{OK: false, ErrorCode: "invalid_request", ErrorMessage: "gptget requires quantity=1, sku_id, request_id and external_order_no"}, nil
	}
	// Recover the original order before any retry can purchase again.
	var existing gptgetOrderResp
	err := a.doRequest(ctx, http.MethodGet, "/orders/by-external/"+url.PathEscape(req.DownstreamOrderNo), nil, &existing)
	if err == nil {
		return gptgetCreatedOrder(existing.Data), nil
	}
	var lookupErr *upstreamHTTPError
	if !errors.As(err, &lookupErr) || lookupErr.Status != http.StatusNotFound {
		if lookupErr != nil && lookupErr.Code != "" {
			return &CreateUpstreamOrderResp{ErrorCode: lookupErr.Code, ErrorMessage: lookupErr.Message}, nil
		}
		return nil, err
	}
	// 先从 SKU mapping 中拿到价格——由调用方通过 sku_mappings.upstream_price 提供
	// req.ManualFormData["expected_unit_price"] 由 submit.go 在 CreateUpstreamOrderReq 之前查好传入
	expectedPrice := ""
	currency := "CNY"
	if req.ManualFormData != nil {
		if v, ok := req.ManualFormData["expected_unit_price"]; ok {
			expectedPrice, _ = v.(string)
		}
		if v, ok := req.ManualFormData["currency"]; ok {
			currency, _ = v.(string)
		}
	}
	if expectedPrice == "" {
		return &CreateUpstreamOrderResp{
			OK:           false,
			ErrorCode:    "invalid_request",
			ErrorMessage: "expected_unit_price is required in ManualFormData for gptget adapter",
		}, nil
	}

	body := gptgetOrderReq{
		RequestID:         req.TraceID,
		ExternalOrderNo:   req.DownstreamOrderNo,
		SKUID:             req.SKUID,
		ExpectedUnitPrice: expectedPrice,
		Currency:          currency,
		CallbackURL:       req.CallbackURL,
	}

	var resp gptgetOrderResp
	if err := a.doRequest(ctx, http.MethodPost, "/orders", body, &resp); err != nil {
		var ue *upstreamHTTPError
		if errors.As(err, &ue) && ue.Code != "" {
			return &CreateUpstreamOrderResp{
				OK:           false,
				ErrorCode:    ue.Code,
				ErrorMessage: ue.Message,
			}, nil
		}
		return nil, err
	}

	return gptgetCreatedOrder(resp.Data), nil
}

// GetOrder gptget 只认字符串 order_no，按数字 ID 无法查询；调用方应走 GetOrderByNo
func (a *GptgetAdapter) GetOrder(_ context.Context, orderID uint) (*UpstreamOrderDetail, error) {
	return nil, fmt.Errorf("gptget: query by numeric order id %d is not supported, use order_no", orderID)
}

// GetOrderByNo 按我方返回的 order_no 查询采购订单
func (a *GptgetAdapter) GetOrderByNo(ctx context.Context, orderNo string) (*UpstreamOrderDetail, error) {
	if strings.TrimSpace(orderNo) == "" {
		return nil, fmt.Errorf("gptget: empty order_no")
	}
	var resp gptgetGetOrderResp
	if err := a.doRequest(ctx, http.MethodGet, "/orders/"+url.PathEscape(orderNo), nil, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("gptget get order failed: %s", resp.Msg)
	}

	d := resp.Data
	// 对本店而言，扣款成功并拿到核销码即视为已交付；客户之后是否核销（progress_status）不影响交付。
	status := "pending"
	switch {
	case d.FinancialStatus == "refunded":
		status = "refunded"
	case d.ProgressStatus == "destroyed":
		status = "canceled"
	case d.FinancialStatus == "paid" && d.Code != "":
		status = "delivered"
	}

	var fulfillment *UpstreamFulfillment
	if d.Code != "" && status == "delivered" {
		now := time.Now()
		fulfillment = &UpstreamFulfillment{
			Type:        "auto",
			Status:      d.ProgressStatus,
			Payload:     d.Code,
			DeliveredAt: &now,
		}
	}

	return &UpstreamOrderDetail{
		OrderNo:     d.OrderNo,
		Status:      status,
		Amount:      d.ChargedAmount,
		Currency:    d.Currency,
		Fulfillment: fulfillment,
	}, nil
}

// Numeric IDs are not valid GPTGET order identifiers.
func (a *GptgetAdapter) CancelOrder(_ context.Context, _ uint) error {
	return fmt.Errorf("%w: gptget requires string order_no", ErrUpstreamCancelNotAllowed)
}

// CancelOrderByNo refunds only the current unused code; never follows revision conflicts automatically.
func (a *GptgetAdapter) CancelOrderByNo(ctx context.Context, orderNo, requestID string) error {
	if orderNo == "" || requestID == "" {
		return fmt.Errorf("%w: missing order_no or request_id", ErrUpstreamCancelNotAllowed)
	}
	var current gptgetOrderResp
	path := "/orders/" + url.PathEscape(orderNo)
	if err := a.doRequest(ctx, http.MethodGet, path, nil, &current); err != nil {
		return err
	}
	if current.Data.FinancialStatus == "refunded" {
		return nil
	}
	if !current.Data.CanRefund || current.Data.CodeRevision < 1 {
		return ErrUpstreamCancelNotAllowed
	}
	var refunded gptgetOrderResp
	err := a.doRequest(ctx, http.MethodPost, path+"/refund", map[string]interface{}{
		"request_id": requestID, "expected_code_revision": current.Data.CodeRevision,
	}, &refunded)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUpstreamCancelNotAllowed, err)
	}
	if refunded.Data.FinancialStatus != "refunded" {
		return fmt.Errorf("gptget refund not confirmed")
	}
	return nil
}

func gptgetCreatedOrder(d gptgetOrderData) *CreateUpstreamOrderResp {
	if d.OrderNo == "" {
		return &CreateUpstreamOrderResp{ErrorCode: "invalid_response", ErrorMessage: "gptget response omitted order_no"}
	}
	return &CreateUpstreamOrderResp{OK: true, OrderNo: d.OrderNo, Status: "pending", Amount: d.ChargedAmount, Currency: d.Currency}
}

// DownloadImage gptget 图片允许跨域（来自 CDN），直接返回原 URL
func (a *GptgetAdapter) DownloadImage(_ context.Context, imageURL string) (string, error) {
	return imageURL, nil
}

// — HTTP 请求签名 ——————————————————————————————————————————————————

func (a *GptgetAdapter) doRequest(ctx context.Context, method, resource string, body interface{}, result interface{}) error {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
	}

	fullPath := gptgetBasePath + resource
	signPath := fullPath
	if idx := strings.Index(fullPath, "?"); idx > 0 {
		signPath = fullPath[:idx]
	}

	timestamp := time.Now().Unix()
	signature := Sign(a.apiSecret, method, signPath, timestamp, bodyBytes)

	reqURL := a.baseURL + fullPath
	var bodyReader io.Reader
	if bodyBytes != nil {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set(HeaderApiKey, a.apiKey)
	req.Header.Set(HeaderTimestamp, fmt.Sprintf("%d", timestamp))
	req.Header.Set(HeaderSignature, signature)
	if bodyBytes != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	var envelope struct {
		OK           bool   `json:"ok"`
		ErrorCode    string `json:"error_code"`
		ErrorMessage string `json:"error_message"`
		Msg          string `json:"msg"`
	}
	decodeErr := json.Unmarshal(respBody, &envelope)
	if resp.StatusCode != http.StatusOK || decodeErr != nil || !envelope.OK {
		message := envelope.ErrorMessage
		if message == "" {
			message = envelope.Msg
		}
		if message == "" {
			message = fmt.Sprintf("gptget request failed (HTTP %d)", resp.StatusCode)
		}
		code := envelope.ErrorCode
		if code == "" {
			code = fmt.Sprintf("http_%d", resp.StatusCode)
		}
		return &upstreamHTTPError{Status: resp.StatusCode, Code: code, Message: message}
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}
	return nil
}

// InspectRedemption returns the current revision without performing an operation.
func (a *GptgetAdapter) InspectRedemption(ctx context.Context, orderNo string) (*procurementcontract.RedemptionState, error) {
	var response gptgetOrderResp
	if err := a.doRequest(ctx, http.MethodGet, "/orders/"+url.PathEscape(orderNo), nil, &response); err != nil {
		return nil, err
	}
	return redemptionState(response.Data), nil
}
func (a *GptgetAdapter) ReplaceRedemption(ctx context.Context, orderNo, requestID string, revision int) (*procurementcontract.RedemptionState, error) {
	var response gptgetOrderResp
	if err := a.doRequest(ctx, http.MethodPost, "/orders/"+url.PathEscape(orderNo)+"/replace", map[string]interface{}{"request_id": requestID, "expected_code_revision": revision}, &response); err != nil {
		return nil, err
	}
	return redemptionState(response.Data), nil
}
func redemptionState(d gptgetOrderData) *procurementcontract.RedemptionState {
	return &procurementcontract.RedemptionState{Code: d.Code, Revision: d.CodeRevision, CanReplace: d.CanReplace, CanRefund: d.CanRefund, Progress: d.ProgressStatus, Financial: d.FinancialStatus}
}

func (a *GptgetAdapter) RefundRedemption(ctx context.Context, orderNo, requestID string, revision int) (*procurementcontract.RedemptionState, error) {
	var response gptgetOrderResp
	if err := a.doRequest(ctx, http.MethodPost, "/orders/"+url.PathEscape(orderNo)+"/refund", map[string]interface{}{"request_id": requestID, "expected_code_revision": revision, "reason": "雪糕AI管理员审核退款"}, &response); err != nil {
		return nil, err
	}
	return redemptionState(response.Data), nil
}
