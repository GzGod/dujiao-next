package contract

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode"

	riskdomain "github.com/dujiao-next/internal/modules/orderrisk/domain"

	settingssecurity "github.com/dujiao-next/internal/modules/settings/schema/security"
)

// OrderItem 是风控所需的最小订单项快照。
type OrderItem struct {
	ProductID uint
	Quantity  int
}

// CheckInput 是订单风控检查所需的调用上下文。
type CheckInput struct {
	RiskDeviceHash   string
	GuestEmail       string
	UserID           uint
	ClientIP         string
	RiskIP           string
	IsGuest          bool
	SkipIPCheck      bool
	ConsumeRateLimit bool
	Items            []OrderItem
}

// CheckResult 返回创建订单后续步骤需要复用的不可变风控上下文。
type CheckResult struct {
	RiskIP               string
	PaymentExpireMinutes int
	ConfigSnapshot       settingssecurity.OrderRiskControlConfig
}

// NormalizeRiskIP 生成游客风控键：IPv4 使用完整地址，IPv6 按 /64 前缀聚合。
func NormalizeRiskIP(raw string) string {
	ip := net.ParseIP(strings.TrimSpace(raw))
	if ip == nil {
		return ""
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		return ipv4.String()
	}
	network := ip.Mask(net.CIDRMask(64, 128))
	if network == nil {
		return ""
	}
	return network.String() + "/64"
}

// SettlementFacts are the payment-side inputs to the risk evaluator.
type SettlementFacts riskdomain.SettlementFacts

// EvaluateSettlement applies only recognized, bounded rule overrides.
func EvaluateSettlement(f SettlementFacts, cfg settingssecurity.OrderRiskControlConfig) riskdomain.SettlementDecision {
	policy := riskdomain.DefaultSettlementPolicy()
	policy.Limits = cfg.SettlementLimits
	if cfg.SettlementThreshold > 0 && cfg.SettlementThreshold <= 800 {
		policy.Threshold = cfg.SettlementThreshold
	}
	for name := range policy.Weights {
		if weight, ok := cfg.SettlementWeights[name]; ok && weight >= 0 && weight <= 100 {
			policy.Weights[name] = weight
		}
	}
	return riskdomain.EvaluateSettlement(riskdomain.SettlementFacts(f), policy)
}

// SettlementIPBlacklisted evaluates the configured exact addresses and networks.
func SettlementIPBlacklisted(raw string, cfg settingssecurity.OrderRiskControlConfig) bool {
	ip := net.ParseIP(strings.TrimSpace(raw))
	if !cfg.Enabled || ip == nil {
		return false
	}
	for _, entry := range cfg.Common.IPBlacklist {
		if ip.Equal(net.ParseIP(entry)) {
			return true
		}
		if _, network, err := net.ParseCIDR(entry); err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

// CompareGuestContacts compares scoped risk keys without changing order identity.
func CompareGuestContacts(a, b string) (same, similar bool) {
	left, right := riskdomain.NormalizeEmail(a), riskdomain.NormalizeEmail(b)
	return left != "" && left == right, riskdomain.SimilarEmail(a, b)
}

// SettlementRiskyEmail matches configured disposable hosts and narrowly
// scoped machine-looking mailbox names as a soft signal.
func SettlementRiskyEmail(email string, cfg settingssecurity.OrderRiskControlConfig) bool {
	local, host, ok := strings.Cut(riskdomain.NormalizeEmail(email), "@")
	if !ok {
		return false
	}
	host = strings.TrimSuffix(host, ".")
	for _, domain := range cfg.RiskyEmailDomains {
		if host == domain {
			return true
		}
	}
	// Skip short names, numeric mailbox IDs and names with readable
	// separators or vowels. This intentionally catches only obvious noise.
	if len(local) < 24 || len(local) > 64 {
		return false
	}
	digits, consonants := 0, 0
	for _, char := range local {
		switch {
		case '0' <= char && char <= '9':
			digits++
		case 'a' <= char && char <= 'z' && !strings.ContainsRune("aeiou", char):
			consonants++
		default:
			return false
		}
	}
	return digits >= 8 && consonants >= 8
}

// MatchSettlementList evaluates active rules. A deny always wins, and
// unverified guest contacts cannot grant exemptions.
func MatchSettlementList(input CheckInput, cfg settingssecurity.OrderRiskControlConfig, now time.Time) (denied, allowed bool) {
	if !cfg.Enabled {
		return false, false
	}
	if !input.SkipIPCheck && SettlementIPBlacklisted(input.ClientIP, cfg) {
		return true, false
	}
	for _, entry := range cfg.Common.SettlementList {
		if entry.ExpiresAt != nil && !now.Before(*entry.ExpiresAt) {
			continue
		}
		value := strings.TrimSpace(entry.Value)
		match := false
		switch entry.Kind {
		case "user_id":
			id, err := strconv.ParseUint(value, 10, 64)
			match = err == nil && input.UserID != 0 && uint64(input.UserID) == id
		case "device_token":
			match = len(input.RiskDeviceHash) == 64 && subtle.ConstantTimeCompare([]byte(input.RiskDeviceHash), []byte(strings.ToLower(value))) == 1
		case "contact_norm":
			normalized := riskdomain.NormalizeEmail(input.GuestEmail)
			match = !entry.Allow && normalized != "" && normalized == riskdomain.NormalizeEmail(value)
		case "ip", "cidr":
			if !input.SkipIPCheck {
				ip := net.ParseIP(strings.TrimSpace(input.ClientIP))
				if ip != nil {
					if entry.Kind == "ip" {
						match = ip.Equal(net.ParseIP(value))
					} else if _, subnet, err := net.ParseCIDR(value); err == nil {
						match = subnet.Contains(ip)
					}
				}
			}
		}
		if match {
			if !entry.Allow {
				return true, false
			}
			allowed = true
		}
	}
	return false, allowed
}

// SettlementLockKeys groups concurrent payment decisions without storing raw
// identity data in the lock table. Storage hashes and sorts these keys.
func SettlementLockKeys(input CheckInput) []string {
	keys := make([]string, 0, 3)
	if input.UserID != 0 {
		keys = append(keys, "settlement:user:"+strconv.FormatUint(uint64(input.UserID), 10))
	} else if email := riskdomain.NormalizeEmail(input.GuestEmail); email != "" {
		keys = append(keys, "settlement:guest:"+email)
	}
	if len(input.RiskDeviceHash) == 64 {
		keys = append(keys, "settlement:device:"+input.RiskDeviceHash)
	}
	if subnet := SettlementSubnet(input.ClientIP); subnet != "" {
		keys = append(keys, "settlement:subnet:"+subnet)
	}
	return keys
}

// GuestContactHash is a normalized risk index, separate from guest credentials.
func GuestContactHash(email string) string {
	normalized := riskdomain.NormalizeEmail(email)
	if normalized == "" {
		return ""
	}
	digest := sha256.Sum256([]byte("settlement-contact:v1:" + normalized))
	return hex.EncodeToString(digest[:])
}

func SettlementSubnet(raw string) string {
	ip := net.ParseIP(strings.TrimSpace(raw))
	if ip == nil {
		return ""
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		return ipv4.Mask(net.CIDRMask(24, 32)).String() + "/24"
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

// SettlementUserAgent is bounded audit context, never a risk identity key.
func SettlementUserAgent(raw string) string {
	var cleaned strings.Builder
	for _, r := range strings.TrimSpace(raw) {
		if unicode.IsControl(r) {
			continue
		}
		if cleaned.Len() >= 512 {
			break
		}
		cleaned.WriteRune(r)
	}
	return cleaned.String()
}

// ReviewListValues derives selectable list rules from verified order context.
func ReviewListValues(input CheckInput) map[string]string {
	values := make(map[string]string, 4)
	if ip := net.ParseIP(strings.TrimSpace(input.ClientIP)); ip != nil {
		values["ip"] = ip.String()
		values["cidr"] = SettlementSubnet(input.ClientIP)
	}
	if raw, err := hex.DecodeString(input.RiskDeviceHash); err == nil && len(raw) == 32 {
		values["device_token"] = hex.EncodeToString(raw)
	}
	if input.UserID != 0 {
		values["user_id"] = strconv.FormatUint(uint64(input.UserID), 10)
	}
	return values
}
