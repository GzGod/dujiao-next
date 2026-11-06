package x402

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

const testPayTo = "0x123e20e5a62a0797aaeae169d3ba2911ca888888"

func testConfig(facilitator string) *Config {
	cfg := &Config{PayTo: testPayTo, ExchangeRate: "0.14", FacilitatorURL: facilitator}
	cfg.Normalize()
	return cfg
}

func TestValidateConfig(t *testing.T) {
	cases := map[string]*Config{
		"bad address":  {PayTo: "0x123", ExchangeRate: "0.14"},
		"no rate":      {PayTo: testPayTo},
		"zero rate":    {PayTo: testPayTo, ExchangeRate: "0"},
		"bad network":  {PayTo: testPayTo, ExchangeRate: "0.14", Network: "eip155:1"},
		"long timeout": {PayTo: testPayTo, ExchangeRate: "0.14", MaxTimeoutSeconds: 99999},
	}
	for name, cfg := range cases {
		cfg.Normalize()
		if err := ValidateConfig(cfg); !errors.Is(err, ErrConfigInvalid) {
			t.Errorf("%s: expected ErrConfigInvalid, got %v", name, err)
		}
	}
	if err := ValidateConfig(testConfig("")); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	if got := testConfig("").Network; got != NetworkBaseSepolia {
		t.Fatalf("default network = %s", got)
	}
}

func TestBuildRequirementsRoundsUpToCent(t *testing.T) {
	// 30 CNY × 0.14 = 4.2 USDC；30.01 × 0.14 = 4.2014 → 向上取整 4.21
	req, display, err := BuildRequirements(testConfig(""), decimal.RequireFromString("30.01"))
	if err != nil {
		t.Fatal(err)
	}
	if display != "4.21" || req.Amount != "4210000" {
		t.Fatalf("got display=%s amount=%s", display, req.Amount)
	}
	if req.Asset != knownAssets[NetworkBaseSepolia].Address || req.Extra["name"] != "USDC" || req.Extra["version"] != "2" {
		t.Fatalf("unexpected asset info: %+v", req)
	}
	if req.PayTo != testPayTo || req.Scheme != SchemeExact || req.Network != NetworkBaseSepolia {
		t.Fatalf("unexpected requirements: %+v", req)
	}
}

func validPayload(req *Requirements, now time.Time) *PaymentPayload {
	return &PaymentPayload{
		X402Version: Version,
		Accepted:    *req,
		Payload: ExactPayload{
			Signature: "0x" + strings.Repeat("ab", 65),
			Authorization: Authorization{
				From:        "0x0000000000000000000000000000000000000001",
				To:          req.PayTo,
				Value:       req.Amount,
				ValidAfter:  "0",
				ValidBefore: strconv.FormatInt(now.Unix()+300, 10),
				Nonce:       "0x" + strings.Repeat("11", 32),
			},
		},
	}
}

func TestValidatePayloadRejectsTampering(t *testing.T) {
	now := time.Now()
	req, _, _ := BuildRequirements(testConfig(""), decimal.RequireFromString("30"))

	if err := ValidatePayload(req, validPayload(req, now), now); err != nil {
		t.Fatalf("valid payload rejected: %v", err)
	}

	mutations := map[string]func(p *PaymentPayload){
		"lower value":       func(p *PaymentPayload) { p.Payload.Authorization.Value = "1" },
		"other recipient":   func(p *PaymentPayload) { p.Payload.Authorization.To = "0x0000000000000000000000000000000000000002" },
		"accepted amount":   func(p *PaymentPayload) { p.Accepted.Amount = "1" },
		"accepted payTo":    func(p *PaymentPayload) { p.Accepted.PayTo = "0x0000000000000000000000000000000000000002" },
		"accepted network":  func(p *PaymentPayload) { p.Accepted.Network = NetworkBase },
		"accepted asset":    func(p *PaymentPayload) { p.Accepted.Asset = "0x0000000000000000000000000000000000000003" },
		"expired":           func(p *PaymentPayload) { p.Payload.Authorization.ValidBefore = strconv.FormatInt(now.Unix()-1, 10) },
		"too far in future": func(p *PaymentPayload) { p.Payload.Authorization.ValidBefore = strconv.FormatInt(now.Unix()+86400, 10) },
		"bad nonce":         func(p *PaymentPayload) { p.Payload.Authorization.Nonce = "0x01" },
		"bad signature":     func(p *PaymentPayload) { p.Payload.Signature = "0x00" },
		"bad version":       func(p *PaymentPayload) { p.X402Version = 1 },
	}
	for name, mutate := range mutations {
		p := validPayload(req, now)
		mutate(p)
		if err := ValidatePayload(req, p, now); !errors.Is(err, ErrPaymentInvalid) {
			t.Errorf("%s: expected ErrPaymentInvalid, got %v", name, err)
		}
	}
}

func TestVerifyAndSettle(t *testing.T) {
	var settleCalled bool
	var gotRequirements Requirements
	verifyValid := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			PaymentRequirements Requirements `json:"paymentRequirements"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotRequirements = body.PaymentRequirements
		switch r.URL.Path {
		case "/verify":
			if verifyValid {
				_, _ = w.Write([]byte(`{"isValid":true,"payer":"0x01"}`))
			} else {
				_, _ = w.Write([]byte(`{"isValid":false,"invalidReason":"insufficient_funds"}`))
			}
		case "/settle":
			settleCalled = true
			_, _ = w.Write([]byte(`{"success":true,"transaction":"0xabc","network":"eip155:84532","payer":"0x01"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := testConfig(server.URL)
	now := time.Now()
	req, _, _ := BuildRequirements(cfg, decimal.RequireFromString("30"))

	result, err := VerifyAndSettle(context.Background(), cfg, req, validPayload(req, now))
	if err != nil {
		t.Fatal(err)
	}
	if !settleCalled || result.Transaction != "0xabc" {
		t.Fatalf("settle not executed: %+v", result)
	}
	if gotRequirements.Amount != req.Amount || gotRequirements.PayTo != testPayTo {
		t.Fatalf("facilitator received wrong requirements: %+v", gotRequirements)
	}

	settleCalled = false
	verifyValid = false
	if _, err := VerifyAndSettle(context.Background(), cfg, req, validPayload(req, now)); !errors.Is(err, ErrPaymentRejected) {
		t.Fatalf("expected ErrPaymentRejected, got %v", err)
	}
	if settleCalled {
		t.Fatal("settle must not run when verify fails")
	}
}

func TestVerifyAndSettleRejectsFailedSettlement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/verify" {
			_, _ = w.Write([]byte(`{"isValid":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":false,"transaction":"","network":"eip155:84532","errorReason":"nonce_used"}`))
	}))
	defer server.Close()
	cfg := testConfig(server.URL)
	now := time.Now()
	req, _, _ := BuildRequirements(cfg, decimal.RequireFromString("30"))
	if _, err := VerifyAndSettle(context.Background(), cfg, req, validPayload(req, now)); !errors.Is(err, ErrPaymentRejected) {
		t.Fatalf("expected ErrPaymentRejected, got %v", err)
	}
}
