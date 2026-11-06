package x402adapter

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	paymentcontract "github.com/dujiao-next/internal/modules/payment/contract"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/money"

	"github.com/shopspring/decimal"
)

const testPayTo = "0x123e20e5a62a0797aaeae169d3ba2911ca888888"

func baseConfig(facilitator string) jsonmap.JSON {
	return jsonmap.JSON{
		"pay_to":          testPayTo,
		"exchange_rate":   "0.14",
		"facilitator_url": facilitator,
	}
}

func TestValidateConfig(t *testing.T) {
	a := NewX402Adapter()
	if err := a.ValidateConfig(baseConfig(""), constants.PaymentChannelTypeX402USDC); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	if err := a.ValidateConfig(baseConfig(""), "usdt"); !errors.Is(err, paymentcontract.ErrGatewayUnsupportedChannel) {
		t.Fatalf("expected unsupported channel, got %v", err)
	}
	if err := a.ValidateConfig(jsonmap.JSON{"pay_to": "0x1"}, ""); !errors.Is(err, paymentcontract.ErrGatewayConfigInvalid) {
		t.Fatalf("expected config invalid, got %v", err)
	}
}

func createPayment(t *testing.T, facilitator string) *paymentcontract.GatewayCreateResult {
	t.Helper()
	result, err := NewX402Adapter().CreatePayment(context.Background(), baseConfig(facilitator), paymentcontract.GatewayCreateInput{
		OrderNo:  "DJP-1",
		Amount:   money.FromDecimal(decimal.RequireFromString("30")),
		Currency: "CNY",
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestCreatePaymentSnapshotsRequirements(t *testing.T) {
	result := createPayment(t, "")
	if result.QRCodeURL != "eip155:84532:"+testPayTo {
		t.Fatalf("payment must carry a resumable identifier, got %q", result.QRCodeURL)
	}
	if result.AmountSent != "4.20" || result.CurrencySent != "USDC" {
		t.Fatalf("unexpected amount: %s %s", result.AmountSent, result.CurrencySent)
	}
	snap, ok := result.Payload[PayloadKeyRequirements].(jsonmap.JSON)
	if !ok || snap["amount"] != "4200000" || snap["payTo"] != testPayTo {
		t.Fatalf("missing requirements snapshot: %#v", result.Payload)
	}
	if result.Payload["exchange_rate"] != "0.14" || result.Payload["original_amount"] != "30" {
		t.Fatalf("missing exchange snapshot: %#v", result.Payload)
	}
}

func signedPayload(amount string, now time.Time) jsonmap.JSON {
	return jsonmap.JSON{
		"x402Version": 2,
		"accepted": jsonmap.JSON{
			"scheme": "exact", "network": "eip155:84532", "amount": amount,
			"asset": "0x036CbD53842c5426634e7929541eC2318f3dCF7e", "payTo": testPayTo,
			"maxTimeoutSeconds": 600, "extra": jsonmap.JSON{"name": "USDC", "version": "2"},
		},
		"payload": jsonmap.JSON{
			"signature": "0x" + strings.Repeat("ab", 65),
			"authorization": jsonmap.JSON{
				"from": "0x0000000000000000000000000000000000000001", "to": testPayTo,
				"value": amount, "validAfter": "0",
				"validBefore": strconv.FormatInt(now.Unix()+300, 10),
				"nonce":       "0x" + strings.Repeat("11", 32),
			},
		},
	}
}

func TestSettlePayloadSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/verify" {
			_, _ = w.Write([]byte(`{"isValid":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"transaction":"0xdeadbeef","network":"eip155:84532"}`))
	}))
	defer server.Close()

	created := createPayment(t, server.URL)
	settler := NewX402Adapter().(paymentcontract.GatewayPayloadSettler)
	result, err := settler.SettlePayload(context.Background(), baseConfig(server.URL), created.Payload, signedPayload("4200000", time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != constants.PaymentStatusSuccess || result.ProviderRef != "0xdeadbeef" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Amount.Decimal.String() != "4.2" || result.Currency != "USDC" {
		t.Fatalf("amount must come from server snapshot: %s %s", result.Amount.Decimal.String(), result.Currency)
	}
}

func TestSettlePayloadRejectsUnderpayWithoutCallingFacilitator(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = w.Write([]byte(`{"isValid":true}`))
	}))
	defer server.Close()

	created := createPayment(t, server.URL)
	settler := NewX402Adapter().(paymentcontract.GatewayPayloadSettler)
	_, err := settler.SettlePayload(context.Background(), baseConfig(server.URL), created.Payload, signedPayload("1", time.Now()))
	if !errors.Is(err, paymentcontract.ErrGatewaySignatureInvalid) {
		t.Fatalf("expected signature invalid, got %v", err)
	}
	if called {
		t.Fatal("facilitator must not be called for tampered payload")
	}
}
