package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	siteconnectiondomain "github.com/dujiao-next/internal/modules/siteconnection/domain"
)

func gptgetTestAdapter(t *testing.T, handler http.HandlerFunc) *GptgetAdapter {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		ts, _ := strconv.ParseInt(r.Header.Get(HeaderTimestamp), 10, 64)
		if r.Header.Get(HeaderApiKey) != "test-key" || !Verify("test-secret", r.Method, r.URL.EscapedPath(), r.Header.Get(HeaderSignature), ts, body) {
			t.Errorf("invalid signed request: %s %s", r.Method, r.URL.Path)
		}
		r.Body = io.NopCloser(strings.NewReader(string(body)))
		w.Header().Set("Content-Type", "application/json")
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return NewGptgetAdapter(&siteconnectiondomain.Connection{BaseURL: server.URL, ApiKey: "test-key", ApiSecret: "test-secret"})
}

func TestGptgetPingReadsAccountAndCapabilities(t *testing.T) {
	for _, state := range []string{"ready", "uninitialized", "review_required"} {
		t.Run(state, func(t *testing.T) {
			a := gptgetTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case gptgetBasePath + "/capabilities":
					io.WriteString(w, `{"ok":true,"data":{"business_operations_enabled":true,"available_operations":["products","create_order"]}}`)
				case gptgetBasePath + "/account":
					fmt.Fprintf(w, `{"ok":true,"data":{"user_id":7,"balance":"123.45","currency":"CNY","funding_state":%q}}`, state)
				default:
					t.Errorf("unexpected endpoint %s", r.URL.Path)
				}
			})
			result, err := a.Ping(context.Background())
			if err != nil || result.Balance != "123.45" || result.UserID != 7 || result.Currency != "CNY" || result.FundingState != state || result.BusinessOperationsEnabled == nil || !*result.BusinessOperationsEnabled {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestGptgetAccountFailureIsNotZeroBalance(t *testing.T) {
	a := gptgetTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/account") {
			w.WriteHeader(409)
			io.WriteString(w, `{"ok":false,"error_code":"wallet_setup_required"}`)
			return
		}
		io.WriteString(w, `{"ok":true,"data":{}}`)
	})
	if result, err := a.Ping(context.Background()); err == nil || result != nil {
		t.Fatal("wallet setup failure must not become a successful zero balance")
	}
}

func TestGptgetCatalogHasMoreWithoutTotal(t *testing.T) {
	a := gptgetTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		if page == "1" {
			items := make([]map[string]interface{}, 50)
			for i := range items {
				items[i] = map[string]interface{}{"product_id": i + 1, "sku_id": i + 1, "product_name": fmt.Sprintf("P%d", i+1), "supply_price": "10.00", "available": true}
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "data": map[string]interface{}{"items": items, "has_more": true, "page_size": 50, "page": 1}})
		} else if page == "2" {
			io.WriteString(w, `{"ok":true,"data":{"items":[{"product_id":51,"sku_id":51,"supply_price":"20.00","available":false},{"product_id":1,"sku_id":99,"supply_price":"11.00","available":true}],"has_more":false}}`)
		} else {
			t.Errorf("unexpected page %s", page)
		}
	})
	result, err := a.ListProducts(context.Background(), ListProductsOpts{Page: 1, PageSize: 100})
	if err != nil || result.Total != 51 || len(result.Items) != 51 || len(result.Items[0].SKUs) != 2 || result.IncludesInactive {
		t.Fatalf("pagination/grouping failed: %+v %v", result, err)
	}
	product, err := a.GetProduct(context.Background(), 51)
	if err != nil || product.IsActive || product.SKUs[0].StockQuantity != 0 {
		t.Fatalf("second upstream page missed: %+v %v", product, err)
	}
	localPage, err := a.ListProducts(context.Background(), ListProductsOpts{Page: 2, PageSize: 50})
	if err != nil || len(localPage.Items) != 1 || localPage.Items[0].ID != 51 {
		t.Fatalf("local pagination failed: %+v %v", localPage, err)
	}
}

func TestGptgetRejectsTruncatedCatalog(t *testing.T) {
	a := gptgetTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"ok":true,"data":{"items":[],"has_more":true}}`)
	})
	if _, err := a.ListProducts(context.Background(), ListProductsOpts{}); err == nil {
		t.Fatal("empty continuation page must fail")
	}
}

func gptgetPurchaseRequest() CreateUpstreamOrderReq {
	return CreateUpstreamOrderReq{SKUID: 3, Quantity: 1, TraceID: "fixed-request-001", DownstreamOrderNo: "local-order-001", ManualFormData: map[string]interface{}{"expected_unit_price": "109.82", "currency": "CNY"}}
}

func TestGptgetCreateRecoversOriginalOrder(t *testing.T) {
	a := gptgetTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/orders/by-external/") {
			t.Errorf("must recover without purchase: %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, `{"ok":true,"data":{"order_no":"AP-original","financial_status":"paid","charged_amount":"109.82","currency":"CNY"}}`)
	})
	result, err := a.CreateOrder(context.Background(), gptgetPurchaseRequest())
	if err != nil || !result.OK || result.OrderNo != "AP-original" {
		t.Fatalf("recovery failed: %+v %v", result, err)
	}
}

func TestGptgetCreateSendsExactSignedContract(t *testing.T) {
	posts := 0
	a := gptgetTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(404)
			io.WriteString(w, `{"ok":false,"error_code":"order_not_found"}`)
			return
		}
		posts++
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		if len(body) != 5 || body["request_id"] != "fixed-request-001" || body["external_order_no"] != "local-order-001" || body["expected_unit_price"] != "109.82" || body["currency"] != "CNY" {
			t.Errorf("unexpected purchase payload %+v", body)
		}
		io.WriteString(w, `{"ok":true,"data":{"order_no":"AP-new","financial_status":"paid","charged_amount":"109.82","currency":"CNY"}}`)
	})
	result, err := a.CreateOrder(context.Background(), gptgetPurchaseRequest())
	if err != nil || !result.OK || posts != 1 {
		t.Fatalf("create failed: %+v %v posts=%d", result, err, posts)
	}
	request := gptgetPurchaseRequest()
	request.Quantity = 2
	result, err = a.CreateOrder(context.Background(), request)
	if err != nil || result.OK || posts != 1 {
		t.Fatal("must reject multi-quantity without purchasing")
	}
}

func TestGptgetLookupFailureNeverPurchases(t *testing.T) {
	a := gptgetTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("must not POST on unknown lookup result")
		}
		w.WriteHeader(503)
		io.WriteString(w, `{"ok":false,"error_code":"service_unavailable"}`)
	})
	result, err := a.CreateOrder(context.Background(), gptgetPurchaseRequest())
	if err == nil && (result.OK || result.ErrorCode != "service_unavailable") {
		t.Fatal("lookup error ignored")
	}
}

func TestGptgetBusinessFailureAndSensitiveBody(t *testing.T) {
	a := gptgetTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"ok":false,"error_code":"credential_restricted","data":{"code":"sensitive-code"}}`)
	})
	_, err := a.ListProducts(context.Background(), ListProductsOpts{})
	if err == nil || strings.Contains(err.Error(), "sensitive-code") {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestGptgetRefundUsesCurrentRevisionAndStableID(t *testing.T) {
	posts := 0
	a := gptgetTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != gptgetBasePath+"/orders/AP-123" && r.URL.Path != gptgetBasePath+"/orders/AP-123/refund" {
			t.Errorf("wrong string order path %s", r.URL.Path)
		}
		if r.Method == http.MethodGet {
			io.WriteString(w, `{"ok":true,"data":{"code_revision":4,"can_refund":true,"financial_status":"paid"}}`)
			return
		}
		posts++
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		if body["expected_code_revision"] != float64(4) || body["request_id"] != "fixed-refund-001" {
			t.Errorf("bad refund payload %+v", body)
		}
		io.WriteString(w, `{"ok":true,"data":{"financial_status":"refunded"}}`)
	})
	if err := a.CancelOrderByNo(context.Background(), "AP-123", "fixed-refund-001"); err != nil || posts != 1 {
		t.Fatalf("refund failed: %v posts=%d", err, posts)
	}
	if err := a.CancelOrder(context.Background(), 123); err == nil || posts != 1 {
		t.Fatal("numeric refund must fail without network")
	}
}

func TestGptgetRefundConflictDoesNotRetry(t *testing.T) {
	posts := 0
	a := gptgetTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			io.WriteString(w, `{"ok":true,"data":{"code_revision":4,"can_refund":true}}`)
			return
		}
		posts++
		w.WriteHeader(409)
		io.WriteString(w, `{"ok":false,"error_code":"revision_conflict"}`)
	})
	if err := a.CancelOrderByNo(context.Background(), "AP-123", "fixed-refund-001"); err == nil || posts != 1 {
		t.Fatal("conflicting revision must stop")
	}
}

func TestGptgetRefundedCodeNeverDelivered(t *testing.T) {
	a := gptgetTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"ok":true,"data":{"order_no":"AP-1","code":"stale-code","financial_status":"refunded","progress_status":"destroyed"}}`)
	})
	result, err := a.GetOrderByNo(context.Background(), "AP-1")
	if err != nil || result.Status != "refunded" || result.Fulfillment != nil {
		t.Fatalf("stale code leaked: %+v %v", result, err)
	}
}

func TestGptgetOfficialSignatureVector(t *testing.T) {
	body := []byte(`{"request_id":"purchase-request-001","external_order_no":"your-order-001","sku_id":123,"expected_unit_price":"80.00","currency":"CNY"}`)
	expected := "d3035fd735359d71f5eee0cce2b099b1d6a6d0976d9a18ed6ad75a83f760f319"
	if got := Sign("TEST_ONLY_SECRET", "POST", gptgetBasePath+"/orders", 1793872800, body); got != expected {
		t.Fatalf("official signature mismatch: %s", got)
	}
}
func TestGptgetNeverFollowsCredentialRedirect(t *testing.T) {
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected = true }))
	defer target.Close()
	a := gptgetTestAdapter(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) })
	if _, err := a.Ping(context.Background()); err == nil || redirected {
		t.Fatal("redirect followed or accepted")
	}
}

func TestRedemptionAfterSaleSignsExactRevisionAndStableIdentity(t *testing.T) {
	calls := 0
	a := gptgetTestAdapter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Fatal("unexpected method")
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["request_id"] != "fixed-id" || body["expected_code_revision"] != float64(3) {
			t.Fatalf("wrong operation identity: %v", body)
		}
		switch r.URL.Path {
		case gptgetBasePath + "/orders/AP-ONE/replace":
			io.WriteString(w, `{"ok":true,"data":{"code":"new-code","code_revision":4,"financial_status":"paid","progress_status":"unused"}}`)
		case gptgetBasePath + "/orders/AP-ONE/refund":
			io.WriteString(w, `{"ok":true,"data":{"code_revision":3,"financial_status":"refunded","progress_status":"destroyed"}}`)
		default:
			t.Fatal("unexpected route")
		}
		calls++
	})
	state, err := a.ReplaceRedemption(context.Background(), "AP-ONE", "fixed-id", 3)
	if err != nil || state.Revision != 4 || state.Code != "new-code" {
		t.Fatalf("replace: %v %v", state, err)
	}
	state, err = a.RefundRedemption(context.Background(), "AP-ONE", "fixed-id", 3)
	if err != nil || state.Financial != "refunded" || state.Progress != "destroyed" {
		t.Fatalf("refund: %v %v", state, err)
	}
	if calls != 2 {
		t.Fatal("adapter retried mutation")
	}
}
