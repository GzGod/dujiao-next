package xorpay

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestSignMatchesDocumentedAlgorithm(t *testing.T) {
	// md5("abc") = 900150983cd24fb0d6963f7d28e17f72
	if got := Sign("a", "b", "c"); got != "900150983cd24fb0d6963f7d28e17f72" {
		t.Fatalf("unexpected sign: %s", got)
	}
}

func TestFormatPrice(t *testing.T) {
	cases := map[string]string{"50": "50.00", "0.1": "0.10", "18.805": "18.81"}
	for in, want := range cases {
		got, err := FormatPrice(in)
		if err != nil || got != want {
			t.Fatalf("FormatPrice(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "0", "-1", "abc"} {
		if _, err := FormatPrice(in); !errors.Is(err, ErrConfigInvalid) {
			t.Fatalf("FormatPrice(%q) should fail, got %v", in, err)
		}
	}
}

func TestValidateConfig(t *testing.T) {
	cfg, err := ParseConfig(map[string]interface{}{"aid": "1", "app_secret": "s", "notify_url": "https://api.example.com/cb"})
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.APIBaseURL != DefaultAPIBaseURL {
		t.Fatalf("default api base url not applied: %s", cfg.APIBaseURL)
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("ValidateConfig: %v", err)
	}
	for _, key := range []string{"aid", "app_secret", "notify_url"} {
		raw := map[string]interface{}{"aid": "1", "app_secret": "s", "notify_url": "https://api.example.com/cb"}
		delete(raw, key)
		bad, _ := ParseConfig(raw)
		if err := ValidateConfig(bad); !errors.Is(err, ErrConfigInvalid) {
			t.Fatalf("missing %s should fail, got %v", key, err)
		}
	}
}

func TestResolvePayType(t *testing.T) {
	if ResolvePayType("wechat") != PayTypeNative || ResolvePayType("wxpay") != PayTypeNative {
		t.Fatal("wechat should map to native")
	}
	if ResolvePayType("alipay") != PayTypeAlipay {
		t.Fatal("alipay should map to alipay")
	}
	if IsSupportedChannelType("usdt") {
		t.Fatal("usdt should not be supported")
	}
}

func TestCreatePaymentSendsSignedForm(t *testing.T) {
	var received url.Values
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		received = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","info":{"qr":"weixin://wxpay/bizpayurl?pr=abc"},"expires_in":7200,"aoid":"AOID-1"}`))
	}))
	defer server.Close()

	cfg := &Config{APIBaseURL: server.URL, AID: "aid-1", AppSecret: "secret", NotifyURL: "https://api.example.com/api/v1/payments/callback"}
	result, err := CreatePayment(context.Background(), cfg, CreateInput{
		OrderNo: "DJP1001", Name: "测试商品", Price: "50", ChannelType: "wechat",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if path != "/api/pay/aid-1" {
		t.Fatalf("unexpected path: %s", path)
	}
	if result.AOID != "AOID-1" || result.QRCode != "weixin://wxpay/bizpayurl?pr=abc" || result.ExpireIn != 7200 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if received.Get("price") != "50.00" || received.Get("pay_type") != "native" {
		t.Fatalf("unexpected form: %v", received)
	}
	want := Sign("测试商品", "native", "50.00", "DJP1001", cfg.NotifyURL, "secret")
	if received.Get("sign") != want {
		t.Fatalf("sign mismatch: got %s want %s", received.Get("sign"), want)
	}
}

func TestCreatePaymentReturnsErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"sign_error"}`))
	}))
	defer server.Close()
	cfg := &Config{APIBaseURL: server.URL, AID: "aid", AppSecret: "s", NotifyURL: "https://x.example.com"}
	_, err := CreatePayment(context.Background(), cfg, CreateInput{OrderNo: "1", Price: "1", ChannelType: "alipay"})
	if !errors.Is(err, ErrRequestFailed) {
		t.Fatalf("expected ErrRequestFailed, got %v", err)
	}
}

func TestVerifyCallback(t *testing.T) {
	cfg := &Config{AppSecret: "secret"}
	form := map[string][]string{
		"aoid":      {"AOID-1"},
		"order_id":  {"DJP1001"},
		"pay_price": {"50.00"},
		"pay_time":  {"2026-11-06 12:00:00"},
		"sign":      {Sign("AOID-1", "DJP1001", "50.00", "2026-11-06 12:00:00", "secret")},
	}
	data, err := ParseCallback(form)
	if err != nil {
		t.Fatalf("ParseCallback: %v", err)
	}
	if err := VerifyCallback(cfg, data); err != nil {
		t.Fatalf("VerifyCallback: %v", err)
	}
	data.PayPrice = "0.01"
	if err := VerifyCallback(cfg, data); !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("tampered price should fail, got %v", err)
	}
	paidAt := ParsePayTime("2026-11-06 12:00:00")
	if paidAt == nil || paidAt.UTC().Hour() != 4 {
		t.Fatalf("pay_time should be parsed as UTC+8, got %v", paidAt)
	}
}
