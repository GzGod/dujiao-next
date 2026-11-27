package httpserver

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"github.com/dujiao-next/internal/app/container"
	"github.com/dujiao-next/internal/app/httpserver/middleware"
	affiliatebootstrap "github.com/dujiao-next/internal/bootstrap/affiliate"
	fulfillmentwiring "github.com/dujiao-next/internal/bootstrap/fulfillment"
	"github.com/dujiao-next/internal/config"
	affiliatetransport "github.com/dujiao-next/internal/modules/affiliate/transport/http"
	apicredentialtransport "github.com/dujiao-next/internal/modules/apicredential/transport/http"
	auditlogtransport "github.com/dujiao-next/internal/modules/auditlog/transport/http"
	captchatransport "github.com/dujiao-next/internal/modules/captcha/transport/http"
	carttransport "github.com/dujiao-next/internal/modules/cart/transport/http"
	categoryhttp "github.com/dujiao-next/internal/modules/catalog/category/transport/http"
	producthttp "github.com/dujiao-next/internal/modules/catalog/product/transport/http"
	contenttransport "github.com/dujiao-next/internal/modules/content/transport/http"
	giftcardtransport "github.com/dujiao-next/internal/modules/giftcard/transport/http"
	userauthtransport "github.com/dujiao-next/internal/modules/identity/userauth/transport/http"
	memberleveltransport "github.com/dujiao-next/internal/modules/memberlevel/transport/http"
	ordertransport "github.com/dujiao-next/internal/modules/order/transport/http"
	orderriskdomain "github.com/dujiao-next/internal/modules/orderrisk/domain"
	paymenttransport "github.com/dujiao-next/internal/modules/payment/transport/http"
	paymentcallbacktransport "github.com/dujiao-next/internal/modules/payment/transport/http/callback"
	resellertransport "github.com/dujiao-next/internal/modules/reseller/transport/http/user"
	publicconfigtransport "github.com/dujiao-next/internal/modules/settings/transport/http/public"
	wallettransport "github.com/dujiao-next/internal/modules/wallet/transport/http"
	"io"
	"net/http"
	"net/mail"
	"net/netip"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func registerStorefrontRoutes(
	apiV1 *gin.RouterGroup,
	cfg *config.Config,
	c *container.Container,
	publicContentHandler *contenttransport.PublicHandler,
	publicCatalogHandler *producthttp.PublicHandler,
	publicCategoryHandler *categoryhttp.PublicHandler,
	userResellerHandler *resellertransport.UserHandler,
	userResellerProductSettingHandler *resellertransport.UserProductSettingHandler,
	userResellerFinanceHandler *resellertransport.UserFinanceHandler,
	userResellerOrderHandler *resellertransport.UserOrderHandler,
	userApiCredentialHandler *apicredentialtransport.UserHandler,
	userAuditLogHandler *auditlogtransport.UserHandler,
	userGiftCardHandler *giftcardtransport.UserHandler,
	publicMemberLevelHandler *memberleveltransport.PublicHandler,
	userProfileHandler *userauthtransport.UserProfileHandler,
	userEmailHandler *userauthtransport.UserEmailHandler,
	userPasswordHandler *userauthtransport.UserPasswordHandler,
	userVerifyHandler *userauthtransport.UserVerifyHandler,
	userTelegramOIDCHandler *userauthtransport.UserTelegramOIDCHandler,
	userTelegramHandler *userauthtransport.UserTelegramHandler,
	userGoogleHandler *userauthtransport.UserGoogleHandler,
	userLoginHandler *userauthtransport.UserLoginHandler,
	user2FAHandler *userauthtransport.User2FAHandler,
	userInviteHandler *userauthtransport.UserInviteHandler,
	publicConfigHandler *publicconfigtransport.Handler,
	userCartHandler *carttransport.UserHandler,
	userOrderHandler *ordertransport.UserHandler,
	guestOrderHandler *ordertransport.GuestHandler,
	orderPreviewHandler *ordertransport.PreviewHandler,
	orderCreateHandler *ordertransport.CreateHandler,
	paymentLatestHandler *paymenttransport.LatestHandler,
	paymentWriteHandler *paymenttransport.WriteHandler,
	userWalletHandler *wallettransport.UserHandler,
	redisClient *redis.Client,
	loginRule middleware.RateLimitRule,
	guestReadRule middleware.RateLimitRule,
	guestWriteRule middleware.RateLimitRule,
	giftCardRedeemRule middleware.RateLimitRule,
) {
	storefront := apiV1.Group("")
	storefront.Use(middleware.ResellerTenantMiddleware(c.ResellerDomainResolver))
	affiliateHandler := affiliatebootstrap.NewStorefrontHandler(c)

	// 公开接口
	public := storefront.Group("/public")
	{
		publicconfigtransport.RegisterPublicRoutes(public, publicConfigHandler)
		public.GET("/reseller-redemption", userResellerHandler.RedemptionSite)
		public.POST("/redemptions/:operation", middleware.CatalogReadRateLimitMiddleware(redisClient, guestReadRule.Prefix), func(ctx *gin.Context) {
			lookup, ok := c.OrderStore.(redemptionRiskLookup)
			if !ok {
				ctx.Status(http.StatusServiceUnavailable)
				return
			}
			userResellerHandler.Redeem(ctx, func(checkCtx context.Context, digest [sha256.Size]byte, email string) (bool, error) {
				blocked, err := lookup.RedemptionRiskBlocked(checkCtx, digest)
				if blocked && err == nil {
					if recorder, ok := c.OrderStore.(redemptionEvidenceRecorder); ok {
						agent := []rune(ctx.GetHeader("User-Agent"))
						if len(agent) > 512 {
							agent = agent[:512]
						}
						recordErr := recorder.RecordRedemptionEvidence(checkCtx, orderriskdomain.RedemptionEvidence{
							CodeHash: hex.EncodeToString(digest[:]), Operation: "submit", Outcome: "blocked",
							ClientIP: ctx.ClientIP(), UserAgent: string(agent), TargetEmail: email,
						})
						if recordErr != nil {
							_ = ctx.Error(recordErr)
						}
					}
				}
				return blocked, err
			})
		})
		riskAuth := redemptionSyncAuthorization()
		requireRiskService := func(ctx *gin.Context) {
			if !riskAuth(ctx) {
				ctx.Abort()
			}
		}
		riskLimit := func(operation string, capacity int) gin.HandlerFunc {
			return middleware.RateLimitMiddleware(redisClient, middleware.RateLimitRule{
				Prefix: "redemption_risk_" + operation, WindowSeconds: 60, MaxRequests: capacity,
			}, func(*gin.Context) string { return "gateway" })
		}
		public.POST("/redemption-risk-check", requireRiskService, riskLimit("check", 240), redemptionRiskHandler(c.OrderStore))
		public.POST("/redemption-risk-check/submitted", requireRiskService, riskLimit("submitted", 240), redemptionSubmissionHandler(c.OrderStore))
		public.POST("/redemption-risk-check/evidence", requireRiskService, riskLimit("evidence", 480), redemptionEvidenceHandler(c.OrderStore))
		// Catalogue traffic has its own budget; callbacks, login and signed
		// procurement APIs keep their existing independent limits.
		{
			public := public.Group("", middleware.CatalogReadRateLimitMiddleware(redisClient, guestReadRule.Prefix), middleware.OptionalUserJWTAuthMiddleware(cfg.UserJWT.SecretKey, c.UserStore), middleware.CatalogMemberLevelMiddleware(c.UserStore))
			producthttp.RegisterPublicRoutes(public, publicCatalogHandler)
		}
		categoryhttp.RegisterPublicRoutes(public, publicCategoryHandler)
		contenttransport.RegisterPublicRoutes(public, publicContentHandler)
		captchatransport.RegisterPublicRoutes(public, captchatransport.NewPublicHandler(c.CaptchaService))
		affiliatetransport.RegisterPublicRoutes(public, affiliateHandler)
		memberleveltransport.RegisterPublicRoutes(public, publicMemberLevelHandler)
	}

	// 游客接口
	guest := storefront.Group("/guest")
	guestRead := guest.Group("")
	guestRead.Use(middleware.RateLimitMiddleware(redisClient, guestReadRule, middleware.KeyByIP))
	{
		ordertransport.RegisterGuestPreviewRoute(guestRead, orderPreviewHandler)
		ordertransport.RegisterGuestReadRoutes(guestRead, guestOrderHandler)
		paymenttransport.RegisterGuestLatestRoute(guestRead, paymentLatestHandler)
	}
	guestWrite := guest.Group("")
	guestWrite.Use(middleware.RateLimitMiddleware(redisClient, guestWriteRule, middleware.KeyByIP))
	guestPaymentLimits, memberPaymentLimits := middleware.PaymentCreationLimitGroups(redisClient, cfg.UserJWT.SecretKey)
	guestWrite.Use(guestPaymentLimits...)
	{
		fulfillmentwiring.NewTelegramDeliveryHandler(c).RegisterGuest(guestWrite)
		ordertransport.RegisterGuestCreateRoute(guestWrite, orderCreateHandler)
		ordertransport.RegisterGuestCreateAndPayRoute(guestWrite, orderCreateHandler)
		paymenttransport.RegisterGuestWriteRoutes(guestWrite, paymentWriteHandler)
		guestWrite.POST("/payments/gm-cashier", paymentLatestHandler.GuestCashier)
	}

	// 注册 / 忘记密码 / 发送验证码：按 IP 计数，共享出口 IP（NAT）下一次完整流程就要 2~4 次请求，
	// 不能照搬登录的 5 次/封 15 分钟，否则同网用户会被成批误封。
	authRule := middleware.RateLimitRule{
		Prefix:        loginRule.Prefix + ":auth",
		WindowSeconds: 300,
		MaxRequests:   20,
		BlockSeconds:  300,
		MessageKey:    "error.rate_limited",
	}
	authRateLimit := middleware.RateLimitMiddleware(redisClient, authRule, middleware.KeyByIP)

	// 用户认证接口
	auth := storefront.Group("/auth")
	{
		userauthtransport.RegisterUserVerifyAuthRoutes(auth, userVerifyHandler, authRateLimit)
		userauthtransport.RegisterUserRegisterAuthRoutes(auth, userLoginHandler, authRateLimit)
		userauthtransport.RegisterUserLoginAuthRoutes(auth, userLoginHandler, middleware.RateLimitMiddleware(redisClient, loginRule, middleware.KeyByIPAndJSONField("email")))
		userauthtransport.RegisterUser2FAAuthRoutes(auth, user2FAHandler, middleware.RateLimitMiddleware(redisClient, loginRule, middleware.KeyByIP))
		userauthtransport.RegisterUserTelegramAuthRoutes(auth, userTelegramHandler, middleware.RateLimitMiddleware(redisClient, loginRule, middleware.KeyByIP))
		userauthtransport.RegisterUserTelegramOIDCAuthRoutes(auth, userTelegramOIDCHandler, middleware.RateLimitMiddleware(redisClient, loginRule, middleware.KeyByIP))
		userauthtransport.RegisterUserGoogleAuthRoutes(auth, userGoogleHandler, middleware.RateLimitMiddleware(redisClient, loginRule, middleware.KeyByIP))
		userauthtransport.RegisterUserInviteAuthRoutes(auth, userInviteHandler, middleware.RateLimitMiddleware(redisClient, loginRule, middleware.KeyByIP))
		userauthtransport.RegisterUserPasswordAuthRoutes(auth, userPasswordHandler, authRateLimit)
	}

	// 用户接口（需鉴权）
	user := storefront.Group("")
	user.Use(middleware.UserJWTAuthMiddleware(cfg.UserJWT.SecretKey, c.UserStore))
	user.Use(memberPaymentLimits...)
	{
		userauthtransport.RegisterUserProfileRoutes(user, userProfileHandler)
		auditlogtransport.RegisterUserRoutes(user, userAuditLogHandler)
		userauthtransport.RegisterUserPasswordRoutes(user, userPasswordHandler)
		userauthtransport.RegisterUserTelegramRoutes(user, userTelegramHandler)
		userauthtransport.RegisterUserTelegramOIDCRoutes(user, userTelegramOIDCHandler)
		userauthtransport.RegisterUserGoogleRoutes(
			user,
			userGoogleHandler,
			middleware.RateLimitMiddleware(redisClient, loginRule, middleware.KeyByUserIDAndIP),
		)
		userauthtransport.RegisterUserEmailRoutes(user, userEmailHandler)
		userauthtransport.RegisterUser2FARoutes(user, user2FAHandler)
		carttransport.RegisterUserRoutes(user, userCartHandler)
		ordertransport.RegisterUserCreateRoute(user, orderCreateHandler)
		ordertransport.RegisterUserCreateAndPayRoute(user, orderCreateHandler)
		ordertransport.RegisterUserPreviewRoute(user, orderPreviewHandler)
		ordertransport.RegisterUserPaymentChannelsRoute(user, userOrderHandler)
		fulfillmentwiring.NewTelegramDeliveryHandler(c).RegisterUser(user)
		ordertransport.RegisterUserReadRoutes(user, userOrderHandler)
		ordertransport.RegisterUserCancelRoute(user, userOrderHandler)
		paymenttransport.RegisterUserWriteRoutes(user, paymentWriteHandler)
		user.POST("/payments/gm-cashier", paymentLatestHandler.UserCashier)
		paymenttransport.RegisterUserLatestRoute(user, paymentLatestHandler)
		wallettransport.RegisterUserRoutes(user, userWalletHandler)
		giftCardRedeem := user.Group("", middleware.RateLimitMiddleware(redisClient, giftCardRedeemRule, middleware.KeyByUserIDAndIP))
		giftcardtransport.RegisterUserRoutes(giftCardRedeem, userGiftCardHandler)
		affiliatetransport.RegisterUserRoutes(user, affiliateHandler)

		resellerConsole := user.Group("/reseller")
		resellerConsole.Use(middleware.RequireMainTenantForResellerConsole())
		{
			resellertransport.RegisterUserConsoleRoutes(resellerConsole, userResellerHandler)
			resellertransport.RegisterUserProductSettingRoutes(resellerConsole, userResellerProductSettingHandler)
			resellertransport.RegisterUserFinanceRoutes(resellerConsole, userResellerFinanceHandler)
			resellertransport.RegisterUserOrderRoutes(resellerConsole, userResellerOrderHandler)
		}

		// API 对接权限（用户中心）
		apicredentialtransport.RegisterUserRoutes(user, userApiCredentialHandler)
	}
}

func registerPaymentCallbackRoutes(apiV1 *gin.RouterGroup, callbackHandler *paymentcallbacktransport.Handler, webhookHandler *paymenttransport.WebhookHandler, epayRedirectHandler *paymenttransport.EpayRedirectHandler, redisClient *redis.Client, callbackRule middleware.RateLimitRule) {
	callbacks := apiV1.Group("", middleware.RateLimitMiddleware(redisClient, callbackRule, middleware.KeyByIP))
	paymentcallbacktransport.RegisterRoutes(callbacks, callbackHandler)
	paymenttransport.RegisterWebhookRoutes(callbacks, webhookHandler)
	paymenttransport.RegisterEpayRedirectRoute(callbacks, epayRedirectHandler)
}

type redemptionRiskLookup interface {
	RedemptionRiskBlocked(context.Context, [sha256.Size]byte) (bool, error)
}

func redemptionRiskHandler(store any) gin.HandlerFunc {
	lookup, _ := store.(redemptionRiskLookup)
	authorized := redemptionSyncAuthorization()
	return func(c *gin.Context) {
		if !authorized(c) {
			return
		}
		digest, valid := readRedemptionDigest(c)
		if !valid {
			return
		}
		if lookup == nil {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		blocked, err := lookup.RedemptionRiskBlocked(ctx, digest)
		if err != nil {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, gin.H{"blocked": blocked})
	}
}

func readRedemptionDigest(c *gin.Context) (digest [sha256.Size]byte, valid bool) {
	var request struct {
		CodeHash string `json:"code_hash"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
		c.Status(http.StatusBadRequest)
		return digest, false
	}
	if len(request.CodeHash) != hex.EncodedLen(len(digest)) || strings.ToLower(request.CodeHash) != request.CodeHash {
		c.Status(http.StatusBadRequest)
		return digest, false
	}
	if _, err := hex.Decode(digest[:], []byte(request.CodeHash)); err != nil {
		c.Status(http.StatusBadRequest)
		return digest, false
	}
	return digest, true
}

type redemptionSubmissionRecorder interface {
	RecordRedemptionSubmission(context.Context, [sha256.Size]byte) error
}

func redemptionSubmissionHandler(store any) gin.HandlerFunc {
	recorder, _ := store.(redemptionSubmissionRecorder)
	authorized := redemptionSyncAuthorization()
	return func(c *gin.Context) {
		if !authorized(c) {
			return
		}
		if recorder == nil {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		digest, valid := readRedemptionDigest(c)
		if !valid {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := recorder.RecordRedemptionSubmission(ctx, digest); err != nil {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Status(http.StatusNoContent)
	}
}

type redemptionEvidenceRecorder interface {
	RecordRedemptionEvidence(context.Context, orderriskdomain.RedemptionEvidence) error
}

func redemptionEvidenceHandler(store any) gin.HandlerFunc {
	recorder, _ := store.(redemptionEvidenceRecorder)
	authorized := redemptionSyncAuthorization()
	return func(c *gin.Context) {
		if !authorized(c) {
			return
		}
		if recorder == nil {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		var input struct {
			EventID     string `json:"event_id"`
			CodeHash    string `json:"code_hash"`
			Operation   string `json:"operation"`
			Outcome     string `json:"outcome"`
			ClientIP    string `json:"client_ip"`
			UserAgent   string `json:"user_agent"`
			TargetEmail string `json:"target_email"`
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
		decoder := json.NewDecoder(c.Request.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
			c.Status(http.StatusBadRequest)
			return
		}
		digest, err := hex.DecodeString(input.CodeHash)
		_, ipErr := netip.ParseAddr(input.ClientIP)
		validEvent := input.Operation == "verify" && input.Outcome == "attempt" && input.TargetEmail == "" ||
			input.Operation == "submit" && (input.Outcome == "blocked" || input.Outcome == "allowed" || input.Outcome == "unchecked" ||
				input.Outcome == "unavailable" || input.Outcome == "submission_sync_unavailable")
		if err != nil || len(digest) != sha256.Size || input.CodeHash != strings.ToLower(input.CodeHash) ||
			!validEvent || ipErr != nil && input.ClientIP != "unknown" || len(input.ClientIP) > 45 || utf8.RuneCountInString(input.UserAgent) > 512 {
			c.Status(http.StatusBadRequest)
			return
		}
		if input.TargetEmail != "" {
			parsed, err := mail.ParseAddress(input.TargetEmail)
			if err != nil || parsed.Address != input.TargetEmail || len(input.TargetEmail) > 254 {
				c.Status(http.StatusBadRequest)
				return
			}
		}
		var eventID *string
		if input.EventID != "" {
			raw := strings.ReplaceAll(input.EventID, "-", "")
			if _, err := hex.DecodeString(raw); err != nil || len(raw) != 32 || len(input.EventID) != 36 ||
				input.EventID[8] != '-' || input.EventID[13] != '-' || input.EventID[18] != '-' || input.EventID[23] != '-' {
				c.Status(http.StatusBadRequest)
				return
			}
			normalized := strings.ToLower(input.EventID)
			eventID = &normalized
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := recorder.RecordRedemptionEvidence(ctx, orderriskdomain.RedemptionEvidence{
			EventID: eventID, CodeHash: input.CodeHash, Operation: input.Operation, Outcome: input.Outcome,
			ClientIP: input.ClientIP, UserAgent: input.UserAgent, TargetEmail: input.TargetEmail,
		}); err != nil {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Status(http.StatusNoContent)
	}
}

func redemptionSyncAuthorization() func(*gin.Context) bool {
	token := os.Getenv("REDEMPTION_RISK_SYNC_TOKEN")
	configured := len(token) >= 32
	expected := sha256.Sum256([]byte("Bearer " + token))
	return func(c *gin.Context) bool {
		c.Header("Cache-Control", "no-store")
		if !configured {
			c.Status(http.StatusServiceUnavailable)
			return false
		}
		actual := sha256.Sum256([]byte(c.GetHeader("Authorization")))
		if subtle.ConstantTimeCompare(actual[:], expected[:]) != 1 {
			c.Status(http.StatusUnauthorized)
			return false
		}
		return true
	}
}
