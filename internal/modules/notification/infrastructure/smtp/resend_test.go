package smtp

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/shared/mailbrand"
)

func TestResendHostUsesHTTPSAPI(t *testing.T) {
	var got resendEmailRequest
	var auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"id":"email_1"}`))
	}))
	defer server.Close()
	old := resendAPIURL
	resendAPIURL = server.URL
	defer func() { resendAPIURL = old }()

	svc := New(&config.EmailConfig{Enabled: true, Host: "smtp.resend.com", Port: 465, Username: "resend", Password: "re_test", From: "noreply@example.com", FromName: "Shop"})
	if err := svc.SendVerifyCode("buyer@example.com", "123456", "register", "zh-CN", mailbrand.Brand{}); err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if auth != "Bearer re_test" {
		t.Fatalf("api key not sent as bearer token: %q", auth)
	}
	if len(got.To) != 1 || got.To[0] != "buyer@example.com" || got.Subject == "" || got.From == "" {
		t.Fatalf("unexpected payload: %+v", got)
	}
}

func TestResendAttachmentIsBase64(t *testing.T) {
	var got resendEmailRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"id":"email_2"}`))
	}))
	defer server.Close()
	old := resendAPIURL
	resendAPIURL = server.URL
	defer func() { resendAPIURL = old }()

	svc := New(&config.EmailConfig{Enabled: true, Host: "smtp.resend.com", Port: 465, Password: "re_test", From: "noreply@example.com"})
	if err := svc.sendEmailWithAttachment("buyer@example.com", "subject", "body", "codes.txt", "AAAA-BBBB"); err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].Filename != "codes.txt" {
		t.Fatalf("attachment missing: %+v", got.Attachments)
	}
	decoded, _ := base64.StdEncoding.DecodeString(got.Attachments[0].Content)
	if string(decoded) != "AAAA-BBBB" {
		t.Fatalf("attachment content mismatch: %q", decoded)
	}
}

func TestResendAPIErrorIsReturned(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"domain not verified"}`))
	}))
	defer server.Close()
	old := resendAPIURL
	resendAPIURL = server.URL
	defer func() { resendAPIURL = old }()

	svc := New(&config.EmailConfig{Enabled: true, Host: "smtp.resend.com", Port: 465, Password: "re_test", From: "noreply@example.com"})
	if err := svc.SendCustomEmail("buyer@example.com", "", ""); err == nil {
		t.Fatal("expected error from resend api")
	}
}

func TestResendFromAddressDecodesDisplayName(t *testing.T) {
	encoded := buildFromAddress("noreply@example.com", "雪糕AI")
	if got := resendFromAddress(encoded); got != "雪糕AI <noreply@example.com>" {
		t.Fatalf("unexpected from: %q (encoded %q)", got, encoded)
	}
	if got := resendFromAddress("noreply@example.com"); got != "noreply@example.com" {
		t.Fatalf("bare address changed: %q", got)
	}
}
