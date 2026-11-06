// Package x402 实现 x402 v2 协议 exact 方案（EVM / EIP-3009）的收款侧逻辑：
// 生成 PaymentRequirements、调用 facilitator 的 /verify 与 /settle。
//
// 规范：https://github.com/x402-foundation/x402/blob/main/specs/x402-specification-v2.md
// 付款人在浏览器钱包里对 TransferWithAuthorization 做 EIP-712 签名，本包只转交签名，
// 不持有任何私钥；链上广播与 gas 由 facilitator 承担。
package x402

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dujiao-next/internal/modules/payment/infrastructure/gateway/common"

	"github.com/shopspring/decimal"
)

const (
	Version = 2

	SchemeExact = "exact"

	// NetworkBaseSepolia Base Sepolia 测试网（CAIP-2）。
	NetworkBaseSepolia = "eip155:84532"
	// NetworkBase Base 主网（CAIP-2）。
	NetworkBase = "eip155:8453"

	// DefaultFacilitatorURL 公共测试网 facilitator，无需鉴权，仅支持测试网。
	DefaultFacilitatorURL = "https://x402.org/facilitator"

	// AssetSymbol 链上结算币种，用于 payment.currency。
	AssetSymbol = "USDC"

	usdcDecimals = 6

	defaultMaxTimeoutSeconds = 600
	maxMaxTimeoutSeconds     = 3600
)

var (
	ErrConfigInvalid   = errors.New("x402 config invalid")
	ErrRequestFailed   = errors.New("x402 facilitator request failed")
	ErrResponseInvalid = errors.New("x402 facilitator response invalid")
	ErrPaymentInvalid  = errors.New("x402 payment payload invalid")
	ErrPaymentRejected = errors.New("x402 payment rejected")
)

var evmAddressPattern = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)

// knownAssets 各网络的原生 USDC 合约及其 EIP-712 domain（name / version）。
// 测试网 name/version 已在链上读取 name()、version() 核实。
var knownAssets = map[string]assetInfo{
	NetworkBaseSepolia: {Address: "0x036CbD53842c5426634e7929541eC2318f3dCF7e", Name: "USDC", Version: "2"},
	NetworkBase:        {Address: "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913", Name: "USD Coin", Version: "2"},
}

type assetInfo struct {
	Address string
	Name    string
	Version string
}

// Config x402 渠道配置。商品以订单币种（一般 CNY）定价，exchange_rate 为
// 1 单位订单币种可兑换的 USDC 数量（例如 1 CNY ≈ 0.14 USDC 时填 0.14）。
type Config struct {
	PayTo             string `json:"pay_to"`              // 收款地址（只需地址，不需要私钥）
	Network           string `json:"network"`             // CAIP-2 网络，默认 Base Sepolia
	FacilitatorURL    string `json:"facilitator_url"`     // facilitator 根地址
	ExchangeRate      string `json:"exchange_rate"`       // 订单币种 → USDC 汇率
	MaxTimeoutSeconds int    `json:"max_timeout_seconds"` // 签名有效期
}

// Normalize 归一化配置。
func (c *Config) Normalize() {
	c.PayTo = strings.TrimSpace(c.PayTo)
	c.Network = strings.TrimSpace(c.Network)
	if c.Network == "" {
		c.Network = NetworkBaseSepolia
	}
	c.FacilitatorURL = strings.TrimRight(strings.TrimSpace(c.FacilitatorURL), "/")
	if c.FacilitatorURL == "" {
		c.FacilitatorURL = DefaultFacilitatorURL
	}
	c.ExchangeRate = strings.TrimSpace(c.ExchangeRate)
	if c.MaxTimeoutSeconds <= 0 {
		c.MaxTimeoutSeconds = defaultMaxTimeoutSeconds
	}
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
	if !evmAddressPattern.MatchString(cfg.PayTo) {
		return fmt.Errorf("%w: pay_to must be a 0x EVM address", ErrConfigInvalid)
	}
	if _, ok := knownAssets[cfg.Network]; !ok {
		return fmt.Errorf("%w: unsupported network %s", ErrConfigInvalid, cfg.Network)
	}
	if u, err := url.ParseRequestURI(cfg.FacilitatorURL); err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("%w: facilitator_url is invalid", ErrConfigInvalid)
	}
	rate, err := decimal.NewFromString(cfg.ExchangeRate)
	if err != nil || !rate.IsPositive() {
		return fmt.Errorf("%w: exchange_rate is required and must be positive", ErrConfigInvalid)
	}
	if cfg.MaxTimeoutSeconds > maxMaxTimeoutSeconds {
		return fmt.Errorf("%w: max_timeout_seconds must not exceed %d", ErrConfigInvalid, maxMaxTimeoutSeconds)
	}
	return nil
}

// Requirements 即 x402 v2 的 PaymentRequirements。
type Requirements struct {
	Scheme            string            `json:"scheme"`
	Network           string            `json:"network"`
	Amount            string            `json:"amount"`
	Asset             string            `json:"asset"`
	PayTo             string            `json:"payTo"`
	MaxTimeoutSeconds int               `json:"maxTimeoutSeconds"`
	Extra             map[string]string `json:"extra"`
}

// BuildRequirements 按订单金额（订单币种）生成收款要求。
// 返回 requirements 与换算后的 USDC 金额（两位小数的展示值）。
func BuildRequirements(cfg *Config, orderAmount decimal.Decimal) (*Requirements, string, error) {
	if err := ValidateConfig(cfg); err != nil {
		return nil, "", err
	}
	if !orderAmount.IsPositive() {
		return nil, "", fmt.Errorf("%w: amount must be positive", ErrConfigInvalid)
	}
	rate, _ := decimal.NewFromString(cfg.ExchangeRate)
	// 向上取整到 0.01 USDC，避免四舍五入后少收。
	usdc := orderAmount.Mul(rate).RoundUp(2)
	if !usdc.IsPositive() {
		return nil, "", fmt.Errorf("%w: converted amount is zero", ErrConfigInvalid)
	}
	atomic := usdc.Shift(usdcDecimals).String()
	asset := knownAssets[cfg.Network]
	return &Requirements{
		Scheme:            SchemeExact,
		Network:           cfg.Network,
		Amount:            atomic,
		Asset:             asset.Address,
		PayTo:             cfg.PayTo,
		MaxTimeoutSeconds: cfg.MaxTimeoutSeconds,
		Extra:             map[string]string{"name": asset.Name, "version": asset.Version},
	}, usdc.StringFixed(2), nil
}

// ChainID 从 CAIP-2 网络标识中取出 EVM chainId。
func ChainID(network string) (int64, error) {
	ref, ok := strings.CutPrefix(network, "eip155:")
	if !ok {
		return 0, fmt.Errorf("%w: not an eip155 network: %s", ErrConfigInvalid, network)
	}
	return strconv.ParseInt(ref, 10, 64)
}

// Authorization EIP-3009 TransferWithAuthorization 参数，数值均为十进制字符串。
type Authorization struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Value       string `json:"value"`
	ValidAfter  string `json:"validAfter"`
	ValidBefore string `json:"validBefore"`
	Nonce       string `json:"nonce"`
}

// ExactPayload exact 方案的 payload。
type ExactPayload struct {
	Signature     string        `json:"signature"`
	Authorization Authorization `json:"authorization"`
}

// PaymentPayload 即 x402 v2 的 PaymentPayload。
type PaymentPayload struct {
	X402Version int          `json:"x402Version"`
	Accepted    Requirements `json:"accepted"`
	Payload     ExactPayload `json:"payload"`
}

// ValidatePayload 在交给 facilitator 之前，按服务端保存的 requirements 逐项核对客户端提交的内容。
// 金额、收款地址、网络、合约都以服务端快照为准，防止客户端篡改；facilitator 只负责验签和余额。
func ValidatePayload(expected *Requirements, payload *PaymentPayload, now time.Time) error {
	if expected == nil || payload == nil {
		return fmt.Errorf("%w: missing payload", ErrPaymentInvalid)
	}
	if payload.X402Version != Version {
		return fmt.Errorf("%w: unsupported x402Version %d", ErrPaymentInvalid, payload.X402Version)
	}
	auth := payload.Payload.Authorization
	if !evmAddressPattern.MatchString(auth.From) {
		return fmt.Errorf("%w: invalid payer address", ErrPaymentInvalid)
	}
	if !strings.EqualFold(auth.To, expected.PayTo) {
		return fmt.Errorf("%w: recipient mismatch", ErrPaymentInvalid)
	}
	if auth.Value != expected.Amount {
		return fmt.Errorf("%w: amount mismatch", ErrPaymentInvalid)
	}
	validBefore, err := strconv.ParseInt(auth.ValidBefore, 10, 64)
	if err != nil || validBefore <= now.Unix() {
		return fmt.Errorf("%w: authorization expired", ErrPaymentInvalid)
	}
	if validBefore > now.Unix()+int64(expected.MaxTimeoutSeconds)+60 {
		return fmt.Errorf("%w: validBefore too far in the future", ErrPaymentInvalid)
	}
	if _, err := strconv.ParseInt(auth.ValidAfter, 10, 64); err != nil {
		return fmt.Errorf("%w: invalid validAfter", ErrPaymentInvalid)
	}
	if len(auth.Nonce) != 66 || !strings.HasPrefix(auth.Nonce, "0x") {
		return fmt.Errorf("%w: invalid nonce", ErrPaymentInvalid)
	}
	if !strings.HasPrefix(payload.Payload.Signature, "0x") || len(payload.Payload.Signature) < 132 {
		return fmt.Errorf("%w: invalid signature", ErrPaymentInvalid)
	}
	// accepted 必须与服务端快照完全一致，再用服务端快照覆盖，杜绝客户端自带 requirements。
	a := payload.Accepted
	if a.Scheme != expected.Scheme || a.Network != expected.Network || a.Amount != expected.Amount ||
		!strings.EqualFold(a.Asset, expected.Asset) || !strings.EqualFold(a.PayTo, expected.PayTo) {
		return fmt.Errorf("%w: accepted requirements mismatch", ErrPaymentInvalid)
	}
	payload.Accepted = *expected
	return nil
}

// SettleResult facilitator /settle 的结果。
type SettleResult struct {
	Success      bool   `json:"success"`
	Transaction  string `json:"transaction"`
	Network      string `json:"network"`
	Payer        string `json:"payer"`
	ErrorReason  string `json:"errorReason"`
	ErrorMessage string `json:"errorMessage"`
	Raw          map[string]interface{}
}

type verifyResult struct {
	IsValid        bool   `json:"isValid"`
	InvalidReason  string `json:"invalidReason"`
	InvalidMessage string `json:"invalidMessage"`
}

// VerifyAndSettle 先 /verify 再 /settle，只有链上结算成功才返回成功。
// 采用"先结算再发货"：调用方拿到 Success 后才标记已支付。
func VerifyAndSettle(ctx context.Context, cfg *Config, expected *Requirements, payload *PaymentPayload) (*SettleResult, error) {
	body := map[string]interface{}{
		"x402Version":         Version,
		"paymentPayload":      payload,
		"paymentRequirements": expected,
	}

	var v verifyResult
	if _, err := postJSON(ctx, cfg.FacilitatorURL+"/verify", body, &v); err != nil {
		return nil, err
	}
	if !v.IsValid {
		return nil, fmt.Errorf("%w: verify failed: %s %s", ErrPaymentRejected, v.InvalidReason, v.InvalidMessage)
	}

	var s SettleResult
	raw, err := postJSON(ctx, cfg.FacilitatorURL+"/settle", body, &s)
	if err != nil {
		return nil, err
	}
	s.Raw = raw
	if !s.Success || strings.TrimSpace(s.Transaction) == "" {
		return nil, fmt.Errorf("%w: settle failed: %s %s", ErrPaymentRejected, s.ErrorReason, s.ErrorMessage)
	}
	if s.Network != "" && s.Network != expected.Network {
		return nil, fmt.Errorf("%w: settled on unexpected network %s", ErrResponseInvalid, s.Network)
	}
	return &s, nil
}

func postJSON(ctx context.Context, endpoint string, body interface{}, out interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal request: %v", ErrRequestFailed, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRequestFailed, err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRequestFailed, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: read response: %v", ErrRequestFailed, err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("%w: status %d: %s", ErrResponseInvalid, resp.StatusCode, truncate(string(respBody), 200))
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return nil, fmt.Errorf("%w: decode: %v", ErrResponseInvalid, err)
	}
	return raw, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
