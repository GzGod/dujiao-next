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
	"strings"
	"time"

	siteconnectiondomain "github.com/dujiao-next/internal/modules/siteconnection/domain"
	"github.com/dujiao-next/internal/logger"
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
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

// — gptget API 请求/响应结构 —————————————————————————————————————

type gptgetPingResp struct {
	OK   bool   `json:"ok"`
	Msg  string `json:"msg"`
	Data struct {
		CanPurchase bool   `json:"can_purchase"`
		Balance     string `json:"balance"`
		Currency    string `json:"currency"`
	} `json:"data"`
}

type gptgetProduct struct {
	SKUID         uint    `json:"sku_id"`
	ProductName   string  `json:"product_name"`
	// 价格字段：兼容多种可能的字段名
	UnitPrice     string  `json:"unit_price"`
	Price         string  `json:"price"`
	SupplyPrice   string  `json:"supply_price"`
	AgentPrice    string  `json:"agent_price"`
	Currency      string  `json:"currency"`
	// 库存字段：兼容多种可能的字段名
	Stock         int     `json:"stock"`
	StockQuantity int     `json:"stock_quantity"`
	StockCount    int     `json:"stock_count"`
	IsActive      bool    `json:"is_active"`
	Available     bool    `json:"available"`
	ProductID     uint    `json:"product_id"`
	ProductSlug   string  `json:"product_slug"`
	CategoryID    uint    `json:"category_id"`
	ImageURL      string  `json:"image_url"`
	Name          string  `json:"name"`
	Title         string  `json:"title"`
}

type gptgetProductsResp struct {
	OK   bool   `json:"ok"`
	Msg  string `json:"msg"`
	Data struct {
		Total int             `json:"total"`
		Items []gptgetProduct `json:"items"`
	} `json:"data"`
}

type gptgetOrderReq struct {
	RequestID        string `json:"request_id"`
	ExternalOrderNo  string `json:"external_order_no"`
	SKUID            uint   `json:"sku_id"`
	ExpectedUnitPrice string `json:"expected_unit_price"`
	Currency         string `json:"currency"`
	CallbackURL      string `json:"callback_url,omitempty"`
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
	if !resp.OK {
		return nil, fmt.Errorf("gptget ping failed: %s", resp.Msg)
	}
	return &PingResult{
		SiteName:        "gptget.fun",
		ProtocolVersion: "gptget-v1",
		Balance:         resp.Data.Balance,
		Currency:        resp.Data.Currency,
	}, nil
}

// ListCategories gptget 不提供分类 API，返回空列表
func (a *GptgetAdapter) ListCategories(_ context.Context) (*CategoryListResult, error) {
	return &CategoryListResult{Supported: false, Categories: []UpstreamCategory{}}, nil
}

// ListProducts 拉取 gptget 商品列表（供货价目录）
func (a *GptgetAdapter) ListProducts(ctx context.Context, opts ListProductsOpts) (*ProductListResult, error) {
	page := opts.Page
	if page < 1 {
		page = 1
	}
	path := fmt.Sprintf("/products?page=%d", page)

	var resp gptgetProductsResp
	if err := a.doRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	// 调试：记录原始第一条数据的结构，用于确认字段名（只记录首页首条）
	if page == 1 && len(resp.Data.Items) > 0 {
		logger.Infow("gptget_products_sample",
			"first_item_sku_id", resp.Data.Items[0].SKUID,
			"first_item_price_unit", resp.Data.Items[0].UnitPrice,
			"first_item_price_supply", resp.Data.Items[0].SupplyPrice,
			"first_item_price_agent", resp.Data.Items[0].AgentPrice,
			"first_item_price", resp.Data.Items[0].Price,
			"first_item_stock", resp.Data.Items[0].Stock,
			"first_item_stock_qty", resp.Data.Items[0].StockQuantity,
			"first_item_is_active", resp.Data.Items[0].IsActive,
			"first_item_available", resp.Data.Items[0].Available,
			"total", resp.Data.Total,
		)
	}
	if !resp.OK {
		return nil, fmt.Errorf("gptget list products failed: %s", resp.Msg)
	}

	items := make([]UpstreamProduct, 0, len(resp.Data.Items))
	for _, p := range resp.Data.Items {
		// 兼容多种价格字段名
		price := p.UnitPrice
		if price == "" {
			price = p.SupplyPrice
		}
		if price == "" {
			price = p.AgentPrice
		}
		if price == "" {
			price = p.Price
		}
		cur := p.Currency
		if cur == "" {
			cur = "CNY"
		}

		// 兼容多种库存字段名
		stockQty := p.Stock
		if stockQty == 0 && p.StockQuantity != 0 {
			stockQty = p.StockQuantity
		}
		if stockQty == 0 && p.StockCount != 0 {
			stockQty = p.StockCount
		}

		// 商品名兼容
		name := p.ProductName
		if name == "" {
			name = p.Name
		}
		if name == "" {
			name = p.Title
		}
		if name == "" {
			name = fmt.Sprintf("Product #%d", p.ProductID)
		}

		// is_active：如果未设置但 available=true 也视为有效
		active := p.IsActive || p.Available

		// 库存：gptget 是按需采购模式，stock=0 不代表缺货
		// 只有明确缺货（stock=-2 或类似负值表示缺货）才标记缺货；
		// stock=0 且 active=true 视为无限库存（-1）
		effectiveStock := stockQty
		stockStatus := "in_stock"
		if !active {
			stockStatus = "out_of_stock"
		} else if stockQty < 0 && stockQty != -1 {
			// 特殊负值表示缺货
			stockStatus = "out_of_stock"
			effectiveStock = 0
		} else if stockQty == 0 {
			// 0 视为无限库存
			effectiveStock = -1
		}

		sku := UpstreamSKU{
			ID:            p.SKUID,
			SKUCode:       fmt.Sprintf("gptget-sku-%d", p.SKUID),
			PriceAmount:   price,
			StockStatus:   stockStatus,
			StockQuantity: effectiveStock,
			IsActive:      active,
		}
		product := UpstreamProduct{
			ID:          p.ProductID,
			Slug:        p.ProductSlug,
			PriceAmount: price,
			Currency:    cur,
			IsActive:    active,
			CategoryID:  p.CategoryID,
			SKUs:        []UpstreamSKU{sku},
		}
		if p.ImageURL != "" {
			product.Images = []string{p.ImageURL}
		}
		items = append(items, product)
	}

	return &ProductListResult{
		Total: resp.Data.Total,
		Items: items,
	}, nil
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
	if !resp.OK {
		return &CreateUpstreamOrderResp{
			OK:           false,
			ErrorCode:    "upstream_rejected",
			ErrorMessage: resp.Msg,
		}, nil
	}

	// gptget 直接在采购响应里返回核销码
	status := "pending"
	if resp.Data.FinancialStatus == "paid" {
		status = "completed"
	}

	return &CreateUpstreamOrderResp{
		OK:       true,
		OrderNo:  resp.Data.OrderNo,
		Status:   status,
		Amount:   resp.Data.ChargedAmount,
		Currency: resp.Data.Currency,
	}, nil
}

// GetOrder 查询采购订单状态
func (a *GptgetAdapter) GetOrder(ctx context.Context, orderID uint) (*UpstreamOrderDetail, error) {
	// gptget 用字符串 order_no，但 Adapter 接口传 uint ID
	// 通过 orders/:order_no 路径查询，order_no 存在 procurement_orders.upstream_order_no 里
	// 这里用 ID 作为 order_no 的占位，实际在 GptgetAdapterWithOrderNo 方法里处理
	path := fmt.Sprintf("/orders/%d", orderID)
	return a.getOrderByPath(ctx, path)
}

// GetOrderByNo 按字符串 order_no 查询（gptget 专用）
func (a *GptgetAdapter) GetOrderByNo(ctx context.Context, orderNo string) (*UpstreamOrderDetail, error) {
	path := fmt.Sprintf("/orders/%s", orderNo)
	return a.getOrderByPath(ctx, path)
}

func (a *GptgetAdapter) getOrderByPath(ctx context.Context, path string) (*UpstreamOrderDetail, error) {
	var resp gptgetGetOrderResp
	if err := a.doRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("gptget get order failed: %s", resp.Msg)
	}

	d := resp.Data
	status := "pending"
	switch d.ProgressStatus {
	case "unused":
		status = "paid"
	case "completed":
		status = "completed"
	case "processing":
		status = "processing"
	}

	var fulfillment *UpstreamFulfillment
	if d.Code != "" {
		now := time.Now()
		fulfillment = &UpstreamFulfillment{
			Type:        "code",
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

// CancelOrder 退款（gptget 用 refund 接口）
func (a *GptgetAdapter) CancelOrder(ctx context.Context, orderID uint) error {
	path := fmt.Sprintf("/orders/%d/refund", orderID)
	body := map[string]interface{}{
		"expected_code_revision": 1,
	}
	var resp struct {
		OK  bool   `json:"ok"`
		Msg string `json:"msg"`
	}
	if err := a.doRequest(ctx, http.MethodPost, path, body, &resp); err != nil {
		var ue *upstreamHTTPError
		if errors.As(err, &ue) && (ue.Code == "cancel_not_allowed" || ue.Code == "state_conflict") {
			return fmt.Errorf("%w: %s", ErrUpstreamCancelNotAllowed, ue.Message)
		}
		return err
	}
	if !resp.OK {
		return fmt.Errorf("%w: %s", ErrUpstreamCancelNotAllowed, resp.Msg)
	}
	return nil
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

	if resp.StatusCode != http.StatusOK {
		logger.Warnw("gptget_request_error",
			"method", method, "resource", resource,
			"status", resp.StatusCode, "body", string(respBody))
		var errPayload struct {
			ErrorCode    string `json:"error_code"`
			ErrorMessage string `json:"error_message"`
			Msg          string `json:"msg"`
		}
		_ = json.Unmarshal(respBody, &errPayload)
		code := errPayload.ErrorCode
		msg := errPayload.ErrorMessage
		if msg == "" {
			msg = errPayload.Msg
		}
		return &upstreamHTTPError{
			Status:  resp.StatusCode,
			Code:    code,
			Message: msg,
			Body:    string(respBody),
		}
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}
	return nil
}
