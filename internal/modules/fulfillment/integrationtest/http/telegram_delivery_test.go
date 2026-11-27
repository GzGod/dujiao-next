package fulfillmenthttp_test

import (
	"context"
	"encoding/json"
	application "github.com/dujiao-next/internal/modules/fulfillment/application"
	telegramapi "github.com/dujiao-next/internal/modules/fulfillment/infrastructure/telegramapi"
	fulfillmenthttp "github.com/dujiao-next/internal/modules/fulfillment/transport/http"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
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

type botStatusRoundTripper func(*http.Request) (*http.Response, error)

func (f botStatusRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNativeBotStatusRequiresAdminAndRedactsSecrets(t *testing.T) {
	calls := 0
	h := &fulfillmenthttp.TelegramDeliveryHandler{Overview: func(context.Context) map[string]interface{} { calls++; return map[string]interface{}{"enabled": true} }}
	for _, authorized := range []bool{false, true} {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = httptest.NewRequest("GET", "/telegram-delivery/status", nil)
		if authorized {
			ctx.Set("admin_id", uint(1))
		}
		h.BotOverview(ctx)
		if authorized && (w.Code != 200 || calls != 1) {
			t.Fatal("admin status failed")
		}
		if !authorized && calls != 0 {
			t.Fatal("public request reached provider")
		}
	}
	client := &telegramapi.Client{Token: "private-test-token", HTTP: &http.Client{Transport: botStatusRoundTripper(func(r *http.Request) (*http.Response, error) {
		body := `{"ok":true,"result":{"username":"xuegaoaibot","is_bot":true}}`
		if strings.HasSuffix(r.URL.Path, "/getWebhookInfo") {
			body = `{"ok":true,"result":{"url":"https://shop.example/webhook","pending_update_count":0,"last_error_message":"sensitive upstream detail"}}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
	})}}
	status := client.ConnectionStatus(context.Background(), "xuegaoaibot", "https://shop.example/webhook")
	encoded, _ := json.Marshal(status)
	if status["connected"] != true || status["webhook_ok"] != true || strings.Contains(string(encoded), "private-test-token") || strings.Contains(string(encoded), "sensitive") {
		t.Fatal("status response incorrect or sensitive")
	}
	wrong := client.ConnectionStatus(context.Background(), "differentbot", "https://shop.example/webhook")
	if wrong["connected"] != false {
		t.Fatal("accepted wrong bot identity")
	}
}
