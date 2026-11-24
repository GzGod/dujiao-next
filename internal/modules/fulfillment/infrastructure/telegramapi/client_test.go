package telegramapi

import (
	"context"
	"encoding/json"
	contract "github.com/dujiao-next/internal/modules/fulfillment/contract"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestDeliveryMessagePlainTextButtonsAndReceipt(t *testing.T) {
	c := &Client{Token: "fixture", HTTP: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		var p map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&p)
		if p["text"] != "password: <secret>" || p["parse_mode"] != nil || p["protect_content"] != true {
			t.Fatal("changed or exposed payload")
		}
		buttons := p["reply_markup"].(map[string]interface{})["inline_keyboard"].([]interface{})[0].([]interface{})
		if len(buttons) != 2 {
			t.Fatal("missing links")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":{"message_id":123}}`))}, nil
	})}}
	id, err := c.SendDelivery(context.Background(), contract.TelegramMessage{ChatID: "42", Text: "password: <secret>", SupportURL: "https://t.me/support", OrderURL: "https://example.test/orders/1"})
	if err != nil || id != 123 {
		t.Fatal(id, err)
	}
}
