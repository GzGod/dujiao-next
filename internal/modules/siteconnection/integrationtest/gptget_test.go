package siteconnection_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/crypto"
	siteconnectionapp "github.com/dujiao-next/internal/modules/siteconnection/application"
	siteconnectiondomain "github.com/dujiao-next/internal/modules/siteconnection/domain"
)

func TestGptgetPingExposesAccountAndCapabilities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/open/redemptions/capabilities" {
			io.WriteString(w, `{"ok":true,"data":{"business_operations_enabled":false,"available_operations":["account","products"]}}`)
		} else if r.URL.Path == "/api/v1/open/redemptions/account" {
			io.WriteString(w, `{"ok":true,"data":{"user_id":7,"balance":"19.95","currency":"CNY","funding_state":"uninitialized"}}`)
		} else {
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
	}))
	defer server.Close()
	encrypted, err := crypto.Encrypt(crypto.DeriveKey("app-key"), "upstream-secret")
	if err != nil {
		t.Fatal(err)
	}
	repo := &siteConnectionRepoStub{conn: &siteconnectiondomain.Connection{ID: 1, BaseURL: server.URL, ApiKey: "key", ApiSecret: encrypted, Protocol: constants.ConnectionProtocolGptget, Status: constants.ConnectionStatusActive}}
	service := siteconnectionapp.NewService(repo, "app-key", t.TempDir())
	result, err := service.Ping(1)
	if err != nil || result.Balance != "19.95" || result.FundingState != "uninitialized" || result.BusinessOperationsEnabled == nil || *result.BusinessOperationsEnabled || len(result.AvailableOperations) != 2 {
		t.Fatalf("lost capability/account fields: %+v err=%v", result, err)
	}
}
