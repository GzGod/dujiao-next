package xorpayadapter

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	paymentcontract "github.com/dujiao-next/internal/modules/payment/contract"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/modules/payment/infrastructure/gateway/xorpay"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/money"

	"github.com/shopspring/decimal"
)

func baseConfig(apiBaseURL string) jsonmap.JSON {
	return jsonmap.JSON{
		"api_base_url": apiBaseURL,
		"aid":          "aid-1",
		"app_secret":   "secret",
		"notify_url":   "https://api.example.com/api/v1/payments/callback",
	}
}

func TestValidateConfigRejectsUnsupportedChannel(t *testing.T) {
	a := NewXorpayAdapter()
	if err := a.ValidateConfig(baseConfig(""), "usdt"); !errors.Is(err, paymentcontract.ErrGatewayUnsupportedChannel) {
		t.Fatalf("expected unsupported channel, got %v", err)
	}
	if err := a.ValidateConfig(baseConfig(""), "alipay"); err != nil {
		t.Fatalf("alipay should be valid: %v", err)
	}
	if err := a.ValidateConfig(jsonmap.JSON{"aid": "1"}, "wechat"); !errors.Is(err, paymentcontract.ErrGatewayConfigInvalid) {
		t.Fatalf("expected config invalid, got %v", err)
	}
}

func TestCreatePaymentAlipayWithExchangeRate(t *testing.T) {
	var price string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		price = r.PostForm.Get("price")
		_, _ = w.Write([]byte(`{"status":"ok","info":{"qr":"https://qr.alipay.com/abc"},"aoid":"AOID-9"}`))
	}))
	defer server.Close()

	cfg := baseConfig(server.URL)
	cfg["target_currency"] = "CNY"
	cfg["exchange_rate"] = "7.2"
	result, err := NewXorpayAdapter().CreatePayment(context.Background(), cfg, paymentcontract.GatewayCreateInput{
		OrderNo: "DJP1", Subject: "商品", Amount: money.FromDecimal(decimal.RequireFromString("10")),
		Currency: "USD", ChannelType: "alipay",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if price != "72.00" || result.AmountSent != "72.00" || result.CurrencySent != "CNY" {
		t.Fatalf("unexpected conversion: price=%s result=%+v", price, result)
	}
	if result.ProviderRef != "AOID-9" || result.QRCodeURL != "https://qr.alipay.com/abc" || result.RedirectURL != "https://qr.alipay.com/abc" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Payload["original_currency"] != "USD" {
		t.Fatalf("payload should keep original currency: %v", result.Payload)
	}
}

func TestCreatePaymentRejectsNonCNY(t *testing.T) {
	_, err := NewXorpayAdapter().CreatePayment(context.Background(), baseConfig("https://xorpay.example"), paymentcontract.GatewayCreateInput{
		OrderNo: "DJP1", Amount: money.FromDecimal(decimal.RequireFromString("10")), Currency: "USD", ChannelType: "wechat",
	})
	if !errors.Is(err, paymentcontract.ErrGatewayConfigInvalid) {
		t.Fatalf("expected config invalid for USD, got %v", err)
	}
}

func TestVerifyCallback(t *testing.T) {
	form := map[string][]string{
		"aoid":      {"AOID-1"},
		"order_id":  {"DJP1001"},
		"pay_price": {"50.00"},
		"pay_time":  {"2026-11-06 12:00:00"},
		"sign":      {xorpay.Sign("AOID-1", "DJP1001", "50.00", "2026-11-06 12:00:00", "secret")},
	}
	verifier := NewXorpayAdapter().(paymentcontract.GatewayCallbackVerifier)
	result, err := verifier.VerifyCallback(baseConfig(""), form, nil)
	if err != nil {
		t.Fatalf("VerifyCallback: %v", err)
	}
	if result.OrderNo != "DJP1001" || result.ProviderRef != "AOID-1" || result.Status != constants.PaymentStatusSuccess ||
		result.Currency != "CNY" || result.Amount.Decimal.StringFixed(2) != "50.00" || result.PaidAt == nil {
		t.Fatalf("unexpected result: %+v", result)
	}

	form["sign"] = []string{"bad"}
	if _, err := verifier.VerifyCallback(baseConfig(""), form, nil); !errors.Is(err, paymentcontract.ErrGatewaySignatureInvalid) {
		t.Fatalf("expected signature invalid, got %v", err)
	}
}
