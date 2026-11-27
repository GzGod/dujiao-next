package middleware

import (
	"context"
	"fmt"
	"github.com/dujiao-next/internal/platform/http/ginutil"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func TestKeyByIPAndJSONField(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth", strings.NewReader(`{"email":" Test@Example.com "}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.RemoteAddr = "1.2.3.4:5678"

	key := KeyByIPAndJSONField("email")(c)
	if key != "test@example.com|1.2.3.4" {
		t.Fatalf("key want test@example.com|1.2.3.4 got %s", key)
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		t.Fatalf("read body after key extraction failed: %v", err)
	}
	if !strings.Contains(string(body), "Test@Example.com") {
		t.Fatalf("request body should be restored after reading field")
	}
}

func TestRateLimitMiddlewareWithoutClient(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(RateLimitMiddleware(nil, RateLimitRule{WindowSeconds: 60, MaxRequests: 1}, KeyByIP))
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status want 200 got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("expected handler response body, got %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/ping", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("second request must be limited even without Redis: status want 429 got %d", w.Code)
	}
}

func TestLocalRateLimiterCapacityDoesNotEvictLiveEntries(t *testing.T) {
	now := time.Now()
	limiter := &localRateLimiter{entries: make(map[string]localRateLimitEntry, localRateLimitMaxEntries)}
	for i := 0; i < localRateLimitMaxEntries; i++ {
		limiter.entries[fmt.Sprintf("live-%d", i)] = localRateLimitEntry{
			count:     1,
			expiresAt: now.Add(10 * time.Minute),
		}
	}

	rule := RateLimitRule{WindowSeconds: 60, MaxRequests: 5}
	count, _, warned := limiter.increment("new-key", rule, now)
	if count <= int64(rule.MaxRequests) {
		t.Fatalf("new key must fail closed when local limiter is at capacity, count=%d", count)
	}
	if !warned {
		t.Fatal("first capacity rejection must request an operator warning")
	}
	_, _, warned = limiter.increment("another-new-key", rule, now.Add(time.Second))
	if warned {
		t.Fatal("capacity warning must be throttled")
	}
	_, _, warned = limiter.increment("later-new-key", rule, now.Add(localRateLimitWarningInterval))
	if !warned {
		t.Fatal("capacity warning should be emitted again after the throttle interval")
	}
	if len(limiter.entries) != localRateLimitMaxEntries {
		t.Fatalf("live entry count changed: got %d want %d", len(limiter.entries), localRateLimitMaxEntries)
	}
	if _, ok := limiter.entries["live-0"]; !ok {
		t.Fatal("capacity handling must not evict live counters")
	}
	if _, ok := limiter.entries["new-key"]; ok {
		t.Fatal("rejected new key must not be stored")
	}
}

func TestLocalRateLimiterPurgesExpiredEntriesBeforeRejectingNewKey(t *testing.T) {
	now := time.Now()
	limiter := &localRateLimiter{entries: make(map[string]localRateLimitEntry, localRateLimitMaxEntries)}
	for i := 0; i < localRateLimitMaxEntries; i++ {
		expiresAt := now.Add(time.Minute)
		if i == 0 {
			expiresAt = now.Add(-time.Second)
		}
		limiter.entries[fmt.Sprintf("entry-%d", i)] = localRateLimitEntry{
			count:     1,
			expiresAt: expiresAt,
		}
	}

	count, _, warned := limiter.increment("new-key", RateLimitRule{WindowSeconds: 60, MaxRequests: 5}, now)
	if count != 1 {
		t.Fatalf("new key count = %d, want 1 after expired entry cleanup", count)
	}
	if warned {
		t.Fatal("successful expired-entry cleanup must not emit a capacity warning")
	}
	if _, ok := limiter.entries["new-key"]; !ok {
		t.Fatal("new key should be stored after expired entry cleanup")
	}
	if _, ok := limiter.entries["entry-0"]; ok {
		t.Fatal("expired entry should be removed")
	}
}

func TestRateLimitMiddlewareFallsBackLocallyWhenRedisIsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve local address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close reserved address: %v", err)
	}
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		DialTimeout:  50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
		WriteTimeout: 50 * time.Millisecond,
		MaxRetries:   -1,
	})
	t.Cleanup(func() { _ = client.Close() })

	r := gin.New()
	r.Use(RateLimitMiddleware(client, RateLimitRule{
		Prefix:        "redis-fallback-test",
		WindowSeconds: 60,
		MaxRequests:   1,
	}, KeyByIP))
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	first := httptest.NewRecorder()
	r.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/ping", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("first fallback request status = %d, want 200", first.Code)
	}

	second := httptest.NewRecorder()
	r.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/ping", nil))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second fallback request status = %d, want 429", second.Code)
	}
}

func TestToInt64(t *testing.T) {
	cases := []struct {
		name  string
		input interface{}
		want  int64
		ok    bool
	}{
		{name: "int64", input: int64(10), want: 10, ok: true},
		{name: "int", input: int(11), want: 11, ok: true},
		{name: "uint8", input: uint8(12), want: 12, ok: true},
		{name: "float64", input: float64(13.9), want: 13, ok: true},
		{name: "string", input: "bad", want: 0, ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := toInt64(tc.input)
			if ok != tc.ok {
				t.Fatalf("ok want %v got %v", tc.ok, ok)
			}
			if got != tc.want {
				t.Fatalf("value want %d got %d", tc.want, got)
			}
		})
	}
}

func TestCatalogLimitSharesListAndDetailButNotOtherTraffic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if err := r.SetTrustedProxies(nil); err != nil {
		t.Fatal(err)
	}
	public := r.Group("/public", CatalogReadRateLimitMiddleware(nil, "catalog-test"))
	public.GET("/products", func(c *gin.Context) { c.Status(204) })
	public.GET("/products/:slug", func(c *gin.Context) { c.Status(204) })
	r.POST("/payments/callback", func(c *gin.Context) { c.Status(204) })
	r.GET("/upstream/products", func(c *gin.Context) { c.Status(204) })
	request := func(method, path, ip string) int {
		req := httptest.NewRequest(method, path, nil)
		req.RemoteAddr = ip + ":1234"
		req.Header.Set("X-Forwarded-For", "8.8.8.8")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	for i := 0; i < 120; i++ {
		if got := request("GET", "/public/products", "203.0.113.1"); got != 204 {
			t.Fatalf("normal browse %d: %d", i, got)
		}
	}
	if got := request("GET", "/public/products/one", "203.0.113.1"); got != 429 {
		t.Fatalf("detail bypassed limit: %d", got)
	}
	if got := request("GET", "/public/products", "203.0.113.2"); got != 204 {
		t.Fatalf("other visitor affected: %d", got)
	}
	if got := request("POST", "/payments/callback", "203.0.113.1"); got != 204 {
		t.Fatalf("callback affected: %d", got)
	}
	if got := request("GET", "/upstream/products", "203.0.113.1"); got != 204 {
		t.Fatalf("procurement affected: %d", got)
	}
}

func TestPaymentCreationSharesLimitWithRetry(t *testing.T) {
	router := gin.New()
	guest := router.Group("/guest", PaymentCreationLimits(nil, false, "")...)
	noContent := func(c *gin.Context) { c.Status(http.StatusNoContent) }
	guest.POST("/orders/create-and-pay", noContent)
	guest.POST("/payments", noContent)
	guest.POST("/payments/:id/capture", noContent)
	post := func(path string, want int) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/guest"+path, nil)
		req.RemoteAddr = "192.0.2.14:1234"
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s got %d, want %d", path, res.Code, want)
		}
	}
	post("/orders/create-and-pay", http.StatusNoContent)
	for i := 0; i < 4; i++ {
		post("/payments", http.StatusNoContent)
	}
	post("/payments", http.StatusTooManyRequests)
	post("/payments/1/capture", http.StatusNoContent)
}

func TestPaymentDeviceCookieRequiresValidSignatureAndExpiry(t *testing.T) {
	now := time.Unix(1800000000, 0)
	key := []byte(strings.Repeat("s", 32))
	read := func(cookie *http.Cookie, at time.Time) (string, []*http.Cookie) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/guest/payments", nil)
		if cookie != nil {
			c.Request.AddCookie(cookie)
		}
		result := paymentDeviceKey(c, key, at)
		return result, w.Result().Cookies()
	}
	first, cookies := read(nil, now)
	if first != "" || len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly {
		t.Fatalf("first request should receive a secure cookie and no device identity")
	}
	valid := cookies[0]
	if id, replacement := read(valid, now); id == "" || len(replacement) != 0 {
		t.Fatal("valid returning device was not accepted")
	}
	tampered := *valid
	if tampered.Value[0] == 'a' {
		tampered.Value = "b" + tampered.Value[1:]
	} else {
		tampered.Value = "a" + tampered.Value[1:]
	}
	if id, replacement := read(&tampered, now); id != "" || len(replacement) != 1 {
		t.Fatal("tampered device was accepted")
	}
	if id, replacement := read(valid, now.Add(31*24*time.Hour)); id != "" || len(replacement) != 1 {
		t.Fatal("expired device was accepted")
	}
}

func TestMemberRechargeSharesPaymentCreationLimit(t *testing.T) {
	router := gin.New()
	member := router.Group("", func(c *gin.Context) { c.Set("user_id", uint(24)) })
	member.Use(PaymentCreationLimits(nil, true, "")...)
	noContent := func(c *gin.Context) { c.Status(http.StatusNoContent) }
	member.POST("/orders/create-and-pay", noContent)
	member.POST("/wallet/recharge", noContent)
	member.POST("/wallet/recharge/payments/:id/capture", noContent)
	post := func(path string, want int) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.RemoteAddr = "192.0.2.24:1234"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s got %d, want %d", path, w.Code, want)
		}
	}
	post("/orders/create-and-pay", http.StatusNoContent)
	for i := 0; i < 4; i++ {
		post("/wallet/recharge", http.StatusNoContent)
	}
	post("/wallet/recharge", http.StatusTooManyRequests)
	post("/wallet/recharge/payments/1/capture", http.StatusNoContent)
}

func TestSignedPaymentDeviceSurvivesGuestToMemberCheckout(t *testing.T) {
	router := gin.New()
	key := strings.Repeat("s", 32)
	guest := router.Group("/guest", PaymentCreationLimits(nil, false, key)...)
	member := router.Group("/member", func(c *gin.Context) { c.Set("user_id", uint(42)) })
	member.Use(PaymentCreationLimits(nil, true, key)...)
	device := func(c *gin.Context) {
		c.Header("X-Test-Device", c.GetString(ginutil.PaymentDeviceContextKey))
		c.Status(http.StatusNoContent)
	}
	guest.POST("/orders", device)
	member.POST("/orders", device)
	call := func(path string, cookie *http.Cookie) *http.Response {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("checkout failed: %d", w.Code)
		}
		return w.Result()
	}
	first := call("/guest/orders", nil)
	if len(first.Cookies()) != 1 || first.Header.Get("X-Test-Device") != "" {
		t.Fatal("first-time checkout must issue a cookie without trusting it yet")
	}
	cookie := first.Cookies()[0]
	guestID := call("/guest/orders", cookie).Header.Get("X-Test-Device")
	memberID := call("/member/orders", cookie).Header.Get("X-Test-Device")
	if len(guestID) != 64 || guestID != memberID {
		t.Fatal("guest and member checkout must identify the same verified device")
	}
}

func TestPaymentDeviceQuotaSharedAcrossGuestAndMemberWithoutRedis(t *testing.T) {
	guestLimits, memberLimits := PaymentCreationLimitGroups(nil, strings.Repeat("s", 32))
	assertPaymentDeviceQuota(t, guestLimits, memberLimits)
}

func TestPaymentDeviceQuotaSharedAcrossInstancesWithRedis(t *testing.T) {
	addr := strings.TrimSpace(os.Getenv("TEST_REDIS_ADDR"))
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR is not configured")
	}
	first := redis.NewClient(&redis.Options{Addr: addr, MaxRetries: -1})
	second := redis.NewClient(&redis.Options{Addr: addr, MaxRetries: -1})
	t.Cleanup(func() { _ = first.Close(); _ = second.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := first.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	key := strings.Repeat("s", 32)
	guestLimits, _ := PaymentCreationLimitGroups(first, key)
	_, memberLimits := PaymentCreationLimitGroups(second, key)
	assertPaymentDeviceQuota(t, guestLimits, memberLimits)
}

func assertPaymentDeviceQuota(t *testing.T, guestLimits, memberLimits []gin.HandlerFunc) {
	t.Helper()
	nonce := time.Now().UnixNano()
	remote := fmt.Sprintf("[2001:db8:%x:%x::1]:1234", (nonce>>16)&0xffff, nonce&0xffff)
	guestRouter, memberRouter := gin.New(), gin.New()
	guest := guestRouter.Group("/guest", guestLimits...)
	member := memberRouter.Group("/member", func(c *gin.Context) { c.Set("user_id", uint(nonce)) })
	member.Use(memberLimits...)
	ok := func(c *gin.Context) { c.Status(http.StatusNoContent) }
	guest.POST("/orders", ok)
	guest.POST("/payments", ok)
	member.POST("/payments", ok)
	call := func(path string, cookie *http.Cookie) *http.Response {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.RemoteAddr = remote
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		if strings.HasPrefix(path, "/guest") {
			guestRouter.ServeHTTP(w, req)
		} else {
			memberRouter.ServeHTTP(w, req)
		}
		return w.Result()
	}
	cookies := call("/guest/orders", nil).Cookies()
	if len(cookies) != 1 {
		t.Fatal("guest checkout did not issue a device cookie")
	}
	for _, path := range []string{"/guest/payments", "/guest/payments", "/member/payments", "/member/payments", "/member/payments"} {
		if got := call(path, cookies[0]).StatusCode; got != http.StatusNoContent {
			t.Fatalf("expected an available checkout attempt, got %d", got)
		}
	}
	if got := call("/member/payments", cookies[0]).StatusCode; got != http.StatusTooManyRequests {
		t.Fatalf("switched checkout should keep the shared device quota, got %d", got)
	}
}
