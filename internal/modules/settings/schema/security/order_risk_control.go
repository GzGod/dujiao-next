package settingssecurity

import (
	"encoding/hex"
	"encoding/json"
	"github.com/dujiao-next/internal/shared/risklimits"
	"net"
	"net/mail"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/dujiao-next/internal/shared/jsonmap"
)

const orderRiskControlVersion = 2

var settlementEmailDomainPattern = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

const (
	SettlementModeShadow     = "shadow"
	SettlementModeHoldNotice = "hold_notice"
	SettlementModeSilentTrap = "silent_trap"
)

const DefaultAlipayRiskSubject = "虚拟商品·非本人购买请勿付款 {order_no}"

// OrderRateLimitConfig 下单频率限制配置。
type OrderRateLimitConfig struct {
	Enabled       bool `json:"enabled"`
	WindowSeconds int  `json:"window_seconds"`
	MaxRequests   int  `json:"max_requests"`
	BlockSeconds  int  `json:"block_seconds"`
}

// SettlementListEntry is a temporary or permanent risk list entry.
type SettlementListEntry struct {
	Kind      string     `json:"kind"`
	Value     string     `json:"value"`
	Allow     bool       `json:"allow,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// OrderRiskCommonPolicy 保存与购买身份无关的订单风控策略。
type OrderRiskCommonPolicy struct {
	SettlementList []SettlementListEntry `json:"settlement_list,omitempty"`
	IPBlacklist    []string              `json:"ip_blacklist"`
}

// OrderRiskGuestPolicy 保存以可信客户端 IP 为主体的游客库存保护策略。
type OrderRiskGuestPolicy struct {
	Enabled                        bool                 `json:"enabled"`
	MaxPendingOrdersPerIP          int                  `json:"max_pending_orders_per_ip"`
	MaxQuantityPerProductPerOrder  int                  `json:"max_quantity_per_product_per_order"`
	MaxPendingQuantityPerIPProduct int                  `json:"max_pending_quantity_per_ip_product"`
	PaymentExpireMinutes           int                  `json:"payment_expire_minutes"`
	RateLimit                      OrderRateLimitConfig `json:"rate_limit"`
}

// OrderRiskMemberPolicy 保存以用户 ID 为主体、默认更宽松的会员策略。
type OrderRiskMemberPolicy struct {
	Enabled                       bool                 `json:"enabled"`
	MaxPendingOrdersPerUser       int                  `json:"max_pending_orders_per_user"`
	MaxPendingOrdersPerIP         int                  `json:"max_pending_orders_per_ip"`
	MaxQuantityPerProductPerOrder int                  `json:"max_quantity_per_product_per_order"`
	RateLimit                     OrderRateLimitConfig `json:"rate_limit"`
}

// OrderRiskControlConfig 订单风控配置。游客邮箱仅用于订单业务，不作为风控身份。
type OrderRiskControlConfig struct {
	SettlementLimits    risklimits.Limits     `json:"settlement_limits"`
	AlipayRiskSubject   string                `json:"alipay_risk_subject"`
	RiskyEmailDomains   []string              `json:"risky_email_domains,omitempty"`
	SettlementThreshold int                   `json:"settlement_threshold"`
	SettlementWeights   map[string]int        `json:"settlement_weights,omitempty"`
	SettlementMode      string                `json:"settlement_mode"`
	Version             int                   `json:"version"`
	Enabled             bool                  `json:"enabled"`
	Common              OrderRiskCommonPolicy `json:"common"`
	Guest               OrderRiskGuestPolicy  `json:"guest"`
	Member              OrderRiskMemberPolicy `json:"member"`
}

// DefaultOrderRiskControlConfig 返回新安装推荐值；总开关默认关闭，避免静默改变订单行为。
func DefaultOrderRiskControlConfig() OrderRiskControlConfig {
	return OrderRiskControlConfig{
		Version:             orderRiskControlVersion,
		SettlementMode:      SettlementModeShadow,
		SettlementThreshold: 60,
		AlipayRiskSubject:   DefaultAlipayRiskSubject,
		Enabled:             false,
		Common:              OrderRiskCommonPolicy{IPBlacklist: []string{}},
		Guest: OrderRiskGuestPolicy{
			Enabled:                        true,
			MaxPendingOrdersPerIP:          2,
			MaxQuantityPerProductPerOrder:  1,
			MaxPendingQuantityPerIPProduct: 2,
			PaymentExpireMinutes:           5,
			RateLimit: OrderRateLimitConfig{
				Enabled:       true,
				WindowSeconds: 60,
				MaxRequests:   3,
				BlockSeconds:  120,
			},
		},
		Member: OrderRiskMemberPolicy{
			Enabled:                       true,
			MaxPendingOrdersPerUser:       5,
			MaxPendingOrdersPerIP:         0,
			MaxQuantityPerProductPerOrder: 0,
			RateLimit: OrderRateLimitConfig{
				Enabled:       false,
				WindowSeconds: 60,
				MaxRequests:   10,
				BlockSeconds:  120,
			},
		},
	}
}

// NormalizeOrderRiskControlConfig 归一化风控配置；0 始终表示对应限制关闭。
func NormalizeOrderRiskControlConfig(cfg OrderRiskControlConfig) OrderRiskControlConfig {
	defaults := DefaultOrderRiskControlConfig()
	cfg.SettlementLimits = cfg.SettlementLimits.Normalized()
	cfg.Version = orderRiskControlVersion
	cfg.AlipayRiskSubject = strings.TrimSpace(cfg.AlipayRiskSubject)
	if cfg.AlipayRiskSubject == "" || !utf8.ValidString(cfg.AlipayRiskSubject) ||
		utf8.RuneCountInString(cfg.AlipayRiskSubject) > 80 || strings.IndexFunc(cfg.AlipayRiskSubject, unicode.IsControl) >= 0 {
		cfg.AlipayRiskSubject = DefaultAlipayRiskSubject
	}
	if cfg.SettlementThreshold < 1 || cfg.SettlementThreshold > 800 {
		cfg.SettlementThreshold = 60
	}
	domains := make([]string, 0, min(len(cfg.RiskyEmailDomains), 250))
	seenDomains := make(map[string]struct{})
	for _, raw := range cfg.RiskyEmailDomains {
		name := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
		if len(name) > 253 || !settlementEmailDomainPattern.MatchString(name) {
			continue
		}
		if _, exists := seenDomains[name]; exists {
			continue
		}
		seenDomains[name] = struct{}{}
		domains = append(domains, name)
		if len(domains) == 250 {
			break
		}
	}
	cfg.RiskyEmailDomains = domains
	switch cfg.SettlementMode {
	case SettlementModeHoldNotice, SettlementModeSilentTrap:
	default:
		cfg.SettlementMode = SettlementModeShadow
	}
	cfg.Guest.MaxPendingOrdersPerIP = normalizeRiskLimit(cfg.Guest.MaxPendingOrdersPerIP, 100, defaults.Guest.MaxPendingOrdersPerIP)
	cfg.Guest.MaxQuantityPerProductPerOrder = normalizeRiskLimit(cfg.Guest.MaxQuantityPerProductPerOrder, 100000, defaults.Guest.MaxQuantityPerProductPerOrder)
	cfg.Guest.MaxPendingQuantityPerIPProduct = normalizeRiskLimit(cfg.Guest.MaxPendingQuantityPerIPProduct, 100000, defaults.Guest.MaxPendingQuantityPerIPProduct)
	cfg.Guest.PaymentExpireMinutes = normalizeRiskLimit(cfg.Guest.PaymentExpireMinutes, 10080, defaults.Guest.PaymentExpireMinutes)
	cfg.Guest.RateLimit = normalizeRateLimit(cfg.Guest.RateLimit, defaults.Guest.RateLimit)

	cfg.Member.MaxPendingOrdersPerUser = normalizeRiskLimit(cfg.Member.MaxPendingOrdersPerUser, 100, defaults.Member.MaxPendingOrdersPerUser)
	cfg.Member.MaxPendingOrdersPerIP = normalizeRiskLimit(cfg.Member.MaxPendingOrdersPerIP, 100, defaults.Member.MaxPendingOrdersPerIP)
	cfg.Member.MaxQuantityPerProductPerOrder = normalizeRiskLimit(cfg.Member.MaxQuantityPerProductPerOrder, 100000, defaults.Member.MaxQuantityPerProductPerOrder)
	cfg.Member.RateLimit = normalizeRateLimit(cfg.Member.RateLimit, defaults.Member.RateLimit)

	cleanIPs := make([]string, 0, len(cfg.Common.IPBlacklist))
	seen := make(map[string]struct{}, len(cfg.Common.IPBlacklist))
	for _, raw := range cfg.Common.IPBlacklist {
		entry := strings.TrimSpace(raw)
		if entry == "" || !isValidIPOrCIDR(entry) {
			continue
		}
		if _, exists := seen[entry]; exists {
			continue
		}
		seen[entry] = struct{}{}
		cleanIPs = append(cleanIPs, entry)
	}
	cfg.Common.IPBlacklist = cleanIPs
	cfg.Common.SettlementList = normalizeSettlementList(cfg.Common.SettlementList)
	return cfg
}

// normalizeSettlementList preserves blacklist priority by keeping opposite
// actions as distinct rules. Unsupported allow rules are ignored.
func normalizeSettlementList(entries []SettlementListEntry) []SettlementListEntry {
	var normalized []SettlementListEntry
	seen := make(map[struct {
		Kind, Value, Expiry string
		Allow               bool
	}]struct{})
	for _, entry := range entries {
		entry.Kind = strings.ToLower(strings.TrimSpace(entry.Kind))
		entry.Value = strings.TrimSpace(entry.Value)
		switch entry.Kind {
		case "ip":
			ip, err := netip.ParseAddr(entry.Value)
			if err != nil || ip.Zone() != "" {
				continue
			}
			entry.Value = ip.Unmap().String()
		case "cidr":
			prefix, err := netip.ParsePrefix(entry.Value)
			if err != nil {
				continue
			}
			entry.Value = prefix.Masked().String()
		case "user_id":
			id, err := strconv.ParseUint(entry.Value, 10, 64)
			if err != nil || id == 0 {
				continue
			}
			entry.Value = strconv.FormatUint(id, 10)
		case "device_token":
			digest, err := hex.DecodeString(entry.Value)
			if err != nil || len(digest) != 32 {
				continue
			}
			entry.Value = hex.EncodeToString(digest)
		case "contact_norm":
			address, err := mail.ParseAddress(entry.Value)
			if err != nil || address.Address != entry.Value || len(entry.Value) > 254 || entry.Allow {
				continue
			}
			entry.Value = strings.ToLower(entry.Value)
		default:
			continue
		}
		expiry := ""
		if entry.ExpiresAt != nil {
			at := entry.ExpiresAt.UTC()
			entry.ExpiresAt = &at
			expiry = at.Format(time.RFC3339Nano)
		}
		key := struct {
			Kind, Value, Expiry string
			Allow               bool
		}{entry.Kind, entry.Value, expiry, entry.Allow}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, entry)
	}
	return normalized
}

func normalizeRiskLimit(value, maximum, fallback int) int {
	if value < 0 || value > maximum {
		return fallback
	}
	return value
}

func normalizeRateLimit(cfg, fallback OrderRateLimitConfig) OrderRateLimitConfig {
	if cfg.WindowSeconds < 10 || cfg.WindowSeconds > 3600 {
		cfg.WindowSeconds = fallback.WindowSeconds
	}
	if cfg.MaxRequests < 1 || cfg.MaxRequests > 100 {
		cfg.MaxRequests = fallback.MaxRequests
	}
	if cfg.BlockSeconds < 0 || cfg.BlockSeconds > 86400 {
		cfg.BlockSeconds = fallback.BlockSeconds
	}
	return cfg
}

func isValidIPOrCIDR(value string) bool {
	if net.ParseIP(value) != nil {
		return true
	}
	_, _, err := net.ParseCIDR(value)
	return err == nil
}

// DecodeOrderRiskControlConfig 解析新版嵌套配置，并把旧版扁平配置一次性映射到游客/会员策略。
func DecodeOrderRiskControlConfig(raw jsonmap.JSON, fallback OrderRiskControlConfig) OrderRiskControlConfig {
	if raw == nil {
		return NormalizeOrderRiskControlConfig(fallback)
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return NormalizeOrderRiskControlConfig(fallback)
	}
	if _, hasGuest := raw["guest"]; hasGuest {
		result := fallback
		if err := json.Unmarshal(data, &result); err != nil {
			return NormalizeOrderRiskControlConfig(fallback)
		}
		return NormalizeOrderRiskControlConfig(result)
	}

	type flatRateLimitConfig struct {
		Enabled       *bool `json:"enabled"`
		WindowSeconds *int  `json:"window_seconds"`
		MaxRequests   *int  `json:"max_requests"`
		BlockSeconds  *int  `json:"block_seconds"`
	}
	type flatOrderRiskConfig struct {
		Enabled                 *bool                `json:"enabled"`
		MaxPendingOrdersPerUser *int                 `json:"max_pending_orders_per_user"`
		MaxPendingOrdersPerIP   *int                 `json:"max_pending_orders_per_ip"`
		OrderRateLimit          *flatRateLimitConfig `json:"order_rate_limit"`
		IPBlacklist             []string             `json:"ip_blacklist"`
	}
	var old flatOrderRiskConfig
	if err := json.Unmarshal(data, &old); err != nil {
		return NormalizeOrderRiskControlConfig(fallback)
	}
	result := fallback
	// 先恢复旧版缺省值，再叠加持久化字段。这样旧的稀疏 JSON 也不会误用 v2 推荐值。
	result.Guest.MaxPendingOrdersPerIP = 5
	result.Member.MaxPendingOrdersPerUser = 3
	result.Member.MaxPendingOrdersPerIP = 5
	result.Guest.MaxQuantityPerProductPerOrder = 0
	result.Guest.MaxPendingQuantityPerIPProduct = 0
	result.Guest.PaymentExpireMinutes = 0
	result.Member.MaxQuantityPerProductPerOrder = 0
	legacyRateLimit := OrderRateLimitConfig{Enabled: false, WindowSeconds: 60, MaxRequests: 5, BlockSeconds: 120}
	result.Guest.RateLimit = legacyRateLimit
	result.Member.RateLimit = legacyRateLimit
	if old.Enabled != nil {
		result.Enabled = *old.Enabled
	}
	if old.MaxPendingOrdersPerUser != nil {
		result.Member.MaxPendingOrdersPerUser = *old.MaxPendingOrdersPerUser
	}
	if old.MaxPendingOrdersPerIP != nil {
		result.Guest.MaxPendingOrdersPerIP = *old.MaxPendingOrdersPerIP
		result.Member.MaxPendingOrdersPerIP = *old.MaxPendingOrdersPerIP
	}
	if old.OrderRateLimit != nil {
		if old.OrderRateLimit.Enabled != nil {
			legacyRateLimit.Enabled = *old.OrderRateLimit.Enabled
		}
		if old.OrderRateLimit.WindowSeconds != nil {
			legacyRateLimit.WindowSeconds = *old.OrderRateLimit.WindowSeconds
		}
		if old.OrderRateLimit.MaxRequests != nil {
			legacyRateLimit.MaxRequests = *old.OrderRateLimit.MaxRequests
		}
		if old.OrderRateLimit.BlockSeconds != nil {
			legacyRateLimit.BlockSeconds = *old.OrderRateLimit.BlockSeconds
		}
		result.Guest.RateLimit = legacyRateLimit
		result.Member.RateLimit = legacyRateLimit
	}
	if old.IPBlacklist != nil {
		result.Common.IPBlacklist = old.IPBlacklist
	}
	return NormalizeOrderRiskControlConfig(result)
}

func EncodeOrderRiskControlConfig(cfg OrderRiskControlConfig) jsonmap.JSON {
	normalized := NormalizeOrderRiskControlConfig(cfg)
	data, err := json.Marshal(normalized)
	if err != nil {
		return jsonmap.JSON{}
	}
	var result jsonmap.JSON
	_ = json.Unmarshal(data, &result)
	return result
}

// NormalizeOrderRiskControlConfigJSON 是 Registry 使用的原始 JSON 写入策略。
func NormalizeOrderRiskControlConfigJSON(value jsonmap.JSON) jsonmap.JSON {
	return EncodeOrderRiskControlConfig(DecodeOrderRiskControlConfig(value, DefaultOrderRiskControlConfig()))
}
