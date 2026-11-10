package x402adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	paymentcontract "github.com/dujiao-next/internal/modules/payment/contract"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/modules/payment/infrastructure/gateway/x402"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/money"

	"github.com/shopspring/decimal"
)

// PayloadKeyRequirements 是创建支付时写入 provider_payload 的 x402 收款要求快照；
// 前台钱包按它签名，提交签名时服务端也只认这份快照。
const PayloadKeyRequirements = "x402_requirements"

// x402Adapter 是 x402 收款渠道的 paymentcontract.GatewayProvider + paymentcontract.GatewayPayloadSettler 实现。
// x402 没有网关侧订单与异步回调：创建支付只生成收款要求，付款人钱包签名后由前台提交，
// 服务端经 facilitator 验签并上链结算成功后才确认支付。
type x402Adapter struct{}

// NewX402Adapter 实例化 x402 adapter。
func NewX402Adapter() paymentcontract.GatewayProvider { return &x402Adapter{} }

var (
	_ paymentcontract.GatewayProvider       = (*x402Adapter)(nil)
	_ paymentcontract.GatewayPayloadSettler = (*x402Adapter)(nil)
)

// Type 返回 provider 标识。
func (a *x402Adapter) Type() string {
	return constants.PaymentProviderX402 + ":"
}

func (a *x402Adapter) parseConfig(raw jsonmap.JSON) (*x402.Config, error) {
	cfg, err := x402.ParseConfig(raw)
	if err != nil {
		return nil, mapX402Error(err)
	}
	if err := x402.ValidateConfig(cfg); err != nil {
		return nil, mapX402Error(err)
	}
	return cfg, nil
}

// ValidateConfig 验证 channel.ConfigJSON 与 channel_type。
func (a *x402Adapter) ValidateConfig(raw jsonmap.JSON, channelType string) error {
	if channelType != "" && channelType != constants.PaymentChannelTypeX402USDC {
		return fmt.Errorf("%w: x402 channel_type %s", paymentcontract.ErrGatewayUnsupportedChannel, channelType)
	}
	_, err := a.parseConfig(raw)
	return err
}

// CreatePayment 按订单金额与渠道汇率生成 USDC 收款要求，不调用任何外部服务。
func (a *x402Adapter) CreatePayment(_ context.Context, raw jsonmap.JSON, input paymentcontract.GatewayCreateInput) (*paymentcontract.GatewayCreateResult, error) {
	cfg, err := a.parseConfig(raw)
	if err != nil {
		return nil, err
	}
	requirements, usdcAmount, err := x402.BuildRequirements(cfg, input.Amount.Decimal)
	if err != nil {
		return nil, mapX402Error(err)
	}
	snapshot, err := toJSONMap(requirements)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", paymentcontract.ErrGatewayConfigInvalid, err)
	}
	payload := jsonmap.JSON{
		PayloadKeyRequirements: snapshot,
		"exchange_rate":        cfg.ExchangeRate,
		"original_amount":      input.Amount.Decimal.String(),
		"original_currency":    strings.ToUpper(strings.TrimSpace(input.Currency)),
		"usdc_amount":          usdcAmount,
	}
	// x402 没有二维码与跳转链接；QRCodeURL 写入 CAIP-10 风格的收款标识，
	// 使支付记录满足"有网关结果"的判定，刷新页面或重新选择同一渠道时能复用这笔待支付记录。
	return &paymentcontract.GatewayCreateResult{
		ProviderRef:  input.OrderNo,
		QRCodeURL:    requirements.Network + ":" + requirements.PayTo,
		Payload:      payload,
		AmountSent:   usdcAmount,
		CurrencySent: x402.AssetSymbol,
	}, nil
}

// SettlePayload 实现 paymentcontract.GatewayPayloadSettler：核对签名载荷 → facilitator verify → settle。
// 只有链上结算成功才返回 success；任何失败都不会改变支付状态。
func (a *x402Adapter) SettlePayload(ctx context.Context, raw jsonmap.JSON, providerPayload jsonmap.JSON, signed jsonmap.JSON) (*paymentcontract.GatewayCallbackResult, error) {
	cfg, err := a.parseConfig(raw)
	if err != nil {
		return nil, err
	}
	expected, err := requirementsFromSnapshot(providerPayload)
	if err != nil {
		return nil, err
	}
	var payload x402.PaymentPayload
	if err := fromJSONMap(signed, &payload); err != nil {
		return nil, fmt.Errorf("%w: %v", paymentcontract.ErrGatewayResponseInvalid, err)
	}
	if err := x402.ValidatePayload(expected, &payload, time.Now()); err != nil {
		return nil, mapX402Error(err)
	}
	result, err := x402.VerifyAndSettle(ctx, cfg, expected, &payload)
	if err != nil {
		return nil, mapX402Error(err)
	}

	amount := money.Amount{}
	if d, parseErr := decimal.NewFromString(expected.Amount); parseErr == nil {
		amount = money.FromDecimal(d.Shift(-6))
	}
	now := time.Now()
	return &paymentcontract.GatewayCallbackResult{
		ProviderRef: result.Transaction,
		Status:      constants.PaymentStatusSuccess,
		Amount:      amount,
		Currency:    x402.AssetSymbol,
		PaidAt:      &now,
		Payload: jsonmap.JSON{
			"x402_settlement": jsonmap.JSON{
				"transaction": result.Transaction,
				"network":     expected.Network,
				"payer":       payload.Payload.Authorization.From,
			},
		},
	}, nil
}

func requirementsFromSnapshot(providerPayload jsonmap.JSON) (*x402.Requirements, error) {
	raw, ok := providerPayload[PayloadKeyRequirements]
	if !ok {
		return nil, fmt.Errorf("%w: missing x402 requirements snapshot", paymentcontract.ErrGatewayConfigInvalid)
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", paymentcontract.ErrGatewayConfigInvalid, err)
	}
	var req x402.Requirements
	if err := json.Unmarshal(data, &req); err != nil || req.Amount == "" || req.PayTo == "" {
		return nil, fmt.Errorf("%w: invalid x402 requirements snapshot", paymentcontract.ErrGatewayConfigInvalid)
	}
	return &req, nil
}

func toJSONMap(v interface{}) (jsonmap.JSON, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	out := jsonmap.JSON{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func fromJSONMap(m jsonmap.JSON, out interface{}) error {
	if len(m) == 0 {
		return errors.New("empty x402 payment payload")
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

// mapX402Error 把 x402 包的 sentinel error 映射为 provider 统一错误。
func mapX402Error(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, x402.ErrConfigInvalid):
		return fmt.Errorf("%w: %v", paymentcontract.ErrGatewayConfigInvalid, err)
	case errors.Is(err, x402.ErrRequestFailed):
		return fmt.Errorf("%w: %v", paymentcontract.ErrGatewayRequestFailed, err)
	case errors.Is(err, x402.ErrPaymentInvalid), errors.Is(err, x402.ErrPaymentRejected):
		return fmt.Errorf("%w: %v", paymentcontract.ErrGatewaySignatureInvalid, err)
	case errors.Is(err, x402.ErrOnchainMismatch):
		// 链上回执与付款要求不符——当作拒付处理，不要重试。
		return fmt.Errorf("%w: %v", paymentcontract.ErrGatewaySignatureInvalid, err)
	case errors.Is(err, x402.ErrOnchainUnconfirmed):
		// 链上交易尚未出块，让调用方知道是暂时性状态。
		return fmt.Errorf("%w: %v", paymentcontract.ErrGatewayRequestFailed, err)
	case errors.Is(err, x402.ErrResponseInvalid):
		return fmt.Errorf("%w: %v", paymentcontract.ErrGatewayResponseInvalid, err)
	default:
		return err
	}
}
