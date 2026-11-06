package xorpay

import (
	"context"
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/modules/payment/infrastructure/gateway/common"

	"github.com/shopspring/decimal"
)

const (
	DefaultAPIBaseURL = "https://xorpay.com"
	DefaultCurrency   = "CNY"

	PayTypeNative = "native"
	PayTypeAlipay = "alipay"

	payTimeLayout = "2006-01-02 15:04:05"
)

var (
	ErrConfigInvalid    = errors.New("xorpay config invalid")
	ErrRequestFailed    = errors.New("xorpay request failed")
	ErrResponseInvalid  = errors.New("xorpay response invalid")
	ErrSignatureInvalid = errors.New("xorpay signature invalid")
	ErrChannelTypeNotOK = errors.New("xorpay channel type invalid")
)

// Config XorPay 配置。
type Config struct {
	common.ExchangeRateConfig
	APIBaseURL string `json:"api_base_url"` // 接口地址，默认 https://xorpay.com
	AID        string `json:"aid"`          // XorPay 后台的 aid
	AppSecret  string `json:"app_secret"`   // XorPay 后台的 app secret
	NotifyURL  string `json:"notify_url"`   // 异步回调地址
	Expire     int    `json:"expire"`       // 订单过期秒数，0 表示使用 XorPay 默认 7200
}

// Normalize 归一化配置。
func (c *Config) Normalize() {
	c.NormalizeExchangeRate()
	c.APIBaseURL = strings.TrimRight(strings.TrimSpace(c.APIBaseURL), "/")
	if c.APIBaseURL == "" {
		c.APIBaseURL = DefaultAPIBaseURL
	}
	c.AID = strings.TrimSpace(c.AID)
	c.AppSecret = strings.TrimSpace(c.AppSecret)
	c.NotifyURL = strings.TrimSpace(c.NotifyURL)
}

// CreateInput 下单输入。
type CreateInput struct {
	OrderNo     string
	Name        string
	Price       string
	ChannelType string
	NotifyURL   string
}

// CreateResult 下单结果。
type CreateResult struct {
	AOID     string
	QRCode   string
	ExpireIn int
	Raw      map[string]interface{}
}

// CallbackData 回调字段。
type CallbackData struct {
	AOID     string
	OrderID  string
	PayPrice string
	PayTime  string
	More     string
	Detail   string
	Sign     string
}

// ParseConfig 解析配置。
func ParseConfig(raw map[string]interface{}) (*Config, error) {
	return common.ParseConfig[Config](raw, ErrConfigInvalid)
}

// ValidateConfig 校验配置完整性。
func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("%w: config is nil", ErrConfigInvalid)
	}
	if cfg.AID == "" {
		return fmt.Errorf("%w: aid is required", ErrConfigInvalid)
	}
	if cfg.AppSecret == "" {
		return fmt.Errorf("%w: app_secret is required", ErrConfigInvalid)
	}
	if cfg.NotifyURL == "" {
		return fmt.Errorf("%w: notify_url is required", ErrConfigInvalid)
	}
	if _, err := url.ParseRequestURI(cfg.APIBaseURL); err != nil {
		return fmt.Errorf("%w: api_base_url is invalid", ErrConfigInvalid)
	}
	if cfg.Expire < 0 {
		return fmt.Errorf("%w: expire must not be negative", ErrConfigInvalid)
	}
	if cfg.ExchangeRate != "" {
		rate, err := decimal.NewFromString(cfg.ExchangeRate)
		if err != nil || !rate.IsPositive() {
			return fmt.Errorf("%w: exchange_rate is invalid", ErrConfigInvalid)
		}
	}
	return nil
}

// IsSupportedChannelType 判断 channel_type 是否受支持。
func IsSupportedChannelType(channelType string) bool {
	return ResolvePayType(channelType) != ""
}

// ResolvePayType 将 channel_type 映射为 XorPay pay_type。
func ResolvePayType(channelType string) string {
	switch strings.ToLower(strings.TrimSpace(channelType)) {
	case constants.PaymentChannelTypeWechat, constants.PaymentChannelTypeWxpay:
		return PayTypeNative
	case constants.PaymentChannelTypeAlipay:
		return PayTypeAlipay
	default:
		return ""
	}
}

// FormatPrice 格式化为 XorPay 要求的两位小数字符串，如 "50.00"。
func FormatPrice(amount string) (string, error) {
	d, err := decimal.NewFromString(strings.TrimSpace(amount))
	if err != nil || !d.IsPositive() {
		return "", fmt.Errorf("%w: invalid price %q", ErrConfigInvalid, amount)
	}
	return d.StringFixed(2), nil
}

// Sign 按 XorPay 规则对参数值顺序拼接后取 MD5 小写十六进制。
func Sign(values ...string) string {
	sum := md5.Sum([]byte(strings.Join(values, "")))
	return hex.EncodeToString(sum[:])
}

// CreatePayment 调用统一下单接口，返回支付二维码内容。
func CreatePayment(ctx context.Context, cfg *Config, input CreateInput) (*CreateResult, error) {
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}
	payType := ResolvePayType(input.ChannelType)
	if payType == "" {
		return nil, fmt.Errorf("%w: %s", ErrChannelTypeNotOK, input.ChannelType)
	}
	orderNo := strings.TrimSpace(input.OrderNo)
	if orderNo == "" {
		return nil, fmt.Errorf("%w: order_id is required", ErrConfigInvalid)
	}
	price, err := FormatPrice(input.Price)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = orderNo
	}
	notifyURL := common.PickFirstNonEmpty(input.NotifyURL, cfg.NotifyURL)

	form := url.Values{}
	form.Set("name", name)
	form.Set("pay_type", payType)
	form.Set("price", price)
	form.Set("order_id", orderNo)
	form.Set("notify_url", notifyURL)
	if cfg.Expire > 0 {
		form.Set("expire", strconv.Itoa(cfg.Expire))
	}
	form.Set("sign", Sign(name, payType, price, orderNo, notifyURL, cfg.AppSecret))

	endpoint := cfg.APIBaseURL + "/api/pay/" + url.PathEscape(cfg.AID)
	body, err := postForm(ctx, endpoint, form)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRequestFailed, err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("%w: decode response failed", ErrResponseInvalid)
	}
	status := common.ReadString(raw, "status")
	if status != "ok" {
		detail := common.ReadString(raw, "info")
		return nil, fmt.Errorf("%w: status=%s info=%s", ErrRequestFailed, status, detail)
	}
	qr := common.ReadString(common.ReadMap(raw, "info"), "qr")
	if qr == "" {
		return nil, fmt.Errorf("%w: missing info.qr", ErrResponseInvalid)
	}
	expireIn, _ := strconv.Atoi(common.PickFirstNonEmpty(common.ReadString(raw, "expires_in"), common.ReadString(raw, "expire_in")))
	return &CreateResult{
		AOID:     common.ReadString(raw, "aoid"),
		QRCode:   qr,
		ExpireIn: expireIn,
		Raw:      raw,
	}, nil
}

// ParseCallback 从回调 form 中读取字段。
func ParseCallback(form map[string][]string) (*CallbackData, error) {
	data := &CallbackData{
		AOID:     common.FormValue(form, "aoid"),
		OrderID:  common.FormValue(form, "order_id"),
		PayPrice: common.FormValue(form, "pay_price"),
		PayTime:  common.FormValue(form, "pay_time"),
		More:     common.FormValue(form, "more"),
		Detail:   common.FormValue(form, "detail"),
		Sign:     common.FormValue(form, "sign"),
	}
	if data.AOID == "" || data.OrderID == "" || data.PayPrice == "" || data.PayTime == "" || data.Sign == "" {
		return nil, fmt.Errorf("%w: missing callback fields", ErrResponseInvalid)
	}
	return data, nil
}

// VerifyCallback 校验回调签名：aoid + order_id + pay_price + pay_time + app_secret。
func VerifyCallback(cfg *Config, data *CallbackData) error {
	if cfg == nil || cfg.AppSecret == "" {
		return fmt.Errorf("%w: app_secret is required", ErrConfigInvalid)
	}
	if data == nil {
		return fmt.Errorf("%w: callback is empty", ErrSignatureInvalid)
	}
	expected := Sign(data.AOID, data.OrderID, data.PayPrice, data.PayTime, cfg.AppSecret)
	if subtle.ConstantTimeCompare([]byte(expected), []byte(strings.ToLower(data.Sign))) != 1 {
		return ErrSignatureInvalid
	}
	return nil
}

// ParsePayTime 解析回调 pay_time（北京时间）。
func ParsePayTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	loc := time.FixedZone("CST", 8*3600)
	parsed, err := time.ParseInLocation(payTimeLayout, value, loc)
	if err != nil {
		return nil
	}
	return &parsed
}

func postForm(ctx context.Context, endpoint string, form url.Values) ([]byte, error) {
	ctx, cancel := common.WithDefaultTimeout(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http status %d", resp.StatusCode)
	}
	return body, nil
}
