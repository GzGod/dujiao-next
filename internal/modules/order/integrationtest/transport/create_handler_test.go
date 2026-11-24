package transport_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	orderhttp "github.com/dujiao-next/internal/modules/order/transport/http"
	paymentdomain "github.com/dujiao-next/internal/modules/payment/domain"
	"github.com/dujiao-next/internal/shared/money"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type amountTestPaymentCreator struct {
	result *orderhttp.CreatePaymentResult
}

func (p amountTestPaymentCreator) CreatePayment(orderhttp.CreatePaymentInput) (*orderhttp.CreatePaymentResult, error) {
	return p.result, nil
}

func TestCreateAndPayPreservesSettlementAmountAndCurrency(t *testing.T) {
	amount := func(s string) money.Amount { return money.FromDecimal(decimal.RequireFromString(s)) }
	for _, tc := range []struct{ name, payable, currency string }{
		{"wallet_and_customer_fee", "62.00", "CNY"},
		{"converted_currency", "7.50", "GBP"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest("POST", "/api/v1/orders/create-and-pay", strings.NewReader(`{"items":[{"product_id":1,"quantity":1}],"channel_id":1,"use_balance":true}`))
			ctx.Request.Header.Set("Content-Type", "application/json")
			ctx.Set("user_id", uint(1))
			creator := amountTestPaymentCreator{result: &orderhttp.CreatePaymentResult{
				OnlinePayAmount: amount("60"), WalletPaidAmount: amount("40"),
				Payment: &paymentdomain.Payment{Amount: amount(tc.payable), Currency: tc.currency, FeeAmount: amount("2"), FeePolicy: "customer_surcharge"},
			}}
			orders := amountTestOrderCreator{order: &orderdomain.Order{OrderNo: "TEST-AMOUNT", Currency: "CNY", TotalAmount: amount("100")}}
			handler := orderhttp.NewCreateHandler(orders, nil, nil, creator)
			handler.CreateOrderAndPay(ctx)
			if recorder.Code != 200 {
				t.Fatalf("HTTP status %d", recorder.Code)
			}
			var body struct {
				Data map[string]json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			for key, want := range map[string]string{"payable_amount": tc.payable, "currency": tc.currency, "fee_amount": "2.00", "fee_policy": "customer_surcharge", "online_pay_amount": "60.00", "wallet_paid_amount": "40.00"} {
				var got string
				if err := json.Unmarshal(body.Data[key], &got); err != nil || got != want {
					t.Fatalf("%s: got %s, want %q", key, body.Data[key], want)
				}
			}
		})
	}
}

type amountTestOrderCreator struct{ order *orderdomain.Order }

func (s amountTestOrderCreator) CreateOrder(orderhttp.CreateOrderInput) (*orderdomain.Order, error) {
	return s.order, nil
}
func (s amountTestOrderCreator) CreateGuestOrder(orderhttp.CreateGuestOrderInput) (*orderdomain.Order, error) {
	return s.order, nil
}
