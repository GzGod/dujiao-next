package gmcashier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/dujiao-next/internal/shared/jsonmap"
)

type Client struct{}

var tradePath = regexp.MustCompile(`^/(?:cashier|pay/checkout-counter)/([A-Za-z0-9_-]+)/*$`)

func tradeID(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "pay.xuegaogx.cc" || u.User != nil {
		return "", errors.New("invalid cashier origin")
	}
	m := tradePath.FindStringSubmatch(u.Path)
	if len(m) != 2 {
		return "", errors.New("invalid cashier trade")
	}
	return m[1], nil
}
func request(ctx context.Context, method, path string, body any) (jsonmap.JSON, error) {
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://pay.xuegaogx.cc"+path, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var envelope struct {
		Code int          `json:"status_code"`
		Data jsonmap.JSON `json:"data"`
	}
	err = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&envelope)
	if err != nil || res.StatusCode != 200 || envelope.Code != 200 || envelope.Data == nil {
		return nil, errors.New("cashier request failed")
	}
	return envelope.Data, nil
}
func (Client) Cashier(ctx context.Context, payURL, network, token string) (jsonmap.JSON, error) {
	trade, err := tradeID(payURL)
	if err != nil {
		return nil, err
	}
	config, err := request(ctx, http.MethodGet, "/payments/gmpay/v1/config", nil)
	if err != nil {
		return nil, err
	}
	if network != "" || token != "" {
		token = strings.ToUpper(token)
		network = strings.ToLower(network)
		if token != "USDT" && token != "USDC" {
			return nil, errors.New("unsupported token")
		}
		supported := false
		if assets, ok := config["supported_assets"].([]interface{}); ok {
			for _, a := range assets {
				if row, ok := a.(map[string]interface{}); ok && row["network"] == network {
					if tokens, ok := row["tokens"].([]interface{}); ok {
						for _, t := range tokens {
							if t == token {
								supported = true
							}
						}
					}
				}
			}
		}
		if !supported {
			return nil, errors.New("unsupported network")
		}
		order, err := request(ctx, http.MethodPost, "/pay/switch-network", map[string]string{"trade_id": trade, "network": network, "token": token})
		if err != nil {
			return nil, err
		}
		return jsonmap.JSON{"order": order, "supported_assets": config["supported_assets"]}, nil
	}
	order, err := request(ctx, http.MethodGet, "/pay/checkout-counter-resp/"+url.PathEscape(trade), nil)
	if err != nil {
		return nil, err
	}
	return jsonmap.JSON{"order": order, "supported_assets": config["supported_assets"]}, nil
}

// ConfirmExpired checks the original merchant trade, including the parent of any network selection.
func ConfirmExpired(ctx context.Context, payURL, ref string) (bool, error) {
	trade, err := tradeID(payURL)
	if err != nil || ref != trade {
		return false, errors.New("cashier trade mismatch")
	}
	data, err := request(ctx, http.MethodGet, "/pay/check-status/"+url.PathEscape(trade), nil)
	if err != nil {
		return false, err
	}
	if data["trade_id"] != trade {
		return false, errors.New("cashier status trade mismatch")
	}
	status, ok := data["status"].(float64)
	return ok && status == 3, nil
}
