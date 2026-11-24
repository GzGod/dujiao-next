package fulfillmenthttp_test

import (
	application "github.com/dujiao-next/internal/modules/fulfillment/application"
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

func TestCustomerCannotInvokeAdministratorButtonsOrSpoofChat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &fulfillmenthttp.TelegramDeliveryHandler{WebhookSecret: strings.Repeat("x", 32), Service: &application.TelegramDeliveryService{AdminChatID: "5760638358"}}
	r := gin.New()
	h.RegisterWebhook(r)
	for _, body := range []string{
		`{"update_id":10,"callback_query":{"data":"deliver:1","from":{"id":42},"message":{"chat":{"id":42,"type":"private"}}}}`,
		`{"update_id":11,"callback_query":{"data":"deliver:1","from":{"id":42},"message":{"chat":{"id":5760638358,"type":"private"}}}}`,
		`{"update_id":12,"callback_query":{"data":"deliver:1","from":{"id":5760638358},"message":{"chat":{"id":-1,"type":"group"}}}}`,
	} {
		req := httptest.NewRequest("POST", "/telegram-delivery/webhook", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", strings.Repeat("x", 32))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
		// No Store or Sender is provided: any unauthorized dispatch would panic.
	}
}
