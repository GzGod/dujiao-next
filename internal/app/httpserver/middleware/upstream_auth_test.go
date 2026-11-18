package middleware

import (
	"bytes"
	"fmt"
	"github.com/dujiao-next/internal/constants"
	credentialdomain "github.com/dujiao-next/internal/modules/apicredential/domain"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	"github.com/dujiao-next/internal/upstream"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type headerTestCredentials struct{}

func (headerTestCredentials) GetByApiKey(key string) (*credentialdomain.ApiCredential, error) {
	if key != "test-key" {
		return nil, nil
	}
	now := time.Now()
	return &credentialdomain.ApiCredential{ID: 1, UserID: 2, ApiSecret: "test-secret", IsActive: true, Status: constants.ApiCredentialStatusApproved, LastUsedAt: &now, User: &userdomain.User{Status: constants.UserStatusActive}}, nil
}
func (headerTestCredentials) TouchLastUsedAt(uint, time.Time) error { return nil }

func TestUpstreamAuthBrandedAndLegacy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, branded := range []bool{false, true} {
		for _, valid := range []bool{false, true} {
			t.Run(fmt.Sprintf("branded=%v/valid=%v", branded, valid), func(t *testing.T) {
				r := gin.New()
				r.Use(UpstreamAPIAuthMiddleware(headerTestCredentials{}))
				r.POST("/api/v1/upstream/ping", func(c *gin.Context) { c.Status(204) })
				body := []byte(`{}`)
				ts := time.Now().Unix()
				sig := upstream.Sign("test-secret", "POST", "/api/v1/upstream/ping", ts, body)
				if !valid {
					sig = "bad-signature"
				}
				req := httptest.NewRequest("POST", "/api/v1/upstream/ping", bytes.NewReader(body))
				names := []string{upstream.HeaderApiKey, upstream.HeaderTimestamp, upstream.HeaderSignature}
				if branded {
					names = []string{upstream.HeaderXuegaoAPIKey, upstream.HeaderXuegaoTimestamp, upstream.HeaderXuegaoSignature}
				}
				req.Header.Set(names[0], "test-key")
				req.Header.Set(names[1], fmt.Sprint(ts))
				req.Header.Set(names[2], sig)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				want := http.StatusUnauthorized
				if valid {
					want = 204
				}
				if w.Code != want {
					t.Fatalf("got %d want %d", w.Code, want)
				}
			})
		}
	}
}
