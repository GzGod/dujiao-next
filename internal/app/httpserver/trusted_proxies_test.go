package httpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	fulfillmentdomain "github.com/dujiao-next/internal/modules/fulfillment/domain"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	orderstore "github.com/dujiao-next/internal/modules/order/infrastructure/gormstore"
	orderriskdomain "github.com/dujiao-next/internal/modules/orderrisk/domain"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestConfigureTrustedProxiesRejectsSpoofedForwardedIPFromUntrustedPeer(t *testing.T) {
	engine := gin.New()
	if err := configureTrustedProxies(engine, nil); err != nil {
		t.Fatal(err)
	}
	engine.GET("/ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "203.0.113.9:1234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	if resp.Body.String() != "203.0.113.9" {
		t.Fatalf("untrusted peer spoofed client IP: %q", resp.Body.String())
	}
}

func TestConfigureTrustedProxiesAcceptsForwardedIPFromConfiguredProxy(t *testing.T) {
	engine := gin.New()
	if err := configureTrustedProxies(engine, []string{"127.0.0.1/32"}); err != nil {
		t.Fatal(err)
	}
	engine.GET("/ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	if resp.Body.String() != "1.2.3.4" {
		t.Fatalf("trusted proxy client IP=%q, want 1.2.3.4", resp.Body.String())
	}
}

func TestConfigureTrustedProxiesRejectsInvalidCIDR(t *testing.T) {
	if err := configureTrustedProxies(gin.New(), []string{"not-a-cidr"}); err == nil {
		t.Fatal("expected invalid trusted proxy configuration to fail")
	}
}

func TestConfigureTrustedProxiesRejectsTrustAllNetworks(t *testing.T) {
	for _, network := range []string{"0.0.0.0/0", "::/0"} {
		t.Run(network, func(t *testing.T) {
			if err := configureTrustedProxies(gin.New(), []string{network}); err == nil {
				t.Fatalf("expected %s to be rejected", network)
			}
		})
	}
}

type redemptionRecorderProbe struct {
	digests [][sha256.Size]byte
	err     error
}

func (p *redemptionRecorderProbe) RecordRedemptionSubmission(_ context.Context, digest [sha256.Size]byte) error {
	p.digests = append(p.digests, digest)
	return p.err
}

func TestRedemptionSubmissionAuthenticationAndPayload(t *testing.T) {
	const validToken = "test-redemption-sync-token-0123456789"
	t.Setenv("REDEMPTION_RISK_SYNC_TOKEN", validToken)
	probe := &redemptionRecorderProbe{}
	r := gin.New()
	r.POST("/", redemptionSubmissionHandler(probe))
	digest := sha256.Sum256([]byte("TEST-REDEMPTION-CODE-00001"))
	body := `{"code_hash":"` + hex.EncodeToString(digest[:]) + `"}`
	call := func(token, payload string, status int) {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		r.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("HTTP status = %d, want %d", w.Code, status)
		}
		if w.Body.Len() != 0 {
			t.Fatal("sync handler returned an unexpected response body")
		}
	}
	call("", body, http.StatusUnauthorized)
	call("invalid", body, http.StatusUnauthorized)
	call(validToken, body+"{}", http.StatusBadRequest)
	call(validToken, `{"code_hash":"invalid"}`, http.StatusBadRequest)
	call(validToken, `{"code_hash":"`+hex.EncodeToString(digest[:])+`","session":"unexpected"}`, http.StatusBadRequest)
	if len(probe.digests) != 0 {
		t.Fatal("an unauthorized or invalid request reached storage")
	}
	probe.err = errors.New("temporary storage error")
	call(validToken, body, http.StatusServiceUnavailable)
	probe.err = nil
	call(validToken, body, http.StatusNoContent)
	call(validToken, body, http.StatusNoContent)
	if len(probe.digests) != 3 {
		t.Fatalf("storage received %d requests, want 3", len(probe.digests))
	}
	for _, recorded := range probe.digests {
		if recorded != digest {
			t.Fatal("storage received an unexpected digest")
		}
	}
}

func TestRedemptionEvidenceRejectsNonSuperAdmin(t *testing.T) {
	r := gin.New()
	r.GET("/", redemptionEvidenceListHandler(nil))
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("HTTP status = %d, want 403", recorder.Code)
	}
}

type redemptionLookupProbe struct{ calls int }

func (p *redemptionLookupProbe) RedemptionRiskBlocked(context.Context, [sha256.Size]byte) (bool, error) {
	p.calls++
	return true, nil
}

func TestRedemptionRiskDecisionRequiresServiceCredential(t *testing.T) {
	const token = "test-service-token-stays-server-side"
	t.Setenv("REDEMPTION_RISK_SYNC_TOKEN", token)
	probe := &redemptionLookupProbe{}
	r := gin.New()
	r.POST("/", redemptionRiskHandler(probe))
	for i, auth := range []string{"", "Bearer invalid", "Bearer " + token} {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"code_hash":"`+strings.Repeat("a", 64)+`"}`))
		req.Header.Set("Authorization", auth)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		want := http.StatusUnauthorized
		if i == 2 {
			want = http.StatusOK
		}
		if w.Code != want || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("status=%d want=%d", w.Code, want)
		}
	}
	if probe.calls != 1 {
		t.Fatalf("expected only the authorized lookup, got %d calls", probe.calls)
	}
}

func TestRedemptionRiskHTTPIntegrationWithOrderStore(t *testing.T) {
	const token = "test-service-token-stays-server-side"
	t.Setenv("REDEMPTION_RISK_SYNC_TOKEN", token)
	dsn := fmt.Sprintf("file:redemption_risk_http_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	if err := db.AutoMigrate(&orderdomain.Order{}, &orderdomain.OrderItem{}, &fulfillmentdomain.Fulfillment{}, &orderriskdomain.RedemptionEvidence{}); err != nil {
		t.Fatal(err)
	}
	store := orderstore.New(db, "test-guest-credential-secret-with-32-bytes")
	now := time.Now()
	parent := orderdomain.Order{OrderNo: "HTTP-RISK-PARENT", PaidAt: &now, RiskHoldStatus: "pending_review"}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatal(err)
	}
	child := orderdomain.Order{OrderNo: "HTTP-RISK-CHILD", ParentID: &parent.ID, PaidAt: &now}
	if err := db.Create(&child).Error; err != nil {
		t.Fatal(err)
	}
	const code = "HTTP-INTEGRATION-CODE-00001"
	delivery := fulfillmentdomain.Fulfillment{OrderID: child.ID, Type: "upstream", Status: "delivered", Payload: code, RedemptionProgress: "unused"}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(code))
	hash := hex.EncodeToString(digest[:])

	router := gin.New()
	router.POST("/risk", redemptionRiskHandler(store))
	router.POST("/risk/submitted", redemptionSubmissionHandler(store))
	router.POST("/risk/evidence", redemptionEvidenceHandler(store))
	router.GET("/admin/evidence", func(c *gin.Context) { c.Set("admin_is_super", true) }, redemptionEvidenceListHandler(store))
	call := func(method, path, body string, expected int) string {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != expected || strings.Contains(w.Body.String(), code) {
			t.Fatalf("unexpected response for %s: HTTP %d %s", path, w.Code, w.Body.String())
		}
		return w.Body.String()
	}

	digestBody := `{"code_hash":"` + hash + `"}`
	if body := call(http.MethodPost, "/risk", digestBody, http.StatusOK); body != `{"blocked":true}` {
		t.Fatalf("pending order must block: %s", body)
	}
	evidence := `{"event_id":"ed942bf7-e555-4f04-946b-48a75077bba0","operation":"submit","outcome":"blocked","code_hash":"` + hash +
		`","client_ip":"192.0.2.1","user_agent":"fixture","target_email":"target@example.test"}`
	call(http.MethodPost, "/risk/evidence", evidence, http.StatusNoContent)
	call(http.MethodPost, "/risk/evidence", evidence, http.StatusNoContent)
	body := call(http.MethodGet, "/admin/evidence?order_no="+parent.OrderNo, "", http.StatusOK)
	var list struct {
		Data []orderriskdomain.RedemptionEvidence `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil || len(list.Data) != 1 ||
		list.Data[0].Outcome != "blocked" || list.Data[0].TargetEmail != "target@example.test" {
		t.Fatalf("parent order evidence missing: %s / %v", body, err)
	}

	if err := db.Model(&parent).UpdateColumn("risk_hold_status", "released").Error; err != nil {
		t.Fatal(err)
	}
	if body := call(http.MethodPost, "/risk", digestBody, http.StatusOK); body != `{"blocked":false}` {
		t.Fatalf("released order must allow: %s", body)
	}
	call(http.MethodPost, "/risk/submitted", digestBody, http.StatusNoContent)
	unused, err := store.HasUnusedRedemption([]uint{parent.ID})
	if err != nil || unused {
		t.Fatalf("submission must retire the unused signal: %v / %v", unused, err)
	}
	if err := db.Model(&child).UpdateColumn("risk_hold_status", "rejected").Error; err != nil {
		t.Fatal(err)
	}
	if body := call(http.MethodPost, "/risk", digestBody, http.StatusOK); body != `{"blocked":true}` {
		t.Fatalf("rejected child order must block: %s", body)
	}
}
