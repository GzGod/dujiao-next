package telegramapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	contract "github.com/dujiao-next/internal/modules/fulfillment/contract"
	"io"
	"net/http"
	"time"
)

type Client struct {
	Token string
	HTTP  *http.Client
}

func (c *Client) SendDelivery(ctx context.Context, m contract.TelegramMessage) (int64, error) {
	buttons := []map[string]string{}
	if m.SupportURL != "" {
		buttons = append(buttons, map[string]string{"text": "联系客服 / Support", "url": m.SupportURL})
	}
	if m.OrderURL != "" {
		buttons = append(buttons, map[string]string{"text": "查看订单 / Orders", "url": m.OrderURL})
	}
	payload := map[string]interface{}{"chat_id": m.ChatID, "text": m.Text, "protect_content": true, "disable_web_page_preview": true}
	for _, b := range m.Buttons {
		buttons = append(buttons, map[string]string{"text": b.Text, "callback_data": b.Callback})
	}
	if len(buttons) > 0 {
		payload["reply_markup"] = map[string]interface{}{"inline_keyboard": [][]map[string]string{buttons}}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.telegram.org/bot"+c.Token+"/sendMessage", bytes.NewReader(data))
	if err != nil {
		return 0, errors.New("telegram request failed")
	}
	req.Header.Set("Content-Type", "application/json")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return 0, errors.New("telegram send result unknown")
	}
	defer res.Body.Close()
	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			ID int64 `json:"message_id"`
		} `json:"result"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&result) != nil {
		return 0, errors.New("telegram send result unknown")
	}
	if !result.OK && (res.StatusCode == 400 || res.StatusCode == 403 || res.StatusCode == 429) {
		return 0, contract.ErrTelegramRejected
	}
	if !result.OK || result.Result.ID == 0 {
		return 0, errors.New("telegram delivery not confirmed")
	}
	return result.Result.ID, nil
}

func (c *Client) AnswerCallback(ctx context.Context, id string) error {
	body, _ := json.Marshal(map[string]interface{}{"callback_query_id": id})
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.telegram.org/bot"+c.Token+"/answerCallbackQuery", bytes.NewReader(body))
	if err != nil {
		return errors.New("callback acknowledgement failed")
	}
	req.Header.Set("Content-Type", "application/json")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return errors.New("callback acknowledgement failed")
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	return nil
}
