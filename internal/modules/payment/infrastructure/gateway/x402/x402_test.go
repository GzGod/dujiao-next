package x402

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
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
			_, _ = w.Write([]byte(`{"success":true,"transaction":"0x` + strings.Repeat("ab", 32) + `","network":"eip155:84532","payer":"0x01"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := testConfig(server.URL)
	now := time.Now()
	req, _, _ := BuildRequirements(cfg, decimal.RequireFromString("30"))
	useFakeRPC(t, NetworkBaseSepolia, matchingReceipt(req, validPayload(req, now).Payload.Authorization))

	result, err := VerifyAndSettle(context.Background(), cfg, req, validPayload(req, now))
	if err != nil {
		t.Fatal(err)
	}
	if !settleCalled || result.Transaction != "0x"+strings.Repeat("ab", 32) {
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

// ---- on-chain receipt tests --------------------------------------------------

// makeReceipt builds a successful receipt carrying a USDC Transfer log and an
// EIP-3009 AuthorizationUsed log, the two events verifyOnchainTransfer requires.
func makeReceipt(asset, payer, payTo, amountHex, nonce string) *ethReceipt {
	return &ethReceipt{
		Status: "0x1",
		Logs: []ethLog{
			{
				Address: strings.ToLower(asset),
				Topics:  []string{erc20TransferTopic, addressTopic(payer), addressTopic(payTo)},
				Data:    "0x" + amountHex,
			},
			{
				Address: strings.ToLower(asset),
				Topics:  []string{authorizationUsedTopic, addressTopic(payer), strings.ToLower(nonce)},
			},
		},
	}
}

// matchingReceipt returns a receipt that satisfies req for the given authorization.
func matchingReceipt(req *Requirements, auth Authorization) *ethReceipt {
	amt, _ := new(big.Int).SetString(req.Amount, 10)
	return makeReceipt(req.Asset, auth.From, req.PayTo, fmt.Sprintf("%064x", amt), auth.Nonce)
}

// useFakeRPC points network's RPC at a test server that returns receipt
// (nil = "not mined yet") and shrinks the polling budget for the test.
func useFakeRPC(t *testing.T, network string, receipt *ethReceipt) *int {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Method != "eth_getTransactionReceipt" {
			http.Error(w, "unexpected method", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": 1, "result": receipt})
	}))
	restore := SetRPCURLForTest(network, srv.URL)
	t.Cleanup(func() { restore(); srv.Close() })
	return &calls
}

func TestMatchReceiptPass(t *testing.T) {
	req, _, err := BuildRequirements(testConfig(""), decimal.RequireFromString("51"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	payload := validPayload(req, now)
	auth := payload.Payload.Authorization

	// amountHex must be req.Amount (atomic) in hex, left-padded to 32 bytes
	atomicBig := new(big.Int)
	atomicBig.SetString(req.Amount, 10)
	amountHex := fmt.Sprintf("%064x", atomicBig)

	receipt := makeReceipt(req.Asset, auth.From, req.PayTo, amountHex, auth.Nonce)
	if err := matchReceipt(receipt, "0x"+strings.Repeat("aa", 32), req, auth); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestMatchReceiptRejectsWrongAmount(t *testing.T) {
	req, _, err := BuildRequirements(testConfig(""), decimal.RequireFromString("51"))
	if err != nil {
		t.Fatal(err)
	}
	payload := validPayload(req, time.Now())
	auth := payload.Payload.Authorization
	// off by one atomic unit
	wrongAmt := new(big.Int)
	wrongAmt.SetString(req.Amount, 10)
	wrongAmt.Sub(wrongAmt, big.NewInt(1))
	receipt := makeReceipt(req.Asset, auth.From, req.PayTo, fmt.Sprintf("%064x", wrongAmt), auth.Nonce)
	if err := matchReceipt(receipt, "0x"+strings.Repeat("bb", 32), req, auth); !errors.Is(err, ErrOnchainMismatch) {
		t.Fatalf("expected ErrOnchainMismatch, got: %v", err)
	}
}

func TestMatchReceiptRejectsWrongPayTo(t *testing.T) {
	req, _, err := BuildRequirements(testConfig(""), decimal.RequireFromString("100"))
	if err != nil {
		t.Fatal(err)
	}
	payload := validPayload(req, time.Now())
	auth := payload.Payload.Authorization
	atomicBig := new(big.Int)
	atomicBig.SetString(req.Amount, 10)
	receipt := makeReceipt(req.Asset, auth.From, "0x000000000000000000000000000000000000dead", fmt.Sprintf("%064x", atomicBig), auth.Nonce)
	if err := matchReceipt(receipt, "0x"+strings.Repeat("cc", 32), req, auth); !errors.Is(err, ErrOnchainMismatch) {
		t.Fatalf("expected ErrOnchainMismatch (wrong payTo), got: %v", err)
	}
}

func TestMatchReceiptRejectsWrongNonce(t *testing.T) {
	req, _, err := BuildRequirements(testConfig(""), decimal.RequireFromString("100"))
	if err != nil {
		t.Fatal(err)
	}
	payload := validPayload(req, time.Now())
	auth := payload.Payload.Authorization
	atomicBig := new(big.Int)
	atomicBig.SetString(req.Amount, 10)
	wrongNonce := "0x" + strings.Repeat("99", 32)
	receipt := makeReceipt(req.Asset, auth.From, req.PayTo, fmt.Sprintf("%064x", atomicBig), wrongNonce)
	if err := matchReceipt(receipt, "0x"+strings.Repeat("dd", 32), req, auth); !errors.Is(err, ErrOnchainMismatch) {
		t.Fatalf("expected ErrOnchainMismatch (wrong nonce), got: %v", err)
	}
}

func TestMatchReceiptRejectsRevertedTx(t *testing.T) {
	req, _, _ := BuildRequirements(testConfig(""), decimal.RequireFromString("1"))
	receipt := &ethReceipt{Status: "0x0", Logs: nil}
	auth := Authorization{From: "0x01", Nonce: "0x" + strings.Repeat("00", 32)}
	if err := matchReceipt(receipt, "0xtx", req, auth); !errors.Is(err, ErrOnchainMismatch) {
		t.Fatalf("expected ErrOnchainMismatch for reverted tx, got: %v", err)
	}
}

func TestVerifyAndSettleRejectsWhenChainDisagrees(t *testing.T) {
	txHash := "0x" + strings.Repeat("ef", 32)
	facilitator := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/verify" {
			_, _ = w.Write([]byte(`{"isValid":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"transaction":"` + txHash + `","network":"eip155:84532"}`))
	}))
	defer facilitator.Close()

	cfg := testConfig(facilitator.URL)
	now := time.Now()
	req, _, _ := BuildRequirements(cfg, decimal.RequireFromString("30"))
	auth := validPayload(req, now).Payload.Authorization

	// Facilitator claims success but the tx paid someone else.
	amt, _ := new(big.Int).SetString(req.Amount, 10)
	wrong := makeReceipt(req.Asset, auth.From, "0x000000000000000000000000000000000000dead", fmt.Sprintf("%064x", amt), auth.Nonce)
	calls := useFakeRPC(t, NetworkBaseSepolia, wrong)
	if _, err := VerifyAndSettle(context.Background(), cfg, req, validPayload(req, now)); !errors.Is(err, ErrOnchainMismatch) {
		t.Fatalf("expected ErrOnchainMismatch, got %v", err)
	}
	if *calls == 0 {
		t.Fatal("on-chain RPC must be consulted after facilitator settle")
	}
}

func TestVerifyAndSettleUnconfirmedWhenTxMissing(t *testing.T) {
	facilitator := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/verify" {
			_, _ = w.Write([]byte(`{"isValid":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"transaction":"0x` + strings.Repeat("12", 32) + `","network":"eip155:84532"}`))
	}))
	defer facilitator.Close()

	cfg := testConfig(facilitator.URL)
	now := time.Now()
	req, _, _ := BuildRequirements(cfg, decimal.RequireFromString("30"))
	calls := useFakeRPC(t, NetworkBaseSepolia, nil) // never mined
	if _, err := VerifyAndSettle(context.Background(), cfg, req, validPayload(req, now)); !errors.Is(err, ErrOnchainUnconfirmed) {
		t.Fatalf("expected ErrOnchainUnconfirmed, got %v", err)
	}
	if *calls < 2 {
		t.Fatalf("expected polling while tx is not mined, got %d calls", *calls)
	}
}
