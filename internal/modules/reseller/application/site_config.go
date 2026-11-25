package application

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/mail"
	"net/url"
	"strings"

	resellercontract "github.com/dujiao-next/internal/modules/reseller/contract"

	resellerdomain "github.com/dujiao-next/internal/modules/reseller/domain"

	productcontract "github.com/dujiao-next/internal/modules/catalog/product/contract"

	"archive/zip"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"github.com/dujiao-next/internal/cache"
	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/shared/contactlink"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/mailbrand"
	"io"
	"mime"
	"path"
	"time"
	"unicode/utf8"
)

type LocalizedTextInput map[string]string

type ResellerAnnouncementInput struct {
	Enabled bool               `json:"enabled"`
	Type    string             `json:"type"`
	Title   LocalizedTextInput `json:"title"`
	Content LocalizedTextInput `json:"content"`
}

type ResellerSupportInput struct {
	Telegram   string `json:"telegram"`
	WhatsApp   string `json:"whatsapp"`
	Email      string `json:"email"`
	SupportURL string `json:"support_url"`
}

type ResellerSEOInput struct {
	Title          LocalizedTextInput `json:"title"`
	Keywords       LocalizedTextInput `json:"keywords"`
	Description    LocalizedTextInput `json:"description"`
	DefaultOGImage string             `json:"default_og_image"`
}

type ResellerFooterLinkInput struct {
	Name LocalizedTextInput `json:"name"`
	URL  string             `json:"url"`
}

type ResellerNavConfigInput struct {
	Builtin     map[string]bool           `json:"builtin"`
	CustomItems []ResellerFooterLinkInput `json:"custom_items"`
}

type ResellerThemeInput struct {
	RedemptionTitle string `json:"redemption_title"`
	RedemptionIntro string `json:"redemption_intro"`
	RedemptionStyle string `json:"redemption_style"`
	Template        string `json:"template"`
	PrimaryColor    string `json:"primary_color"`
}

type ResellerSiteConfigInput struct {
	Theme        *ResellerThemeInput       `json:"theme"`
	SiteName     string                    `json:"site_name"`
	Logo         string                    `json:"logo"`
	Favicon      string                    `json:"favicon"`
	Announcement ResellerAnnouncementInput `json:"announcement"`
	Support      ResellerSupportInput      `json:"support"`
	SEO          ResellerSEOInput          `json:"seo"`
	FooterLinks  []ResellerFooterLinkInput `json:"footer_links"`
	NavConfig    ResellerNavConfigInput    `json:"nav_config"`
}

type SiteConfigService struct {
	repo resellercontract.SiteConfigRepository
}

func NewSiteConfigService(repo resellercontract.SiteConfigRepository) *SiteConfigService {
	return &SiteConfigService{repo: repo}
}

func trimLimit(raw string, max int) string {
	value := strings.TrimSpace(raw)
	if max <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) > max {
		return string(runes[:max])
	}
	return value
}

// ResellerSiteConfigFieldError 在站点配置校验失败时携带具体字段，便于前端给出精确提示。
// 通过 Unwrap 兼容既有的 errors.Is(err, resellercontract.ErrSiteConfigInvalid) 判断。
type ResellerSiteConfigFieldError struct {
	Field string
}

func (e *ResellerSiteConfigFieldError) Error() string {
	return "reseller site config field invalid: " + e.Field
}

func (e *ResellerSiteConfigFieldError) Unwrap() error { return resellercontract.ErrSiteConfigInvalid }

func newResellerFieldError(field string) error {
	return &ResellerSiteConfigFieldError{Field: field}
}

func normalizeResellerLocalizedText(raw LocalizedTextInput, max int) jsonmap.JSON {
	out := jsonmap.JSON{}
	for _, lang := range []string{"zh-CN", "zh-TW", "en-US"} {
		out[lang] = trimLimit(raw[lang], max)
	}
	return out
}

func validateHTTPOrUploadPath(raw string) (string, error) {
	value := trimLimit(raw, 500)
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, "/uploads/") {
		return value, nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return "", resellercontract.ErrSiteConfigInvalid
	}
	if parsed.Scheme == "http" || parsed.Scheme == "https" {
		return value, nil
	}
	return "", resellercontract.ErrSiteConfigInvalid
}

func validateSupportURL(raw string) (string, error) {
	value := trimLimit(raw, 500)
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, "tg://") {
		return value, nil
	}
	if strings.HasPrefix(value, "mailto:") {
		addr := strings.TrimPrefix(value, "mailto:")
		if _, err := mail.ParseAddress(addr); err != nil {
			return "", resellercontract.ErrSiteConfigInvalid
		}
		return value, nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.Scheme != "https" {
		return "", resellercontract.ErrSiteConfigInvalid
	}
	return value, nil
}

// NormalizeResellerSupport 归一化并校验客服联系方式。
func NormalizeResellerSupport(input ResellerSupportInput) (jsonmap.JSON, error) {
	telegram := trimLimit(input.Telegram, 500)
	if telegram != "" && !contactlink.IsTelegram(telegram) {
		return nil, newResellerFieldError("support_telegram")
	}
	whatsApp := trimLimit(input.WhatsApp, 500)
	if whatsApp != "" && !contactlink.IsWhatsApp(whatsApp) {
		return nil, newResellerFieldError("support_whatsapp")
	}
	email := trimLimit(input.Email, 320)
	email = strings.TrimPrefix(email, "mailto:")
	if email != "" {
		if _, err := mail.ParseAddress(email); err != nil {
			return nil, newResellerFieldError("support_email")
		}
	}
	supportURL, err := validateSupportURL(input.SupportURL)
	if err != nil {
		return nil, newResellerFieldError("support_url")
	}
	return jsonmap.JSON{"telegram": telegram, "whatsapp": whatsApp, "email": email, "support_url": supportURL}, nil
}

func normalizeResellerAnnouncement(input ResellerAnnouncementInput) jsonmap.JSON {
	typ := trimLimit(input.Type, 32)
	if typ != "info" && typ != "success" && typ != "warning" {
		typ = "info"
	}
	return jsonmap.JSON{
		"enabled": input.Enabled,
		"type":    typ,
		"title":   normalizeResellerLocalizedText(input.Title, 120),
		// 公告内容为富文本 HTML（前端 TipTap），含标签开销，限额需高于纯文本；
		// 按 rune 截断仍可能切断标签，但前端渲染统一经 DOMPurify 解析补全，不会破坏页面。
		"content": normalizeResellerLocalizedText(input.Content, 4000),
	}
}

func normalizeResellerSEO(input ResellerSEOInput) (jsonmap.JSON, error) {
	image, err := validateHTTPOrUploadPath(input.DefaultOGImage)
	if err != nil {
		return nil, newResellerFieldError("image")
	}
	return jsonmap.JSON{
		"title":            normalizeResellerLocalizedText(input.Title, 120),
		"keywords":         normalizeResellerLocalizedText(input.Keywords, 200),
		"description":      normalizeResellerLocalizedText(input.Description, 300),
		"default_og_image": image,
	}, nil
}

func normalizeResellerFooterLinks(input []ResellerFooterLinkInput) (jsonmap.JSON, error) {
	items := make([]jsonmap.JSON, 0, min(len(input), 10))
	for i, item := range input {
		if i >= 10 {
			break
		}
		urlValue, err := validateSupportURL(item.URL)
		if err != nil {
			return nil, newResellerFieldError("link")
		}
		if urlValue == "" {
			continue
		}
		items = append(items, jsonmap.JSON{
			"name": normalizeResellerLocalizedText(item.Name, 80),
			"url":  urlValue,
		})
	}
	return jsonmap.JSON{"items": items}, nil
}

func normalizeResellerNavConfig(input ResellerNavConfigInput) (jsonmap.JSON, error) {
	builtin := jsonmap.JSON{"blog": true, "notice": true, "about": true}
	for _, key := range []string{"blog", "notice", "about"} {
		if value, ok := input.Builtin[key]; ok {
			builtin[key] = value
		}
	}
	custom, err := normalizeResellerFooterLinks(input.CustomItems)
	if err != nil {
		return nil, err
	}
	return jsonmap.JSON{"builtin": builtin, "custom_items": custom["items"]}, nil
}

func (s *SiteConfigService) buildModel(resellerID uint, input ResellerSiteConfigInput) (*resellerdomain.SiteConfig, error) {
	logo, err := validateHTTPOrUploadPath(input.Logo)
	if err != nil {
		return nil, newResellerFieldError("image")
	}
	favicon, err := validateHTTPOrUploadPath(input.Favicon)
	if err != nil {
		return nil, newResellerFieldError("image")
	}
	support, err := NormalizeResellerSupport(input.Support)
	if err != nil {
		return nil, err
	}
	seo, err := normalizeResellerSEO(input.SEO)
	if err != nil {
		return nil, err
	}
	footerLinks, err := normalizeResellerFooterLinks(input.FooterLinks)
	if err != nil {
		return nil, err
	}
	navConfig, err := normalizeResellerNavConfig(input.NavConfig)
	if err != nil {
		return nil, err
	}
	theme := jsonmap.JSON{}
	if input.Theme != nil {
		var err error
		theme, err = normalizeResellerTheme(*input.Theme)
		if err != nil {
			return nil, err
		}
	} else {
		existing, err := s.repo.GetSiteConfigByResellerID(resellerID)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			theme = existing.ThemeJSON
		}
	}
	return &resellerdomain.SiteConfig{
		ResellerID:       resellerID,
		SiteName:         trimLimit(input.SiteName, 120),
		Logo:             logo,
		Favicon:          favicon,
		AnnouncementJSON: normalizeResellerAnnouncement(input.Announcement),
		SupportJSON:      support,
		SEOJSON:          seo,
		FooterLinksJSON:  footerLinks,
		NavConfigJSON:    navConfig,
		ThemeJSON:        theme,
	}, nil
}

func (s *SiteConfigService) GetUserSiteConfig(userID uint) (*resellerdomain.Profile, *resellerdomain.SiteConfig, bool, error) {
	if s == nil || s.repo == nil || userID == 0 {
		return nil, nil, false, productcontract.ErrNotFound
	}
	profile, err := s.repo.GetProfileByUserID(userID)
	if err != nil {
		return nil, nil, false, err
	}
	if profile == nil {
		return nil, nil, false, resellercontract.ErrNotOpened
	}
	canEdit := profile.Status == resellerdomain.ProfileStatusActive
	row, err := s.repo.GetSiteConfigByResellerID(profile.ID)
	if err != nil {
		return nil, nil, canEdit, err
	}
	return profile, row, canEdit, nil
}

func (s *SiteConfigService) UpdateUserSiteConfig(ctx context.Context, userID uint, input ResellerSiteConfigInput) (*resellerdomain.SiteConfig, error) {
	profile, _, canEdit, err := s.GetUserSiteConfig(userID)
	if err != nil {
		return nil, err
	}
	if !canEdit {
		return nil, resellercontract.ErrProfileInactive
	}
	row, err := s.buildModel(profile.ID, input)
	if err != nil {
		return nil, err
	}
	saved, err := s.repo.UpsertSiteConfig(*row)
	if err != nil {
		return nil, err
	}
	_ = cache.Del(ctx, cache.PublicConfigCacheKey(&profile.ID))
	return saved, nil
}

func (s *SiteConfigService) UpdateAdminSiteConfig(ctx context.Context, resellerID uint, input ResellerSiteConfigInput) (*resellerdomain.SiteConfig, error) {
	if resellerID == 0 {
		return nil, productcontract.ErrNotFound
	}
	profile, err := s.repo.GetProfileByID(resellerID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, productcontract.ErrNotFound
	}
	row, err := s.buildModel(resellerID, input)
	if err != nil {
		return nil, err
	}
	saved, err := s.repo.UpsertSiteConfig(*row)
	if err != nil {
		return nil, err
	}
	_ = cache.Del(ctx, cache.PublicConfigCacheKey(&resellerID))
	return saved, nil
}

func (s *SiteConfigService) ResetAdminSiteConfig(ctx context.Context, resellerID uint) error {
	if resellerID == 0 {
		return productcontract.ErrNotFound
	}
	profile, err := s.repo.GetProfileByID(resellerID)
	if err != nil {
		return err
	}
	if profile == nil {
		return productcontract.ErrNotFound
	}
	if err := s.repo.DeleteSiteConfigByResellerID(resellerID); err != nil {
		return err
	}
	_ = cache.Del(ctx, cache.PublicConfigCacheKey(&resellerID))
	return nil
}

func (s *SiteConfigService) ApplyPublicConfigOverlay(ctx context.Context, tenant resellercontract.TenantContext, base map[string]interface{}) (map[string]interface{}, error) {
	out := clonePublicConfigMap(base)
	if tenant.ResellerID == nil || tenant.IsMain || tenant.Unavailable {
		out["tenant"] = map[string]interface{}{"mode": "main", "host": tenant.Host}
		return out, nil
	}
	resellerID := *tenant.ResellerID
	out["tenant"] = map[string]interface{}{
		"mode":           "reseller",
		"host":           tenant.Host,
		"primary_domain": tenant.PrimaryDomain,
	}
	// 主站的统计/客服等自定义脚本不能在白标分销站上运行，否则主站能看到分销站顾客
	out["scripts"] = []interface{}{}
	brand, _ := out["brand"].(map[string]interface{})
	if brand == nil {
		brand = map[string]interface{}{}
	}
	brandHost := tenant.Host
	if strings.TrimSpace(brandHost) == "" {
		brandHost = tenant.PrimaryDomain
	}
	brand["site_url"] = mailbrand.ResellerFallback(brandHost).SiteURL
	out["brand"] = brand
	if s == nil || s.repo == nil {
		return out, nil
	}
	profile, err := s.repo.GetProfileByID(resellerID)
	if err != nil {
		return nil, err
	}
	if profile == nil || profile.Status != resellerdomain.ProfileStatusActive {
		return out, nil
	}
	cfg, err := s.repo.GetSiteConfigByResellerID(resellerID)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return out, nil
	}
	applyResellerSiteConfigToPublicConfig(out, cfg)
	return out, nil
}

func clonePublicConfigMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return map[string]interface{}{}
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return map[string]interface{}{}
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return map[string]interface{}{}
	}
	return out
}

func footerItemsFromEnvelope(raw jsonmap.JSON) []interface{} {
	if raw == nil {
		return make([]interface{}, 0)
	}
	if items, ok := raw["items"].([]interface{}); ok {
		return items
	}
	if typed, ok := raw["items"].([]jsonmap.JSON); ok {
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	}
	return make([]interface{}, 0)
}

// resellerAnnouncementLocalizedMap 兼容两种载体：DB 反序列化后的 map[string]interface{}
// 与 Upsert 后内存对象里的 jsonmap.JSON。
func resellerAnnouncementLocalizedMap(raw interface{}) map[string]interface{} {
	switch typed := raw.(type) {
	case map[string]interface{}:
		return typed
	case jsonmap.JSON:
		return map[string]interface{}(typed)
	default:
		return map[string]interface{}{}
	}
}

// applyResellerAnnouncementToPublicConfig 将分销站公告以主站同构的
// {type,title,content,version} 写入 public config；未启用或内容为空时移除该字段。
func applyResellerAnnouncementToPublicConfig(out map[string]interface{}, raw jsonmap.JSON) {
	delete(out, "announcement")
	if !parseOverlayBool(raw["enabled"]) {
		return
	}
	content := resellerAnnouncementLocalizedMap(raw["content"])
	if !hasAnnouncementContent(content) {
		return
	}
	title := resellerAnnouncementLocalizedMap(raw["title"])
	annType, _ := raw["type"].(string)
	if annType == "" {
		annType = "info"
	}
	out["announcement"] = map[string]interface{}{
		"type":    annType,
		"title":   title,
		"content": content,
		"version": announcementVersion(annType, title, content),
	}
}

func applyResellerSiteConfigToPublicConfig(out map[string]interface{}, cfg *resellerdomain.SiteConfig) {
	if cfg == nil {
		return
	}
	if cfg.ThemeJSON != nil {
		// Only allowlisted values reach the browser, including legacy DB rows.
		template, _ := cfg.ThemeJSON["template"].(string)
		color, _ := cfg.ThemeJSON["primary_color"].(string)
		if theme, err := normalizeResellerTheme(ResellerThemeInput{Template: template, PrimaryColor: color, RedemptionTitle: resellerThemeString(cfg.ThemeJSON, "redemption_title"), RedemptionIntro: resellerThemeString(cfg.ThemeJSON, "redemption_intro"), RedemptionStyle: resellerThemeString(cfg.ThemeJSON, "redemption_style")}); err == nil {
			if template != "" && template != "inherit" {
				out["storefront_template"] = template
			}
			out["reseller_theme"] = theme
		}
	}
	brand, _ := out["brand"].(map[string]interface{})
	if brand == nil {
		brand = map[string]interface{}{}
	}
	if cfg.SiteName != "" {
		brand["site_name"] = cfg.SiteName
	}
	if cfg.Logo != "" {
		brand["site_logo"] = cfg.Logo
	}
	if cfg.Favicon != "" {
		brand["site_icon"] = cfg.Favicon
	}
	out["brand"] = brand

	// A saved reseller config owns contact entirely: empty fields stay hidden
	// instead of falling back to the main site's support channels.
	contact := map[string]interface{}{}
	for _, key := range []string{"telegram", "whatsapp", "email", "support_url"} {
		if value, ok := cfg.SupportJSON[key].(string); ok && strings.TrimSpace(value) != "" {
			contact[key] = value
		}
	}
	out["contact"] = contact

	if len(cfg.SEOJSON) > 0 {
		out["seo"] = cfg.SEOJSON
	}
	// A saved reseller config intentionally owns announcement and navigation:
	// a disabled/empty announcement removes the field so main-site content never
	// leaks into a white-label site; an enabled one is emitted in the same
	// {type,title,content,version} shape the storefront expects (see GetActiveHomeAnnouncement).
	applyResellerAnnouncementToPublicConfig(out, cfg.AnnouncementJSON)
	if len(cfg.FooterLinksJSON) > 0 {
		out["footer_links"] = footerItemsFromEnvelope(cfg.FooterLinksJSON)
	}
	if len(cfg.NavConfigJSON) > 0 {
		out["nav_config"] = cfg.NavConfigJSON
	}
}

func parseOverlayBool(raw interface{}) bool {
	switch value := raw.(type) {
	case bool:
		return value
	case int:
		return value != 0
	case int64:
		return value != 0
	case float64:
		return value != 0
	case string:
		normalized := strings.ToLower(strings.TrimSpace(value))
		return normalized == "1" || normalized == "true" || normalized == "yes" || normalized == "on"
	default:
		return false
	}
}

func hasAnnouncementContent(content map[string]interface{}) bool {
	for _, lang := range constants.SupportedLocales {
		if text, ok := content[lang].(string); ok && strings.TrimSpace(text) != "" {
			return true
		}
	}
	return false
}

func announcementVersion(annType string, title, content map[string]interface{}) string {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(annType))
	for _, lang := range constants.SupportedLocales {
		_, _ = hasher.Write([]byte{0})
		if title != nil {
			if text, ok := title[lang].(string); ok {
				_, _ = hasher.Write([]byte(text))
			}
		}
		_, _ = hasher.Write([]byte{0})
		if content != nil {
			if text, ok := content[lang].(string); ok {
				_, _ = hasher.Write([]byte(text))
			}
		}
	}
	return fmt.Sprintf("%08x", hasher.Sum32())
}

const FrontendMaxZIP = 10 << 20

var ErrFrontendInvalid = errors.New("frontend archive invalid")
var ErrFrontendStale = errors.New("frontend draft changed")
var frontendExtensions = map[string]bool{".html": true, ".js": true, ".mjs": true, ".css": true, ".json": true, ".svg": true, ".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".avif": true, ".ico": true, ".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".txt": true, ".webmanifest": true}

type FrontendService struct {
	repo resellercontract.FrontendStore
}

func NewFrontendService(repo resellercontract.FrontendStore) *FrontendService {
	return &FrontendService{repo: repo}
}

type FrontendStatus struct {
	DraftHash     string    `json:"draft_hash"`
	PublishedHash string    `json:"published_hash"`
	DraftSize     int       `json:"draft_size"`
	UpdatedAt     time.Time `json:"updated_at"`
	PreviewToken  string    `json:"preview_token,omitempty"`
}

func (s *FrontendService) requireUser(user uint) (*resellerdomain.Profile, error) {
	p, err := s.repo.GetProfileByUserID(user)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, resellercontract.ErrNotOpened
	}
	if p.Status != resellerdomain.ProfileStatusActive {
		return nil, resellercontract.ErrProfileInactive
	}
	return p, nil
}
func frontendDigest(b []byte) string { d := sha256.Sum256(b); return hex.EncodeToString(d[:]) }
func ValidateFrontendArchive(raw []byte) (*zip.Reader, error) {
	if len(raw) == 0 || len(raw) > FrontendMaxZIP {
		return nil, ErrFrontendInvalid
	}
	z, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil || len(z.File) > 1000 {
		return nil, ErrFrontendInvalid
	}
	var total uint64
	seen := map[string]bool{}
	hasIndex := false
	for _, f := range z.File {
		n := f.Name
		if !utf8.ValidString(n) || strings.ContainsAny(n, "\\\x00") || strings.HasPrefix(n, "/") || path.Clean(n) != strings.TrimSuffix(n, "/") || (!f.Mode().IsRegular() && !f.FileInfo().IsDir()) {
			return nil, ErrFrontendInvalid
		}
		for _, part := range strings.Split(strings.TrimSuffix(n, "/"), "/") {
			if part == "" || strings.HasPrefix(part, ".") {
				return nil, ErrFrontendInvalid
			}
		}
		top := strings.Split(n, "/")[0]
		if top == "api" || top == "uploads" || top == "health" || top == "__frontend_preview" {
			return nil, ErrFrontendInvalid
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if seen[n] || !frontendExtensions[strings.ToLower(path.Ext(n))] || f.UncompressedSize64 > 5<<20 {
			return nil, ErrFrontendInvalid
		}
		seen[n] = true
		total += f.UncompressedSize64
		if total > 25<<20 {
			return nil, ErrFrontendInvalid
		}
		reader, err := f.Open()
		if err != nil {
			return nil, ErrFrontendInvalid
		}
		b, err := io.ReadAll(io.LimitReader(reader, int64(5<<20)+1))
		reader.Close()
		if err != nil || len(b) > 5<<20 {
			return nil, ErrFrontendInvalid
		}
		if n == "index.html" {
			hasIndex = true
			if !utf8.Valid(b) || !bytes.Contains(bytes.ToLower(b), []byte("<html")) {
				return nil, ErrFrontendInvalid
			}
		}
	}
	if !hasIndex {
		return nil, ErrFrontendInvalid
	}
	return z, nil
}
func statusFrontend(row *resellerdomain.FrontendBundle) *FrontendStatus {
	if row == nil {
		return &FrontendStatus{}
	}
	return &FrontendStatus{DraftHash: row.DraftHash, PublishedHash: row.PublishedHash, DraftSize: len(row.Draft), UpdatedAt: row.UpdatedAt}
}
func (s *FrontendService) Status(user uint) (*FrontendStatus, error) {
	p, e := s.requireUser(user)
	if e != nil {
		return nil, e
	}
	r, e := s.repo.GetFrontend(p.ID)
	return statusFrontend(r), e
}
func (s *FrontendService) Upload(user uint, raw []byte) (*FrontendStatus, error) {
	p, e := s.requireUser(user)
	if e != nil {
		return nil, e
	}
	if _, e = ValidateFrontendArchive(raw); e != nil {
		return nil, e
	}
	token := make([]byte, 24)
	if _, e = rand.Read(token); e != nil {
		return nil, e
	}
	key := hex.EncodeToString(token)
	e = s.repo.PutFrontendDraft(p.ID, raw, frontendDigest(raw), frontendDigest([]byte(key)), time.Now().Add(time.Hour))
	if e != nil {
		return nil, e
	}
	row, e := s.repo.GetFrontend(p.ID)
	out := statusFrontend(row)
	out.PreviewToken = key
	return out, e
}
func (s *FrontendService) PreviewToken(user uint) (*FrontendStatus, error) {
	p, e := s.requireUser(user)
	if e != nil {
		return nil, e
	}
	if e = s.requireFrontendDomain(p.ID); e != nil {
		return nil, e
	}
	row, e := s.repo.GetFrontend(p.ID)
	if e != nil {
		return nil, e
	}
	if row == nil || len(row.Draft) == 0 {
		return nil, ErrFrontendInvalid
	}
	token := make([]byte, 24)
	if _, e = rand.Read(token); e != nil {
		return nil, e
	}
	key := hex.EncodeToString(token)
	ok, e := s.repo.RotateFrontendPreview(p.ID, row.DraftHash, frontendDigest([]byte(key)), time.Now().Add(time.Hour))
	if e != nil {
		return nil, e
	}
	if !ok {
		return nil, ErrFrontendStale
	}
	out := statusFrontend(row)
	out.PreviewToken = key
	return out, nil
}
func (s *FrontendService) Publish(user uint, expected string) error {
	p, e := s.requireUser(user)
	if e != nil {
		return e
	}
	if e = s.requireFrontendDomain(p.ID); e != nil {
		return e
	}
	ok, e := s.repo.PublishFrontend(p.ID, expected)
	if e != nil {
		return e
	}
	if !ok {
		return ErrFrontendStale
	}
	return nil
}
func (s *FrontendService) Disable(user uint) error {
	p, e := s.requireUser(user)
	if e != nil {
		return e
	}
	return s.repo.DisableFrontend(p.ID)
}

type FrontendAsset struct {
	Data        []byte
	ContentType string
	Preview     bool
}

func (s *FrontendService) Asset(tenant resellercontract.TenantContext, requestPath, token string) (*FrontendAsset, error) {
	if !tenant.IsReseller() {
		return nil, nil
	}
	p, e := s.repo.GetProfileByID(*tenant.ResellerID)
	if e != nil {
		return nil, e
	}
	if p == nil || p.Status != resellerdomain.ProfileStatusActive {
		return nil, nil
	}
	row, e := s.repo.GetFrontend(p.ID)
	if e != nil || row == nil {
		return nil, e
	}
	raw := row.Published
	preview := token != ""
	if preview {
		if time.Now().After(row.PreviewExpiresAt) || subtle.ConstantTimeCompare([]byte(row.PreviewHash), []byte(frontendDigest([]byte(token)))) != 1 {
			return nil, ErrFrontendInvalid
		}
		raw = row.Draft
	}
	if len(raw) == 0 {
		return nil, nil
	}
	z, e := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if e != nil {
		return nil, e
	}
	name := strings.TrimPrefix(requestPath, "/")
	if name == "" {
		name = "index.html"
	}
	if path.Clean(name) != name || strings.ContainsAny(name, "\\\x00") {
		return nil, ErrFrontendInvalid
	}
	var selected *zip.File
	for _, f := range z.File {
		if f.Name == name {
			selected = f
			break
		}
	}
	if selected == nil && path.Ext(name) == "" {
		for _, f := range z.File {
			if f.Name == "index.html" {
				selected = f
				break
			}
		}
	}
	if selected == nil {
		return &FrontendAsset{}, nil
	}
	reader, e := selected.Open()
	if e != nil {
		return nil, e
	}
	defer reader.Close()
	data, e := io.ReadAll(io.LimitReader(reader, 5<<20+1))
	if e != nil {
		return nil, e
	}
	ext := strings.ToLower(path.Ext(selected.Name))
	contentType := mime.TypeByExtension(ext)
	if ext == ".js" || ext == ".mjs" {
		contentType = "text/javascript; charset=utf-8"
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if preview && (ext == ".html" || ext == ".js" || ext == ".mjs" || ext == ".css" || ext == ".json") {
		prefix := "/__frontend_preview/" + token + "/"
		dirs := map[string]bool{}
		for _, f := range z.File {
			top := strings.Split(f.Name, "/")[0]
			dirs[top] = true
		}
		text := string(data)
		for top := range dirs {
			for _, quote := range []string{"\"", "'", "`", "("} {
				text = strings.ReplaceAll(text, quote+"/"+top, quote+prefix+top)
			}
		}
		if ext == ".html" {
			text = strings.Replace(text, "<head>", "<head><base href=\""+prefix+"\">", 1)
		}
		data = []byte(text)
	}
	return &FrontendAsset{Data: data, ContentType: contentType, Preview: preview}, nil
}

func normalizeResellerTheme(input ResellerThemeInput) (jsonmap.JSON, error) {
	template := strings.TrimSpace(input.Template)
	switch template {
	case "", "inherit", "xgai", "classic", "vault":
	default:
		return nil, newResellerFieldError("theme")
	}
	color := strings.TrimSpace(input.PrimaryColor)
	if color != "" {
		if len(color) != 7 || color[0] != '#' {
			return nil, newResellerFieldError("theme")
		}
		if _, err := hex.DecodeString(color[1:]); err != nil {
			return nil, newResellerFieldError("theme")
		}
	}
	style := strings.TrimSpace(input.RedemptionStyle)
	if style != "" && style != "glass" && style != "simple" {
		return nil, newResellerFieldError("theme")
	}
	if len([]rune(input.RedemptionTitle)) > 120 || len([]rune(input.RedemptionIntro)) > 1500 {
		return nil, newResellerFieldError("theme")
	}
	return jsonmap.JSON{"template": template, "primary_color": strings.ToLower(color), "redemption_title": strings.TrimSpace(input.RedemptionTitle), "redemption_intro": strings.TrimSpace(input.RedemptionIntro), "redemption_style": style}, nil
}

func (s *FrontendService) requireFrontendDomain(resellerID uint) error {
	domains, err := s.repo.ListDomainsByResellerID(resellerID)
	if err != nil {
		return err
	}
	for _, d := range domains {
		if d.Status == resellerdomain.DomainStatusActive && d.VerificationStatus == resellerdomain.DomainVerificationVerified {
			return nil
		}
	}
	return ErrFrontendInvalid
}

// RedemptionSite returns only public branding for an active, verified reseller domain.
// Read directly from storage: disabling a site must revoke redemption access immediately.
func (s *SiteConfigService) RedemptionSite(host string) (map[string]interface{}, error) {
	if host == "" || host != resellercontract.NormalizeHost(host) || strings.ContainsAny(host, "/:@?#\\ ") {
		return nil, nil
	}
	lookup, ok := s.repo.(resellercontract.DomainLookupRepository)
	if !ok {
		return nil, nil
	}
	row, err := lookup.FindActiveVerifiedDomain(host)
	if err != nil {
		return nil, err
	}
	if row == nil || row.Profile == nil || row.Profile.Status != resellerdomain.ProfileStatusActive {
		return nil, nil
	}
	cfg, err := s.repo.GetSiteConfigByResellerID(row.ResellerID)
	if err != nil {
		return nil, err
	}
	out := map[string]interface{}{"enabled": true, "origin": "https://" + host, "site_name": "雪糕 AI", "logo": "", "favicon": "", "primary_color": "", "telegram": ""}
	if cfg != nil {
		out["site_name"] = cfg.SiteName
		out["logo"] = cfg.Logo
		out["favicon"] = cfg.Favicon
		if theme, err := normalizeResellerTheme(ResellerThemeInput{PrimaryColor: resellerThemeColor(cfg.ThemeJSON)}); err == nil {
			out["primary_color"] = theme["primary_color"]
		}
		out["telegram"], _ = cfg.SupportJSON["telegram"].(string)
	}
	return out, nil
}

func resellerThemeColor(theme map[string]interface{}) string {
	value, _ := theme["primary_color"].(string)
	return value
}

func resellerThemeString(theme map[string]interface{}, key string) string {
	v, _ := theme[key].(string)
	return v
}
