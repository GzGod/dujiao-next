package xorpayadapter

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gatewaycommon "github.com/dujiao-next/internal/modules/payment/infrastructure/gateway/common"

	paymentcontract "github.com/dujiao-next/internal/modules/payment/contract"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/modules/payment/infrastructure/gateway/xorpay"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/money"

	"github.com/shopspring/decimal"
)

// xorpayAdapter 是 XorPay 网关的 paymentcontract.GatewayProvider + paymentcontract.GatewayCallbackVerifier 实现。
// channel_type 为 wechat（NATIVE 扫码）或 alipay（当面付），下单返回二维码内容；
// 回调是同步 form POST，所以**不**实现 paymentcontract.GatewayCapturer 和 paymentcontract.GatewayWebhooker。
type xorpayAdapter struct{}

// NewXorpayAdapter 实例化 XorPay adapter。
func NewXorpayAdapter() paymentcontract.GatewayProvider { return &xorpayAdapter{} }

var (
	_ paymentcontract.GatewayProvider         = (*xorpayAdapter)(nil)
	_ paymentcontract.GatewayCallbackVerifier = (*xorpayAdapter)(nil)
)

// Type 返回 provider 标识。
func (a *xorpayAdapter) Type() string {
	return constants.PaymentProviderXorpay + ":"
}

func (a *xorpayAdapter) parseConfig(raw jsonmap.JSON) (*xorpay.Config, error) {
	cfg, err := xorpay.ParseConfig(raw)
	if err != nil {
		return nil, mapXorpayError(err)
	}
	if err := xorpay.ValidateConfig(cfg); err != nil {
		return nil, mapXorpayError(err)
	}
	return cfg, nil
}

// ValidateConfig 验证 channel.ConfigJSON 与 channel_type。
func (a *xorpayAdapter) ValidateConfig(raw jsonmap.JSON, channelType string) error {
	if channelType != "" && !xorpay.IsSupportedChannelType(channelType) {
		return fmt.Errorf("%w: xorpay channel_type %s", paymentcontract.ErrGatewayUnsupportedChannel, channelType)
	}
	_, err := a.parseConfig(raw)
	return err
}

// CreatePayment 调用统一下单。XorPay 只收人民币：配置了汇率时先换算，
// 换算后币种仍不是 CNY 则拒绝下单，避免按错误币种扣款。
func (a *xorpayAdapter) CreatePayment(ctx context.Context, raw jsonmap.JSON, input paymentcontract.GatewayCreateInput) (*paymentcontract.GatewayCreateResult, error) {
	if !xorpay.IsSupportedChannelType(input.ChannelType) {
		return nil, fmt.Errorf("%w: xorpay channel_type %s", paymentcontract.ErrGatewayUnsupportedChannel, input.ChannelType)
	}
	cfg, err := a.parseConfig(raw)
	if err != nil {
		return nil, err
	}

	originalAmount := input.Amount.Decimal.String()
	originalCurrency := strings.ToUpper(strings.TrimSpace(input.Currency))
	payAmount, payCurrency, err := cfg.ConvertAmount(originalAmount, originalCurrency, 2)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", paymentcontract.ErrGatewayConfigInvalid, err)
	}
	payCurrency = strings.ToUpper(strings.TrimSpace(payCurrency))
	if payCurrency == "" {
		payCurrency = xorpay.DefaultCurrency
	}
	if payCurrency != xorpay.DefaultCurrency {
		return nil, fmt.Errorf("%w: xorpay only supports CNY, got %s", paymentcontract.ErrGatewayConfigInvalid, payCurrency)
	}
	price, err := xorpay.FormatPrice(payAmount)
	if err != nil {
		return nil, mapXorpayError(err)
	}

	result, err := xorpay.CreatePayment(ctx, cfg, xorpay.CreateInput{
		OrderNo:     input.OrderNo,
		Name:        input.Subject,
		Price:       price,
		ChannelType: input.ChannelType,
		NotifyURL:   input.NotifyURL,
	})
	if err != nil {
		return nil, mapXorpayError(err)
	}

	payload := jsonmap.JSON{}
	for k, v := range result.Raw {
		payload[k] = v
	}
	if cfg.NeedsCurrencyConversion() {
		payload["exchange_rate"] = cfg.ExchangeRate
		payload["original_amount"] = originalAmount
		payload["original_currency"] = originalCurrency
	}

	// 支付宝当面付的 qr 链接在手机上可直接跳转拉起支付宝；微信 NATIVE 的 weixin:// 链接只能扫码。
	redirectURL := ""
	if xorpay.ResolvePayType(input.ChannelType) == xorpay.PayTypeAlipay {
		redirectURL = result.QRCode
	}
	return &paymentcontract.GatewayCreateResult{
		ProviderRef:  result.AOID,
		RedirectURL:  redirectURL,
		QRCodeURL:    result.QRCode,
		Payload:      payload,
		AmountSent:   price,
		CurrencySent: xorpay.DefaultCurrency,
	}, nil
}

// VerifyCallback 实现 paymentcontract.GatewayCallbackVerifier。XorPay 只在支付成功时回调。
func (a *xorpayAdapter) VerifyCallback(raw jsonmap.JSON, form map[string][]string, _ []byte) (*paymentcontract.GatewayCallbackResult, error) {
	cfg, err := xorpay.ParseConfig(raw)
	if err != nil {
		return nil, mapXorpayError(err)
	}
	data, err := xorpay.ParseCallback(form)
	if err != nil {
		return nil, mapXorpayError(err)
	}
	if err := xorpay.VerifyCallback(cfg, data); err != nil {
		return nil, mapXorpayError(err)
	}

	amount := money.Amount{}
	if d, parseErr := decimal.NewFromString(data.PayPrice); parseErr == nil {
		amount = money.FromDecimal(d)
	}
	return &paymentcontract.GatewayCallbackResult{
		OrderNo:     data.OrderID,
		ProviderRef: data.AOID,
		Status:      constants.PaymentStatusSuccess,
		Amount:      amount,
		Currency:    xorpay.DefaultCurrency,
		PaidAt:      xorpay.ParsePayTime(data.PayTime),
		Payload:     gatewaycommon.FormToJSON(form),
	}, nil
}

// mapXorpayError 把 xorpay 包的 sentinel error 映射为 provider 统一错误。
func mapXorpayError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, xorpay.ErrConfigInvalid):
		return fmt.Errorf("%w: %v", paymentcontract.ErrGatewayConfigInvalid, err)
	case errors.Is(err, xorpay.ErrChannelTypeNotOK):
		return fmt.Errorf("%w: %v", paymentcontract.ErrGatewayUnsupportedChannel, err)
	case errors.Is(err, xorpay.ErrRequestFailed):
		return fmt.Errorf("%w: %v", paymentcontract.ErrGatewayRequestFailed, err)
	case errors.Is(err, xorpay.ErrResponseInvalid):
		return fmt.Errorf("%w: %v", paymentcontract.ErrGatewayResponseInvalid, err)
	case errors.Is(err, xorpay.ErrSignatureInvalid):
		return fmt.Errorf("%w: %v", paymentcontract.ErrGatewaySignatureInvalid, err)
	default:
		return err
	}
}
