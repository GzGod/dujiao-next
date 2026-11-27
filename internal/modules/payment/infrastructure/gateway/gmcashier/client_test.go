package gmcashier

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestCashierOriginAndTradeValidation(t *testing.T) {
	for _, raw := range []string{"http://pay.xuegaogx.cc/cashier/a", "https://evil.test/cashier/a", "https://pay.xuegaogx.cc.evil.test/cashier/a", "https://pay.xuegaogx.cc@evil.test/cashier/a", "https://pay.xuegaogx.cc/admin"} {
		if _, err := tradeID(raw); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	if id, err := tradeID("https://pay.xuegaogx.cc/pay/checkout-counter/TEST-123"); err != nil || id != "TEST-123" {
		t.Fatal(id, err)
	}
}
func TestSelectUsesOwnedTradeAndValidatedAssets(t *testing.T) {
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	calls := 0
	http.DefaultTransport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "pay.xuegaogx.cc" {
			t.Fatal("untrusted host")
		}
		body := `{"status_code":200,"data":{"supported_assets":[{"network":"base","tokens":["USDT","USDC"]}]}}`
		if r.Method == "POST" {
			calls++
			data, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(data), `"trade_id":"OWNED-1"`) || !strings.Contains(string(data), `"token":"USDC"`) {
				t.Fatal(string(data))
			}
			body = `{"status_code":200,"data":{"trade_id":"CHILD-1","network":"base","token":"USDC","receive_address":"0x1111111111111111111111111111111111111111","actual_amount":10.01,"status":1}}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	client := Client{}
	for _, input := range [][2]string{{"evil", "USDC"}, {"base", "ETH"}} {
		if _, err := client.Cashier(context.Background(), "https://pay.xuegaogx.cc/cashier/OWNED-1", input[0], input[1]); err == nil {
			t.Fatal("accepted invalid asset")
		}
	}
	result, err := client.Cashier(context.Background(), "https://pay.xuegaogx.cc/cashier/OWNED-1", "base", "USDC")
	if err != nil || result["order"] == nil || calls != 1 {
		t.Fatal(result, err, calls)
	}
}

func TestConfirmExpiredRequiresExactTradeAndTerminalState(t *testing.T) {
	original := http.DefaultTransport
	defer func() { http.DefaultTransport = original }()
	for _, tc := range []struct {
		trade, status string
		expired       bool
	}{{"OWN", "1", false}, {"OWN", "2", false}, {"OWN", "3", true}, {"OWN", "4", false}, {"OWN", "5", false}, {"OWN", "6", false}, {"OTHER", "3", false}} {
		http.DefaultTransport = transportFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != "GET" || r.URL.Path != "/pay/check-status/OWN" {
				t.Fatal(r.Method, r.URL.Path)
			}
			body := "{\"status_code\":200,\"data\":{\"trade_id\":\"" + tc.trade + "\",\"status\":" + tc.status + "}}"
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		expired, _ := ConfirmExpired(context.Background(), "https://pay.xuegaogx.cc/cashier/OWN", "OWN")
		if expired != tc.expired {
			t.Fatal(tc)
		}
	}
}
