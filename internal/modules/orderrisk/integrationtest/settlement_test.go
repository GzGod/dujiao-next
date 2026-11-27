package integrationtest

import (
	riskcontract "github.com/dujiao-next/internal/modules/orderrisk/contract"
	risk "github.com/dujiao-next/internal/modules/orderrisk/domain"
	settingssecurity "github.com/dujiao-next/internal/modules/settings/schema/security"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestEvaluateSettlement(t *testing.T) {
	high := risk.SettlementFacts{Amount: 25000, NewContact: true, NewIP: true}
	slow := high
	slow.PaymentDelay = 150 * time.Second
	established := slow
	established.CustomerAge = 7 * 24 * time.Hour
	established.HistoricalSuccessfulOrders = 2
	funded := established
	funded.RecentMonitoredWalletFunding = true
	blacklisted := established
	blacklisted.Blacklisted = true
	for _, c := range []struct {
		name         string
		facts        risk.SettlementFacts
		hold, exempt bool
	}{
		{"normal high first payment", high, false, false},
		{"slow high first payment", slow, true, false},
		{"established customer", established, false, true},
		{"recent wallet funding removes exemption", funded, true, false},
		{"blacklist overrides exemption", blacklisted, true, false},
		{"rapid second payment", risk.SettlementFacts{Amount: 15000, PaidCount30m: 1}, true, false},
		{"contact variant", risk.SettlementFacts{ApproximateContact: true}, true, false},
		{"unpaid burst", risk.SettlementFacts{Unpaid30m: 3}, true, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := risk.EvaluateSettlement(c.facts, risk.DefaultSettlementPolicy())
			if got.Hold != c.hold || got.Exempt != c.exempt {
				t.Fatalf("unexpected decision: %+v", got)
			}
		})
	}
}

func TestRiskEmailVariants(t *testing.T) {
	if got := risk.NormalizeEmail(" A.B+tag@GoogleMail.com "); got != "ab@gmail.com" {
		t.Fatalf("unexpected normalization: %q", got)
	}
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"test_01@qq.com", "test02@qq.com", true},
		{"abc@example.com", "abcdef@example.com", false},
		{"abcdef@example.com", "abc@example.com", false},
		{"apple@example.com", "apply@other.example", false},
		{"a.b+tag@gmail.com", "ab@gmail.com", false},
	} {
		if got := risk.SimilarEmail(c.a, c.b); got != c.want || risk.SimilarEmail(c.b, c.a) != c.want {
			t.Errorf("unexpected similarity for %q and %q: %v", c.a, c.b, got)
		}
	}
}

func TestSettlementConfigurationCanDisableAWeightedSignal(t *testing.T) {
	cfg := settingssecurity.DefaultOrderRiskControlConfig()
	cfg.SettlementWeights = map[string]int{risk.RuleSlowPayment: 0}
	facts := riskcontract.SettlementFacts{Amount: 25000, NewContact: true, NewIP: true, PaymentDelay: 150 * time.Second}
	got := riskcontract.EvaluateSettlement(facts, cfg)
	if got.Hold || got.Score != 25 {
		t.Fatalf("zero-weight signal should not contribute: %+v", got)
	}
	cfg.SettlementThreshold = 25
	if got := riskcontract.EvaluateSettlement(facts, cfg); !got.Hold {
		t.Fatalf("threshold override not applied: %+v", got)
	}
}

func TestRiskEmailDomainMatchesOnlyConfiguredHost(t *testing.T) {
	cfg := settingssecurity.DefaultOrderRiskControlConfig()
	cfg.RiskyEmailDomains = []string{" Mail.Example. ", "invalid host", "mail.example"}
	cfg = settingssecurity.NormalizeOrderRiskControlConfig(cfg)
	if len(cfg.RiskyEmailDomains) != 1 || !riskcontract.SettlementRiskyEmail("guest+tag@MAIL.example", cfg) {
		t.Fatalf("configured domain did not match: %v", cfg.RiskyEmailDomains)
	}
	if riskcontract.SettlementRiskyEmail("guest@notmail.example", cfg) {
		t.Fatal("a similar domain must not match")
	}
}

func TestSettlementDenyOverridesAllowAndEmailCannotExempt(t *testing.T) {
	cfg := settingssecurity.DefaultOrderRiskControlConfig()
	cfg.Enabled = true
	now := time.Now()
	expired := now.Add(-time.Minute)
	cfg.Common.SettlementList = []settingssecurity.SettlementListEntry{
		{Kind: "user_id", Value: "7", Allow: true},
		{Kind: "contact_norm", Value: "a.b@gmail.com", Allow: true},
		{Kind: "cidr", Value: "192.0.2.0/24"},
		{Kind: "user_id", Value: "7", ExpiresAt: &expired},
	}
	denied, allowed := riskcontract.MatchSettlementList(riskcontract.CheckInput{UserID: 7, ClientIP: "192.0.2.8"}, cfg, now)
	if !denied || allowed {
		t.Fatal("network deny must override account allow")
	}
	denied, allowed = riskcontract.MatchSettlementList(riskcontract.CheckInput{UserID: 7}, cfg, now)
	if denied || !allowed {
		t.Fatal("expired deny must not block active account allow")
	}
	denied, allowed = riskcontract.MatchSettlementList(riskcontract.CheckInput{GuestEmail: "ab+tag@gmail.com"}, cfg, now)
	if denied || allowed {
		t.Fatal("guest email must not grant an exemption")
	}
}

func TestSettlementLocksGroupAliasesAndNetworkNeighbors(t *testing.T) {
	hash := riskcontract.GuestContactHash("A.B+first@googlemail.com")
	if len(hash) != 64 || hash != riskcontract.GuestContactHash("ab+second@gmail.com") ||
		hash == riskcontract.GuestContactHash("another@gmail.com") {
		t.Fatal("risk index did not group only the equivalent mailbox aliases")
	}
	one := riskcontract.SettlementLockKeys(riskcontract.CheckInput{
		GuestEmail: "A.B+first@googlemail.com", ClientIP: "192.0.2.10",
	})
	two := riskcontract.SettlementLockKeys(riskcontract.CheckInput{
		GuestEmail: "ab+second@gmail.com", ClientIP: "192.0.2.250",
	})
	if len(one) != 2 || !slices.Equal(one, two) {
		t.Fatalf("expected guest aliases and subnet neighbors to share both lock keys: %v / %v", one, two)
	}
}

func TestGuestAndMemberSettlementShareDeviceLock(t *testing.T) {
	device := "c91abb3b821a044f699b98e191265bc75510aa8d1242b8da3cb25aa6b4feebd3"
	guest := riskcontract.SettlementLockKeys(riskcontract.CheckInput{
		GuestEmail: "guest@example.com", RiskDeviceHash: device, ClientIP: "192.0.2.5",
	})
	member := riskcontract.SettlementLockKeys(riskcontract.CheckInput{
		UserID: 42, RiskDeviceHash: device, ClientIP: "203.0.113.10",
	})
	shared := "settlement:device:" + device
	if !slices.Contains(guest, shared) || !slices.Contains(member, shared) {
		t.Fatalf("settlements on the same device must serialize: %v / %v", guest, member)
	}
}

func TestNetworkAllowCanExemptButCannotOverrideDeny(t *testing.T) {
	cfg := settingssecurity.DefaultOrderRiskControlConfig()
	cfg.Enabled = true
	cfg.Common.SettlementList = []settingssecurity.SettlementListEntry{
		{Kind: "cidr", Value: "192.0.2.17/24", Allow: true},
		{Kind: "ip", Value: "192.0.2.7"},
		{Kind: "ip", Value: "2001:db8::1", Allow: true},
	}
	cfg = settingssecurity.NormalizeOrderRiskControlConfig(cfg)
	for _, tc := range []struct {
		ip                string
		skip, deny, allow bool
	}{
		{ip: "192.0.2.8", allow: true},
		{ip: "192.0.2.7", deny: true},
		{ip: "2001:db8::1", allow: true},
		{ip: "192.0.2.8", skip: true},
		{ip: "198.51.100.1"},
	} {
		deny, allow := riskcontract.MatchSettlementList(riskcontract.CheckInput{
			ClientIP: tc.ip, SkipIPCheck: tc.skip,
		}, cfg, time.Now())
		if deny != tc.deny || allow != tc.allow {
			t.Fatalf("unexpected list decision for %s: deny=%v allow=%v", tc.ip, deny, allow)
		}
	}
}

func TestObviousGeneratedMailboxOnlyContributesSoftSignal(t *testing.T) {
	cfg := settingssecurity.DefaultOrderRiskControlConfig()
	if !riskcontract.SettlementRiskyEmail("q3rt7wx9k4q6p2v8z5mn3t7s@example.com", cfg) {
		t.Fatal("obvious generated mailbox was missed")
	}
	for _, email := range []string{
		"5776012@qq.com",
		"customer-support-renewals-2026@example.com",
		"customer+q3rt7wx9k4q6p2v8z5mn3t7s@example.com",
		"123456789012345678901234@example.com",
	} {
		if riskcontract.SettlementRiskyEmail(email, cfg) {
			t.Fatalf("expected an ordinary mailbox to pass: %s", email)
		}
	}
	decision := riskcontract.EvaluateSettlement(riskcontract.SettlementFacts{RiskyEmail: true}, cfg)
	if decision.Hold || decision.Score != 25 {
		t.Fatalf("the mailbox signal alone must not hold an order: %+v", decision)
	}
}

func TestSettlementAuditContextNormalizesSubnetAndBoundsUserAgent(t *testing.T) {
	for input, expected := range map[string]string{
		"::ffff:192.0.2.91":     "192.0.2.0/24",
		"2001:db8:1:2:abcd::37": "2001:db8:1:2::/64",
		"invalid":               "",
	} {
		if got := riskcontract.SettlementSubnet(input); got != expected {
			t.Fatalf("subnet(%q) = %q, want %q", input, got, expected)
		}
	}
	agent := riskcontract.SettlementUserAgent("  client\x00\n" + strings.Repeat("a", 600))
	if len(agent) != 512 || !strings.HasPrefix(agent, "clienta") {
		t.Fatal("user agent was not sanitized and bounded")
	}
}

func TestSettlementVelocityThresholds(t *testing.T) {
	for _, tc := range []struct {
		name  string
		facts risk.SettlementFacts
		hard  string
	}{
		{"first payment above daily threshold", risk.SettlementFacts{Amount: 40000}, ""},
		{"second payment one fen below short thresholds", risk.SettlementFacts{Amount: 14999, PaidCount30m: 1, PaidAmount30m: 15000, PaidCount24h: 1, PaidAmount24h: 15000}, ""},
		{"second payment reaches short cumulative threshold", risk.SettlementFacts{Amount: 14999, PaidCount30m: 1, PaidAmount30m: 15001, PaidCount24h: 1, PaidAmount24h: 15001}, "multi_paid_strong_identity"},
		{"second payment reaches daily cumulative threshold", risk.SettlementFacts{Amount: 12000, PaidCount24h: 1, PaidAmount24h: 28000}, "multi_paid_strong_identity"},
		{"third payment in day", risk.SettlementFacts{Amount: 100, PaidCount24h: 2, PaidAmount24h: 200}, "multi_paid_strong_identity"},
		{"three unpaid probes in day alone", risk.SettlementFacts{Unpaid24h: 3}, ""},
		{"four unpaid probes in day", risk.SettlementFacts{Unpaid24h: 4}, "unpaid_probe_burst"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := risk.EvaluateSettlement(tc.facts, risk.DefaultSettlementPolicy())
			var want []string
			if tc.hard != "" {
				want = append(want, tc.hard)
			}
			if got.Hold != (tc.hard != "") || !slices.Equal(got.HardRules, want) {
				t.Fatalf("got hold=%v rules=%v, want rules=%v", got.Hold, got.HardRules, want)
			}
		})
	}
}
