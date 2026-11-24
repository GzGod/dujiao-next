package middleware

import (
	"encoding/json"
	"github.com/dujiao-next/internal/constants"
	usercontract "github.com/dujiao-next/internal/modules/identity/user/contract"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	userauth "github.com/dujiao-next/internal/modules/identity/userauth/application"
	"github.com/golang-jwt/jwt/v5"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestResolveAllowedOrigin(t *testing.T) {
	got := resolveAllowedOrigin("https://example.com", []string{"*"}, false)
	if got != "*" {
		t.Fatalf("wildcard without credentials should return *, got %s", got)
	}

	got = resolveAllowedOrigin("https://example.com", []string{"*"}, true)
	if got != "https://example.com" {
		t.Fatalf("wildcard with credentials should echo origin, got %s", got)
	}

	got = resolveAllowedOrigin("https://a.example.com", []string{"https://a.example.com", "https://b.example.com"}, false)
	if got != "https://a.example.com" {
		t.Fatalf("allow-list should return matched origin, got %s", got)
	}

	got = resolveAllowedOrigin("https://x.example.com", []string{"https://a.example.com"}, false)
	if got != "" {
		t.Fatalf("unmatched origin should be empty, got %s", got)
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(RequestIDMiddleware())
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"request_id": getRequestID(c)})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(requestIDHeader, "req-123")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status want 200 got %d", w.Code)
	}
	if w.Header().Get(requestIDHeader) != "req-123" {
		t.Fatalf("response request id want req-123 got %s", w.Header().Get(requestIDHeader))
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if resp["request_id"] != "req-123" {
		t.Fatalf("context request id want req-123 got %s", resp["request_id"])
	}

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/ping", nil)
	r.ServeHTTP(w2, req2)
	generated := w2.Header().Get(requestIDHeader)
	if generated == "" {
		t.Fatalf("generated request id should not be empty")
	}
	if resp := strings.TrimSpace(generated); resp == "" {
		t.Fatalf("generated request id should not be blank")
	}
}

func TestJWTAuthMiddlewareMissingSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(JWTAuthMiddleware("", nil))
	r.GET("/admin/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status want 200 got %d", w.Code)
	}
	var resp struct {
		StatusCode int `json:"status_code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("status_code want 401 got %d", resp.StatusCode)
	}
}

type catalogUsers struct {
	usercontract.Store
	level uint
}

func (s *catalogUsers) GetByID(id uint) (*userdomain.User, error) {
	return &userdomain.User{ID: id, Status: constants.UserStatusActive, MemberLevelID: s.level}, nil
}
func TestCatalogViewerUsesAuthenticatedCurrentMembership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	users := &catalogUsers{level: 2}
	secret := "catalog-test-only"
	claims := userauth.UserJWTClaims{UserID: 917231, RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)), IssuedAt: jwt.NewNumericDate(time.Now())}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(OptionalUserJWTAuthMiddleware(secret, users), CatalogMemberLevelMiddleware(users))
	r.GET("/products", func(c *gin.Context) { c.JSON(200, gin.H{"level": c.GetUint("catalog_member_level_id")}) })
	for _, tc := range []struct {
		auth     string
		level    uint
		rejected bool
	}{{"", 0, false}, {"Bearer invalid", 0, true}, {"Bearer " + token, 2, false}} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/products?member_level_id=999", nil)
		req.Header.Set("Authorization", tc.auth)
		req.Header.Set("X-Member-Level", "999")
		r.ServeHTTP(w, req)
		var body map[string]any
		json.Unmarshal(w.Body.Bytes(), &body)
		if tc.rejected {
			if body["status_code"] != float64(401) {
				t.Fatalf("invalid token accepted: %s", w.Body.String())
			}
			continue
		}
		if body["level"] != float64(tc.level) || w.Header().Get("Cache-Control") != "private, no-store" || w.Header().Get("Vary") != "Authorization" {
			t.Fatalf("wrong privacy response: %s %v", w.Body.String(), w.Header())
		}
	}
	users.level = 1
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/products", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Body.String() != "{\"level\":1}" {
		t.Fatalf("downgrade retained previous pricing: %s", w.Body.String())
	}
}
