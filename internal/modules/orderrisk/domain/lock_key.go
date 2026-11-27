package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/dujiao-next/internal/shared/risklimits"
	"strings"
	"time"
)

// LockKey 仅保存风控维度摘要，用于跨进程序列化相同身份的库存配额检查。
type LockKey struct {
	KeyHash   string    `gorm:"type:varchar(64);primaryKey" json:"-"`
	CreatedAt time.Time `gorm:"not null" json:"-"`
}

func (LockKey) TableName() string { return "order_risk_lock_keys" }

const (
	RuleFirstHigh      = "first_seen_high_amount"
	RuleSlowPayment    = "slow_qr_payment"
	RuleDeviceSwitch   = "same_device_contact_switch"
	RuleUnpaidModerate = "unpaid_probe_moderate"
	RuleUnredeemed     = "unredeemed_high_reorder"
	RuleSubnetBurst    = "subnet_burst_high_amount"
	RuleSharedIP       = "same_ip_different_contact"
	RuleDisposable     = "disposable_or_random_email"
)

// SettlementFacts contains scoped historical aggregates, with the current order
// excluded. Amounts are expressed in CNY fen.
type SettlementFacts struct {
	Amount, PaidAmount30m, PaidAmount24h int64
	PaidCount30m, PaidCount24h           int
	Unpaid30m, Unpaid24h                 int
	PaymentDelay, CustomerAge            time.Duration
	HistoricalSuccessfulOrders           int
	Blacklisted, ApproximateContact      bool
	RecentMonitoredWalletFunding         bool
	NewContact, NewIP, DeviceSwitch      bool
	UnredeemedHighOrder, SubnetBurst     bool
	DifferentContactSameIP, RiskyEmail   bool
}

type SettlementPolicy struct {
	Limits    risklimits.Limits
	Threshold int
	Weights   map[string]int
}

type SettlementDecision struct {
	Score     int
	HardRules []string
	Signals   map[string]int
	Exempt    bool
	Hold      bool
}

func DefaultSettlementPolicy() SettlementPolicy {
	return SettlementPolicy{Threshold: 60, Weights: map[string]int{
		RuleFirstHigh: 25, RuleSlowPayment: 35, RuleDeviceSwitch: 40,
		RuleUnpaidModerate: 35, RuleUnredeemed: 35, RuleSubnetBurst: 30,
		RuleSharedIP: 15, RuleDisposable: 25,
	}}
}

func EvaluateSettlement(f SettlementFacts, p SettlementPolicy) SettlementDecision {
	limits := p.Limits.Normalized()
	d := SettlementDecision{Signals: make(map[string]int)}
	if f.Blacklisted {
		d.HardRules = append(d.HardRules, "blacklist_hit")
		d.Hold = true
		return d
	}
	if !f.RecentMonitoredWalletFunding && f.CustomerAge >= time.Duration(limits.LoyalDays)*24*time.Hour && f.HistoricalSuccessfulOrders >= int(limits.LoyalOrders) {
		d.Exempt = true
		return d
	}
	if f.ApproximateContact {
		d.HardRules = append(d.HardRules, "contact_variant_approx")
	}
	if (f.PaidCount30m > 0 && (f.Amount >= limits.RepeatAmount*100 || f.PaidAmount30m+f.Amount >= limits.ShortTotal*100)) ||
		int64(f.PaidCount24h)+1 >= limits.PaidCount || f.PaidAmount24h+f.Amount >= limits.LongTotal*100 && f.PaidCount24h > 0 {
		d.HardRules = append(d.HardRules, "multi_paid_strong_identity")
	}
	if int64(f.Unpaid30m) >= limits.ShortUnpaid || int64(f.Unpaid24h) >= limits.LongUnpaid {
		d.HardRules = append(d.HardRules, "unpaid_probe_burst")
	}
	add := func(key string, hit bool, weight int) {
		if hit && weight > 0 {
			d.Signals[key] = weight
			d.Score += weight
		}
	}
	add(RuleFirstHigh, f.NewContact && f.NewIP && f.Amount >= limits.FirstAmount*100, p.Weights[RuleFirstHigh])
	add(RuleSlowPayment, f.PaymentDelay >= time.Duration(limits.SlowSeconds)*time.Second, p.Weights[RuleSlowPayment])
	add(RuleDeviceSwitch, f.DeviceSwitch, p.Weights[RuleDeviceSwitch])
	probeWeight := p.Weights[RuleUnpaidModerate]
	if f.Unpaid30m == 1 {
		probeWeight = probeWeight * 15 / 35
	}
	add(RuleUnpaidModerate, f.Unpaid30m > 0, probeWeight)
	add(RuleUnredeemed, f.UnredeemedHighOrder, p.Weights[RuleUnredeemed])
	add(RuleSubnetBurst, f.SubnetBurst, p.Weights[RuleSubnetBurst])
	add(RuleSharedIP, f.DifferentContactSameIP, p.Weights[RuleSharedIP])
	add(RuleDisposable, f.RiskyEmail, p.Weights[RuleDisposable])
	d.Hold = len(d.HardRules) > 0 || p.Threshold > 0 && d.Score >= p.Threshold
	return d
}

// NormalizeEmail is a risk key only. It must not be used for authentication,
// delivery, or deciding that two accounts belong to the same person.
func NormalizeEmail(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if len(raw) > 254 {
		return ""
	}
	local, host, ok := strings.Cut(raw, "@")
	if !ok || local == "" || host == "" || strings.Contains(host, "@") {
		return ""
	}
	local, _, _ = strings.Cut(local, "+")
	if host == "gmail.com" || host == "googlemail.com" {
		local = strings.ReplaceAll(local, ".", "")
		host = "gmail.com"
	}
	if local == "" {
		return ""
	}
	return local + "@" + host
}

// SimilarEmail compares only bounded local parts within the same domain.
// The caller scopes candidates to a recent, related set of orders.
func SimilarEmail(a, b string) bool {
	a, b = NormalizeEmail(a), NormalizeEmail(b)
	if a == "" || b == "" || a == b {
		return false
	}
	left, host, _ := strings.Cut(a, "@")
	right, otherHost, _ := strings.Cut(b, "@")
	if host != otherHost {
		return false
	}
	x, y := []rune(left), []rune(right)
	const cutoff = 2
	if len(x)-len(y) > cutoff || len(y)-len(x) > cutoff {
		return false
	}
	prev, next := make([]int, len(y)+1), make([]int, len(y)+1)
	for j := range prev {
		prev[j] = j
	}
	for i, ch := range x {
		next[0] = i + 1
		lo, hi := max(1, i+1-cutoff), min(len(y), i+1+cutoff)
		if lo > 1 {
			next[lo-1] = cutoff + 1
		}
		best := next[0]
		for j := lo; j <= hi; j++ {
			cost := 0
			if ch != y[j-1] {
				cost = 1
			}
			next[j] = min(next[j-1]+1, prev[j]+1, prev[j-1]+cost)
			best = min(best, next[j])
		}
		if best > cutoff {
			return false
		}
		if hi < len(y) {
			next[hi+1] = cutoff + 1
		}
		prev, next = next, prev
	}
	return prev[len(y)] <= cutoff
}

// RedemptionEvidence records a gateway observation without storing the code or Session.
type RedemptionEvidence struct {
	EventID     *string   `gorm:"type:varchar(36);uniqueIndex" json:"-"`
	ID          uint      `gorm:"primarykey" json:"id"`
	CodeHash    string    `gorm:"type:char(64);index;not null" json:"code_hash"`
	Operation   string    `gorm:"type:varchar(16);not null" json:"operation"`
	Outcome     string    `gorm:"type:varchar(40);not null" json:"outcome"`
	ClientIP    string    `gorm:"type:varchar(45);not null" json:"client_ip"`
	UserAgent   string    `gorm:"type:text" json:"user_agent"`
	TargetEmail string    `gorm:"type:varchar(254)" json:"target_email,omitempty"`
	CreatedAt   time.Time `gorm:"index;not null" json:"created_at"`
}

func (RedemptionEvidence) TableName() string { return "order_risk_redemption_evidence" }

// RedemptionPayloadHashes maps each standalone code to its audit identifier.
func RedemptionPayloadHashes(payload string) []string {
	var hashes []string
	for _, line := range strings.Split(payload, "\n") {
		code := strings.TrimSpace(line)
		if len(code) < 24 || len(code) > 80 || strings.IndexFunc(code, func(r rune) bool {
			return !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-')
		}) >= 0 {
			continue
		}
		digest := sha256.Sum256([]byte(code))
		hashes = append(hashes, hex.EncodeToString(digest[:]))
	}
	return hashes
}
