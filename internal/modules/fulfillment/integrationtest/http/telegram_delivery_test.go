package fulfillmenthttp_test

import (
	fulfillmenthttp "github.com/dujiao-next/internal/modules/fulfillment/transport/http"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeliveryWebhookRejectsMissingOrForgedSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &fulfillmenthttp.TelegramDeliveryHandler{WebhookSecret: strings.Repeat("x", 32)}
	r := gin.New()
	h.RegisterWebhook(r)
	for _, secret := range []string{"", "bad"} {
		req := httptest.NewRequest("POST", "/telegram-delivery/webhook", strings.NewReader(`{}`))
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 403 {
			t.Fatal(w.Code)
		}
	}
	req := httptest.NewRequest("POST", "/telegram-delivery/webhook", strings.NewReader(`{"message":{"text":"/start d_fixture","chat":{"id":-10,"type":"group"},"from":{"id":42}}}`))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", strings.Repeat("x", 32))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
}
