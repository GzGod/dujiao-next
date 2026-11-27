package middleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/dujiao-next/internal/i18n"
	"github.com/dujiao-next/internal/logger"
	"github.com/dujiao-next/internal/platform/http/ginutil"
	"github.com/dujiao-next/internal/platform/http/response"
	"github.com/dujiao-next/internal/upstream"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RateLimitKeyFunc 生成限流 key 的函数
type RateLimitKeyFunc func(*gin.Context) string

// RateLimitRule 限流规则
type RateLimitRule struct {
	Prefix        string
	WindowSeconds int
	MaxRequests   int
	BlockSeconds  int
	MessageKey    string
}

var rateLimitScript = redis.NewScript(`
local current = redis.call("INCR", KEYS[1])
if current == 1 then
	redis.call("EXPIRE", KEYS[1], ARGV[1])
end
if tonumber(ARGV[2]) > 0 and tonumber(ARGV[3]) > 0 and current == tonumber(ARGV[2]) + 1 then
	redis.call("EXPIRE", KEYS[1], ARGV[3])
end
local ttl = redis.call("TTL", KEYS[1])
return {current, ttl}
`)

type localRateLimitEntry struct {
	count     int64
	expiresAt time.Time
}

type localRateLimiter struct {
	mu                      sync.Mutex
	entries                 map[string]localRateLimitEntry
	lastCapacityWarningAt   time.Time
	lastRedisFallbackWarnAt time.Time
}

const localRateLimitMaxEntries = 10000
const localRateLimitWarningInterval = time.Minute

func (l *localRateLimiter) increment(key string, rule RateLimitRule, now time.Time) (int64, int64, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.entries == nil {
		l.entries = make(map[string]localRateLimitEntry)
	}

	entry, exists := l.entries[key]
	if exists && (entry.expiresAt.IsZero() || !now.Before(entry.expiresAt)) {
		delete(l.entries, key)
		exists = false
	}
	if !exists {
		if len(l.entries) >= localRateLimitMaxEntries {
			for existingKey, existing := range l.entries {
				if existing.expiresAt.IsZero() || !now.Before(existing.expiresAt) {
					delete(l.entries, existingKey)
				}
			}
		}
		if len(l.entries) >= localRateLimitMaxEntries {
			// 不淘汰仍在窗口内的计数器；容量耗尽时仅拒绝新的 key，
			// 防止攻击者用高基数 IP/标识绕过已有 key 的频率限制。
			ttl := int64(rule.WindowSeconds)
			if ttl < 1 {
				ttl = 1
			}
			shouldWarn := l.lastCapacityWarningAt.IsZero() ||
				now.Sub(l.lastCapacityWarningAt) >= localRateLimitWarningInterval
			if shouldWarn {
				l.lastCapacityWarningAt = now
			}
			return int64(rule.MaxRequests) + 1, ttl, shouldWarn
		}
		entry = localRateLimitEntry{expiresAt: now.Add(time.Duration(rule.WindowSeconds) * time.Second)}
	}
	entry.count++
	if entry.count == int64(rule.MaxRequests)+1 && rule.BlockSeconds > 0 {
		entry.expiresAt = now.Add(time.Duration(rule.BlockSeconds) * time.Second)
	}
	l.entries[key] = entry
	ttl := int64(entry.expiresAt.Sub(now).Seconds())
	if ttl < 1 {
		ttl = 1
	}
	return entry.count, ttl, false
}

func (l *localRateLimiter) shouldWarnRedisFallback(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.lastRedisFallbackWarnAt.IsZero() &&
		now.Sub(l.lastRedisFallbackWarnAt) < localRateLimitWarningInterval {
		return false
	}
	l.lastRedisFallbackWarnAt = now
	return true
}

// RateLimitMiddleware Redis 频率限制中间件。
// Redis 未配置或运行时暂时不可用时使用进程内兜底；该兜底仅对当前实例生效，
// 多实例部署必须配置共享 Redis 才能获得全局一致的计数。Redis 故障降级和
// 本地容量耗尽都会输出节流后的 warning，避免故障期间产生日志洪泛。
func RateLimitMiddleware(client *redis.Client, rule RateLimitRule, keyFunc RateLimitKeyFunc) gin.HandlerFunc {
	return rateLimitWithFallback(client, rule, keyFunc, &localRateLimiter{})
}

func rateLimitWithFallback(client *redis.Client, rule RateLimitRule, keyFunc RateLimitKeyFunc, local *localRateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if rule.WindowSeconds <= 0 || rule.MaxRequests <= 0 {
			c.Next()
			return
		}

		key := ""
		if keyFunc != nil {
			key = strings.TrimSpace(keyFunc(c))
		}
		if key == "" {
			key = c.ClientIP()
		}
		if rule.Prefix != "" {
			key = fmt.Sprintf("%s:%s", rule.Prefix, key)
		}

		var count, ttlSeconds int64
		now := time.Now()
		incrementLocal := func() {
			var capacityWarning bool
			count, ttlSeconds, capacityWarning = local.increment(key, rule, now)
			if capacityWarning {
				logger.Warnw(
					"rate_limit_local_capacity_exhausted",
					"prefix", rule.Prefix,
					"max_entries", localRateLimitMaxEntries,
					"window_seconds", rule.WindowSeconds,
				)
			}
		}
		if client == nil {
			incrementLocal()
		} else {
			result, err := rateLimitScript.Run(
				c.Request.Context(),
				client,
				[]string{key},
				rule.WindowSeconds,
				rule.MaxRequests,
				rule.BlockSeconds,
			).Result()
			if err != nil {
				if local.shouldWarnRedisFallback(now) {
					logger.Warnw("rate_limit_redis_fallback", "prefix", rule.Prefix, "error", err)
				}
				incrementLocal()
			} else {
				values, ok := result.([]interface{})
				if !ok || len(values) < 2 {
					if local.shouldWarnRedisFallback(now) {
						logger.Warnw(
							"rate_limit_redis_fallback",
							"prefix", rule.Prefix,
							"error", fmt.Sprintf("unexpected result shape %T", result),
						)
					}
					incrementLocal()
				} else {
					count, ok = toInt64(values[0])
					if !ok {
						if local.shouldWarnRedisFallback(now) {
							logger.Warnw(
								"rate_limit_redis_fallback",
								"prefix", rule.Prefix,
								"error", fmt.Sprintf("invalid count type %T", values[0]),
							)
						}
						incrementLocal()
					} else {
						ttlSeconds, _ = toInt64(values[1])
					}
				}
			}
		}
		if count > int64(rule.MaxRequests) {
			waitSeconds := int(ttlSeconds)
			if waitSeconds < 1 {
				waitSeconds = rule.WindowSeconds
			}
			if waitSeconds < 1 {
				waitSeconds = 1
			}
			msgKey := strings.TrimSpace(rule.MessageKey)
			if msgKey == "" {
				msgKey = "error.rate_limited"
			}
			msg := i18n.Sprintf(i18n.ResolveLocale(c), msgKey, waitSeconds)
			if isChannelAPIRequest(c) {
				response.ChannelError(c, 429, response.CodeTooManyRequests, msg, "rate_limit_exceeded")
			} else {
				response.ErrorWithHTTPStatus(c, 429, response.CodeTooManyRequests, msg)
			}
			c.Abort()
			return
		}

		c.Next()
	}
}

func isChannelAPIRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	return strings.HasPrefix(c.Request.URL.Path, "/api/v1/channel")
}

// KeyByIP 使用 IP 作为限流 key
func KeyByIP(c *gin.Context) string {
	return c.ClientIP()
}

// KeyByUserIDAndIP isolates authenticated mutation limits by both account and
// source IP. It falls back to IP when authentication context is unavailable.
func KeyByUserIDAndIP(c *gin.Context) string {
	if c == nil {
		return ""
	}
	userID, exists := c.Get("user_id")
	if !exists {
		return c.ClientIP()
	}
	return fmt.Sprintf("%v|%s", userID, c.ClientIP())
}

// KeyByIPAndHeader 以 "IP|header 值" 作为限流 key。
// 限流位于鉴权之前，header 未经校验，因此计数必须同时绑定来源 IP。
func KeyByIPAndHeader(header string) RateLimitKeyFunc {
	return func(c *gin.Context) string {
		value := c.GetHeader(header)
		if len(value) > 128 {
			value = value[:128]
		}
		if value == "" {
			return c.ClientIP()
		}
		return c.ClientIP() + "|" + value
	}
}

// KeyByUpstreamApiKey 使用 "IP|上游 API Key" 作为限流 key
func KeyByUpstreamApiKey(c *gin.Context) string {
	if c.GetHeader(upstream.HeaderXuegaoAPIKey) != "" {
		return KeyByIPAndHeader(upstream.HeaderXuegaoAPIKey)(c)
	}
	return KeyByIPAndHeader(upstream.HeaderApiKey)(c)
}

// KeyByIPAndJSONField 使用 IP + JSON 字段作为限流 key
func KeyByIPAndJSONField(field string) RateLimitKeyFunc {
	return func(c *gin.Context) string {
		value := strings.ToLower(strings.TrimSpace(readJSONField(c, field)))
		if value == "" {
			return c.ClientIP()
		}
		return fmt.Sprintf("%s|%s", value, c.ClientIP())
	}
}

func readJSONField(c *gin.Context, field string) string {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return ""
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return ""
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
	if len(body) == 0 {
		return ""
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	value, ok := payload[field]
	if !ok {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func toInt64(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case int16:
		return int64(v), true
	case int8:
		return int64(v), true
	case uint64:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint8:
		return int64(v), true
	case float64:
		return int64(v), true
	case float32:
		return int64(v), true
	default:
		return 0, false
	}
}

// PaymentCreationLimits shares limits across initial checkout and later payment attempts.
func PaymentCreationLimits(client *redis.Client, member bool, signingKey string) []gin.HandlerFunc {
	return paymentCreationLimits(client, member, signingKey, &localRateLimiter{})
}

// PaymentCreationLimitGroups shares counters across authentication modes,
// including when Redis is temporarily unavailable.
func PaymentCreationLimitGroups(client *redis.Client, signingKey string) (guest, member []gin.HandlerFunc) {
	local := &localRateLimiter{}
	return paymentCreationLimits(client, false, signingKey, local), paymentCreationLimits(client, true, signingKey, local)
}

func paymentCreationLimits(client *redis.Client, member bool, signingKey string, local *localRateLimiter) []gin.HandlerFunc {
	key := RateLimitKeyFunc(KeyByIP)
	scope := "guest"
	if member {
		scope = "member"
		key = func(c *gin.Context) string {
			id, ok := c.Get("user_id")
			if !ok {
				return c.ClientIP()
			}
			return fmt.Sprint(id)
		}
	}
	wrap := func(rule RateLimitRule, key RateLimitKeyFunc) gin.HandlerFunc {
		limit := rateLimitWithFallback(client, rule, key, local)
		return func(c *gin.Context) {
			path := c.FullPath()
			if c.Request.Method == http.MethodPost &&
				(strings.HasSuffix(path, "/payments") || strings.HasSuffix(path, "/orders/create-and-pay") ||
					member && strings.HasSuffix(path, "/wallet/recharge")) {
				limit(c)
			}
		}
	}
	limits := []gin.HandlerFunc{
		wrap(RateLimitRule{Prefix: "pay_create_short_" + scope, WindowSeconds: 900, MaxRequests: 5}, key),
		wrap(RateLimitRule{Prefix: "pay_create_daily_" + scope, WindowSeconds: 86400, MaxRequests: 8}, key),
	}
	if len(signingKey) >= 32 {
		deviceKey := func(c *gin.Context) string { return c.GetString(ginutil.PaymentDeviceContextKey) }
		limits = append(limits, func(c *gin.Context) {
			path := c.FullPath()
			if c.Request.Method == http.MethodPost && (strings.HasSuffix(path, "/orders") ||
				strings.HasSuffix(path, "/orders/create-and-pay") || strings.HasSuffix(path, "/payments") ||
				member && strings.HasSuffix(path, "/wallet/recharge")) {
				if hash := paymentDeviceKey(c, []byte(signingKey), time.Now()); hash != "" {
					c.Set(ginutil.PaymentDeviceContextKey, hash)
				}
			}
		})
		verified := func(rule RateLimitRule) gin.HandlerFunc {
			limit := wrap(rule, deviceKey)
			return func(c *gin.Context) {
				if deviceKey(c) != "" {
					limit(c)
				}
			}
		}
		limits = append(limits,
			verified(RateLimitRule{Prefix: "pay_create_device_short", WindowSeconds: 900, MaxRequests: 5}),
			verified(RateLimitRule{Prefix: "pay_create_device_daily", WindowSeconds: 86400, MaxRequests: 8}))
	}
	fresh := func(limit gin.HandlerFunc) gin.HandlerFunc {
		return func(c *gin.Context) {
			if c.GetString(ginutil.PaymentDeviceContextKey) == "" {
				limit(c)
			}
		}
	}
	subnetLimit := wrap(RateLimitRule{Prefix: "pay_create_guest_subnet", WindowSeconds: 900, MaxRequests: 30}, paymentSubnetKey)
	limits = append(limits,
		fresh(wrap(RateLimitRule{Prefix: "pay_create_fresh_ip", WindowSeconds: 900, MaxRequests: 5}, KeyByIP)),
		fresh(wrap(RateLimitRule{Prefix: "pay_create_fresh_ip_daily", WindowSeconds: 86400, MaxRequests: 8}, KeyByIP)))
	if member {
		limits = append(limits, fresh(subnetLimit))
	} else {
		limits = append(limits, subnetLimit)
	}
	return limits
}

func paymentSubnetKey(c *gin.Context) string {
	ip, err := netip.ParseAddr(c.ClientIP())
	if err != nil {
		return c.ClientIP()
	}
	ip = ip.Unmap()
	bits := 64
	if ip.Is4() {
		bits = 24
	}
	return netip.PrefixFrom(ip, bits).Masked().String()
}

const paymentDeviceCookie = "__Host-aishop-payment-device"

func signPaymentDevice(key, payload []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("aishop/payment-device/v1:"))
	mac.Write(payload)
	return mac.Sum(nil)
}

// paymentDeviceKey returns a verified risk identifier. A fresh cookie does
// not identify a returning device until it is sent back and verified.
func paymentDeviceKey(c *gin.Context, key []byte, now time.Time) string {
	const lifetime = 30 * 24 * time.Hour
	const payloadSize = 24
	if value, err := c.Cookie(paymentDeviceCookie); err == nil && len(value) == 75 {
		raw, err := base64.RawURLEncoding.DecodeString(value)
		if err == nil && len(raw) == payloadSize+sha256.Size {
			payload, signature := raw[:payloadSize], raw[payloadSize:]
			expires := int64(binary.BigEndian.Uint64(payload[16:]))
			if expires > now.Unix() && expires <= now.Add(lifetime).Unix() &&
				hmac.Equal(signature, signPaymentDevice(key, payload)) {
				hash := sha256.Sum256(payload[:16])
				return hex.EncodeToString(hash[:])
			}
		}
	}
	if _, issued := c.Get(paymentDeviceCookie); !issued {
		payload := make([]byte, payloadSize)
		if _, err := rand.Read(payload[:16]); err == nil {
			binary.BigEndian.PutUint64(payload[16:], uint64(now.Add(lifetime).Unix()))
			value := base64.RawURLEncoding.EncodeToString(append(payload, signPaymentDevice(key, payload)...))
			http.SetCookie(c.Writer, &http.Cookie{
				Name: paymentDeviceCookie, Value: value, Path: "/", MaxAge: int(lifetime.Seconds()),
				Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
			})
			c.Set(paymentDeviceCookie, true)
		}
	}
	return ""
}
