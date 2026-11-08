package smtp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

// Railway 等平台在低价套餐上封锁出站 SMTP 端口（25/465/587/2525），连接会一直挂起到超时。
// 当 SMTP 主机配置为 Resend 时改走 Resend 的 HTTPS API（443 端口），API Key 复用 SMTP 密码字段，
// 后台配置方式不变。

const resendSMTPHost = "smtp.resend.com"

var resendAPIURL = "https://api.resend.com/emails"

var resendHTTPClient = &http.Client{Timeout: 20 * time.Second}

type resendAttachment struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

type resendEmailRequest struct {
	From        string             `json:"from"`
	To          []string           `json:"to"`
	Subject     string             `json:"subject"`
	Text        string             `json:"text"`
	ReplyTo     string             `json:"reply_to,omitempty"`
	Attachments []resendAttachment `json:"attachments,omitempty"`
}

// usesResendAPI 判断是否应通过 Resend HTTPS API 发送。
func (s *Service) usesResendAPI() bool {
	return s.cfg != nil && strings.EqualFold(strings.TrimSpace(s.cfg.Host), resendSMTPHost)
}

// sendViaResendAPI 通过 Resend HTTPS API 发送纯文本邮件（可带一个文本附件）。
func (s *Service) sendViaResendAPI(from, toEmail, subject, body, replyTo, attachName, attachContent string) error {
	apiKey := strings.TrimSpace(s.cfg.Password)
	if apiKey == "" {
		return fmt.Errorf("resend api key is empty")
	}
	payload := resendEmailRequest{
		From:    resendFromAddress(from),
		To:      []string{toEmail},
		Subject: subject,
		Text:    body,
		ReplyTo: normalizeReplyToHeader(replyTo),
	}
	if attachName != "" && attachContent != "" {
		payload.Attachments = []resendAttachment{{
			Filename: attachName,
			Content:  base64.StdEncoding.EncodeToString([]byte(attachContent)),
		}}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, resendAPIURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := resendHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("resend api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
}

// resendFromAddress 把 SMTP 用的 RFC 2047 编码发件人（=?utf-8?q?...?=）还原成明文，
// Resend API 需要 "名称 <地址>" 的原文，否则部分客户端会直接显示编码串。
func resendFromAddress(from string) string {
	addr, err := mail.ParseAddress(from)
	if err != nil || strings.TrimSpace(addr.Name) == "" {
		return from
	}
	name := strings.NewReplacer("<", "", ">", "", "\"", "", "\r", " ", "\n", " ").Replace(addr.Name)
	return fmt.Sprintf("%s <%s>", strings.TrimSpace(name), addr.Address)
}
