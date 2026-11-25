package userhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"time"

	resellerdomain "github.com/dujiao-next/internal/modules/reseller/domain"

	resellermodule "github.com/dujiao-next/internal/modules/reseller/application"
	resellercontract "github.com/dujiao-next/internal/modules/reseller/contract"
	dto "github.com/dujiao-next/internal/modules/reseller/transport/http/presenter"
	uploadcontract "github.com/dujiao-next/internal/modules/upload/contract"
	"github.com/dujiao-next/internal/platform/http/ginutil"
	"github.com/dujiao-next/internal/platform/http/response"

	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"strings"
)

type ManagementService interface {
	GetUserManagementSnapshot(userID uint) (*resellerdomain.Profile, []resellerdomain.Domain, bool, error)
	ApplyUserReseller(userID uint, input resellermodule.ResellerApplyInput) (*resellerdomain.Profile, error)
	SubmitUserCustomDomain(userID uint, rawDomain string) (*resellerdomain.Domain, error)
}

type SiteConfigService interface {
	GetUserSiteConfig(userID uint) (*resellerdomain.Profile, *resellerdomain.SiteConfig, bool, error)
	UpdateUserSiteConfig(ctx context.Context, userID uint, input resellermodule.ResellerSiteConfigInput) (*resellerdomain.SiteConfig, error)
}

type UploadService interface {
	SaveFileWithMeta(file *multipart.FileHeader, category string) (*uploadcontract.Result, error)
}

type UserHandler struct {
	frontend   *resellermodule.FrontendService
	management ManagementService
	siteConfig SiteConfigService
	uploads    UploadService
}

func NewUserHandler(management ManagementService, siteConfig SiteConfigService, uploads UploadService) *UserHandler {
	if management == nil || siteConfig == nil || uploads == nil {
		panic("reseller user handler: required dependency is nil")
	}
	return &UserHandler{management: management, siteConfig: siteConfig, uploads: uploads}
}

type applyRequest struct {
	Reason string `json:"reason"`
}

type customDomainRequest struct {
	Domain string `json:"domain" binding:"required"`
}

type siteConfigRequest struct {
	Theme        *resellermodule.ResellerThemeInput       `json:"theme"`
	SiteName     string                                   `json:"site_name"`
	Logo         string                                   `json:"logo"`
	Favicon      string                                   `json:"favicon"`
	Announcement resellermodule.ResellerAnnouncementInput `json:"announcement"`
	Support      resellermodule.ResellerSupportInput      `json:"support"`
	SEO          resellermodule.ResellerSEOInput          `json:"seo"`
	FooterLinks  []resellermodule.ResellerFooterLinkInput `json:"footer_links"`
	NavConfig    resellermodule.ResellerNavConfigInput    `json:"nav_config"`
}

func (req siteConfigRequest) toInput() resellermodule.ResellerSiteConfigInput {
	return resellermodule.ResellerSiteConfigInput{
		Theme:        req.Theme,
		SiteName:     req.SiteName,
		Logo:         req.Logo,
		Favicon:      req.Favicon,
		Announcement: req.Announcement,
		Support:      req.Support,
		SEO:          req.SEO,
		FooterLinks:  req.FooterLinks,
		NavConfig:    req.NavConfig,
	}
}

// GetManagementSnapshot 获取当前用户的分销商准入与域名状态。
func (h *UserHandler) GetManagementSnapshot(c *gin.Context) {
	uid, ok := ginutil.GetUserID(c)
	if !ok {
		return
	}
	profile, domains, canApply, err := h.management.GetUserManagementSnapshot(uid)
	if err != nil {
		respondUserManagementError(c, err, "error.user_fetch_failed")
		return
	}
	response.Success(c, dto.NewResellerManagementSnapshotResp(profile, domains, canApply))
}

// ApplyProfile 提交当前用户的分销商申请。
func (h *UserHandler) ApplyProfile(c *gin.Context) {
	uid, ok := ginutil.GetUserID(c)
	if !ok {
		return
	}
	var req applyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ginutil.RespondBindError(c, err)
		return
	}
	profile, err := h.management.ApplyUserReseller(uid, resellermodule.ResellerApplyInput{Reason: req.Reason})
	if err != nil {
		respondUserManagementError(c, err, "error.save_failed")
		return
	}
	response.Success(c, dto.NewResellerManagementProfileResp(profile))
}

// ListDomains 查询当前用户的分销域名。
func (h *UserHandler) ListDomains(c *gin.Context) {
	uid, ok := ginutil.GetUserID(c)
	if !ok {
		return
	}
	profile, domains, _, err := h.management.GetUserManagementSnapshot(uid)
	if err != nil {
		respondUserManagementError(c, err, "error.user_fetch_failed")
		return
	}
	if profile == nil {
		respondUserManagementError(c, resellercontract.ErrNotOpened, "error.user_fetch_failed")
		return
	}
	response.Success(c, dto.NewResellerDomainRespList(domains))
}

// SubmitCustomDomain 提交当前用户的自定义分销域名。
func (h *UserHandler) SubmitCustomDomain(c *gin.Context) {
	uid, ok := ginutil.GetUserID(c)
	if !ok {
		return
	}
	var req customDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ginutil.RespondBindError(c, err)
		return
	}
	row, err := h.management.SubmitUserCustomDomain(uid, req.Domain)
	if err != nil {
		respondUserManagementError(c, err, "error.save_failed")
		return
	}
	response.Success(c, dto.NewResellerDomainResp(row))
}

// GetSiteConfig 获取当前用户的分销站点配置。
func (h *UserHandler) GetSiteConfig(c *gin.Context) {
	uid, ok := ginutil.GetUserID(c)
	if !ok {
		return
	}
	profile, row, canEdit, err := h.siteConfig.GetUserSiteConfig(uid)
	if err != nil {
		respondUserManagementError(c, err, "error.user_fetch_failed")
		return
	}
	response.Success(c, dto.NewResellerSiteConfigSnapshotResp(profile, row, canEdit))
}

// UpdateSiteConfig 更新当前用户的分销站点配置。
func (h *UserHandler) UpdateSiteConfig(c *gin.Context) {
	uid, ok := ginutil.GetUserID(c)
	if !ok {
		return
	}
	var req siteConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ginutil.RespondBindError(c, err)
		return
	}
	row, err := h.siteConfig.UpdateUserSiteConfig(c.Request.Context(), uid, req.toInput())
	if err != nil {
		var fieldErr *resellermodule.ResellerSiteConfigFieldError
		if errors.As(err, &fieldErr) {
			ginutil.RespondError(c, response.CodeBadRequest, siteConfigFieldErrorKey(fieldErr.Field), nil)
			return
		}
		respondUserManagementError(c, err, "error.save_failed")
		return
	}
	response.Success(c, dto.NewResellerSiteConfigResp(row))
}

// UploadImage 分销商上传站点图片。
func (h *UserHandler) UploadImage(c *gin.Context) {
	uid, ok := ginutil.GetUserID(c)
	if !ok {
		return
	}
	_, _, canEdit, err := h.siteConfig.GetUserSiteConfig(uid)
	if err != nil {
		respondUserManagementError(c, err, "error.forbidden")
		return
	}
	if !canEdit {
		ginutil.RespondError(c, response.CodeForbidden, "error.forbidden", nil)
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		ginutil.RespondError(c, response.CodeBadRequest, "error.file_missing", nil)
		return
	}
	result, err := h.uploads.SaveFileWithMeta(file, "reseller")
	if err != nil {
		if isUploadValidationError(err) {
			ginutil.RespondErrorWithMsg(c, response.CodeBadRequest, err.Error(), nil)
			return
		}
		ginutil.RespondError(c, response.CodeInternal, "error.upload_failed", err)
		return
	}
	response.Success(c, gin.H{
		"url":      result.URL,
		"filename": result.Filename,
		"size":     result.Size,
	})
}

func (h *UserHandler) SetFrontendService(s *resellermodule.FrontendService) { h.frontend = s }
func frontendError(c *gin.Context, err error) {
	key := "error.frontend_invalid"
	code := response.CodeBadRequest
	if errors.Is(err, resellercontract.ErrNotOpened) || errors.Is(err, resellercontract.ErrProfileInactive) {
		key = "error.forbidden"
		code = response.CodeForbidden
	}
	if errors.Is(err, resellermodule.ErrFrontendStale) {
		key = "error.frontend_stale"
	}
	ginutil.RespondError(c, code, key, nil)
}
func (h *UserHandler) FrontendStatus(c *gin.Context) {
	uid, ok := ginutil.GetUserID(c)
	if !ok {
		return
	}
	if h.frontend == nil {
		c.Status(503)
		return
	}
	out, e := h.frontend.Status(uid)
	if e != nil {
		frontendError(c, e)
		return
	}
	response.Success(c, out)
}
func (h *UserHandler) UploadFrontend(c *gin.Context) {
	uid, ok := ginutil.GetUserID(c)
	if !ok {
		return
	}
	if h.frontend == nil {
		c.Status(503)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, resellermodule.FrontendMaxZIP+(1<<20))
	if e := c.Request.ParseMultipartForm(resellermodule.FrontendMaxZIP + (1 << 20)); e != nil {
		frontendError(c, e)
		return
	}
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	file, e := c.FormFile("file")
	if e != nil || file.Size > resellermodule.FrontendMaxZIP {
		frontendError(c, resellermodule.ErrFrontendInvalid)
		return
	}
	src, e := file.Open()
	if e != nil {
		frontendError(c, e)
		return
	}
	defer src.Close()
	raw, e := io.ReadAll(io.LimitReader(src, resellermodule.FrontendMaxZIP+1))
	if e != nil {
		frontendError(c, e)
		return
	}
	out, e := h.frontend.Upload(uid, raw)
	if e != nil {
		frontendError(c, e)
		return
	}
	response.Success(c, out)
}
func (h *UserHandler) PublishFrontend(c *gin.Context) {
	uid, ok := ginutil.GetUserID(c)
	if !ok {
		return
	}
	if h.frontend == nil {
		c.Status(503)
		return
	}
	var req struct {
		DraftHash string `json:"draft_hash"`
	}
	if e := c.ShouldBindJSON(&req); e != nil {
		ginutil.RespondBindError(c, e)
		return
	}
	if e := h.frontend.Publish(uid, req.DraftHash); e != nil {
		frontendError(c, e)
		return
	}
	response.Success(c, gin.H{"published": true})
}
func (h *UserHandler) PreviewFrontend(c *gin.Context) {
	uid, ok := ginutil.GetUserID(c)
	if !ok {
		return
	}
	if h.frontend == nil {
		c.Status(503)
		return
	}
	out, e := h.frontend.PreviewToken(uid)
	if e != nil {
		frontendError(c, e)
		return
	}
	response.Success(c, out)
}
func (h *UserHandler) DisableFrontend(c *gin.Context) {
	uid, ok := ginutil.GetUserID(c)
	if !ok {
		return
	}
	if h.frontend == nil {
		c.Status(503)
		return
	}
	if e := h.frontend.Disable(uid); e != nil {
		frontendError(c, e)
		return
	}
	response.Success(c, gin.H{"published": false})
}

// TryServeFrontend runs only in the storefront fallback, after API/admin routes.
func (h *UserHandler) TryServeFrontend(c *gin.Context, tenant resellercontract.TenantContext) bool {
	if h.frontend == nil || !tenant.IsReseller() {
		return false
	}
	if c.Request.Method != "GET" && c.Request.Method != "HEAD" {
		return false
	}
	if c.Query("platform_storefront") == "1" {
		return false
	}
	requestPath := c.Request.URL.Path
	token := ""
	if strings.HasPrefix(requestPath, "/__frontend_preview/") {
		parts := strings.SplitN(strings.TrimPrefix(requestPath, "/__frontend_preview/"), "/", 2)
		if len(parts) != 2 {
			c.Status(404)
			return true
		}
		token = parts[0]
		requestPath = "/" + parts[1]
	}
	// A stable escape route always leaves account and reseller management available.
	for _, prefix := range []string{"/reseller", "/me", "/auth", "/guest", "/pay", "/checkout", "/cart", "/orders"} {
		if requestPath == prefix || strings.HasPrefix(requestPath, prefix+"/") {
			return false
		}
	}
	asset, e := h.frontend.Asset(tenant, requestPath, token)
	if e != nil {
		c.Status(404)
		return true
	}
	if asset == nil {
		return false
	}
	if len(asset.Data) == 0 {
		c.Status(404)
		return true
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	if asset.Preview {
		c.Header("Content-Security-Policy", "sandbox allow-scripts; default-src 'self' data: blob:; script-src 'self' 'unsafe-inline' blob:; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https:; connect-src 'none'; form-action 'none'; base-uri 'self'")
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("X-Robots-Tag", "noindex, nofollow")
	} else {
		c.Header("Content-Security-Policy", "frame-ancestors 'self'; object-src 'none'; base-uri 'self'; worker-src 'none'")
	}
	if c.Request.Method == "HEAD" {
		c.Status(200)
	} else {
		c.Data(200, asset.ContentType, asset.Data)
	}
	return true
}

// RedemptionSite exposes no account, credential, order, or customer data.
func (h *UserHandler) RedemptionSite(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	service, ok := h.siteConfig.(interface {
		RedemptionSite(string) (map[string]interface{}, error)
	})
	if !ok {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	site, err := service.RedemptionSite(c.Query("host"))
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	if site == nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": site})
}

// Redeem proxies the fixed customer API; never forwards browser credentials.
func (h *UserHandler) Redeem(c *gin.Context) {
	op := c.Param("operation")
	if op != "verify" && op != "submit" && op != "progress" {
		c.Status(404)
		return
	}
	host := resellercontract.NormalizeHost(c.Request.Host)
	if host != "xuegaogx.cc" {
		lookup, ok := h.siteConfig.(interface {
			RedemptionSite(string) (map[string]interface{}, error)
		})
		if !ok {
			c.Status(403)
			return
		}
		site, err := lookup.RedemptionSite(host)
		if err != nil || site == nil {
			c.Status(403)
			return
		}
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 100000)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil || !json.Valid(body) {
		c.Status(400)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 35*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", "https://redeem.xuegaogx.cc/api/v1/redemptions/"+op, bytes.NewReader(body))
	if err != nil {
		c.Status(502)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://redeem.xuegaogx.cc")
	client := &http.Client{Timeout: 35 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	result, err := client.Do(req)
	if err != nil {
		c.Status(502)
		return
	}
	defer result.Body.Close()
	data, err := io.ReadAll(io.LimitReader(result.Body, 1<<20))
	if err != nil || !json.Valid(data) {
		c.Status(502)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Data(result.StatusCode, "application/json; charset=utf-8", data)
}
