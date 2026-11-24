package middleware

import (
	"strings"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/i18n"
	usercontract "github.com/dujiao-next/internal/modules/identity/user/contract"
	"github.com/dujiao-next/internal/platform/http/response"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// OptionalUserJWTAuthMiddleware permits anonymous browsing, but never treats an
// invalid, revoked or cross-tenant token as an authenticated viewer.
func OptionalUserJWTAuthMiddleware(secret string, users usercontract.Store) gin.HandlerFunc {
	authenticate := UserJWTAuthMiddleware(secret, users)
	return func(c *gin.Context) {
		if strings.TrimSpace(c.GetHeader("Authorization")) == "" {
			c.Next()
			return
		}
		authenticate(c)
	}
}

// CatalogMemberLevelMiddleware runs after optional authentication. Read current
// membership from storage so a downgrade cannot retain access via cached claims.
func CatalogMemberLevelMiddleware(users usercontract.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Writer.Header().Add("Vary", "Authorization")
		if id := c.GetUint("user_id"); id > 0 {
			user, err := users.GetByID(id)
			if err != nil || user == nil || user.Status != constants.UserStatusActive {
				response.Unauthorized(c, i18n.T(i18n.ResolveLocale(c), "error.token_invalid"))
				c.Abort()
				return
			}
			c.Set("catalog_member_level_id", user.MemberLevelID)
		}
		c.Next()
	}
}

// CatalogReadRateLimitMiddleware keeps browsing separate from payment and
// procurement traffic, using the shared Redis limiter with local fallback.
func CatalogReadRateLimitMiddleware(client *redis.Client, prefix string) gin.HandlerFunc {
	return RateLimitMiddleware(client, RateLimitRule{
		Prefix: prefix + ":catalog", WindowSeconds: 60, MaxRequests: 120,
		BlockSeconds: 60, MessageKey: "error.rate_limited",
	}, KeyByIP)
}
