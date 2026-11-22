package productguide

import (
	"strings"
	"testing"
)

func TestEveryListedProductHasCompleteLocalizedBuyingGuidance(t *testing.T) {
	slugs := []string{"upstream-1-18-1793979792290", "upstream-1-3-1793979782658", "upstream-1-4-1793979782822", "upstream-1-5-1793979782952", "upstream-1-12-1793979783532", "claude-pro-us-account-with-ip", "claude-pro-us-account-no-ip"}
	for _, slug := range slugs {
		for _, lang := range []string{"zh-CN", "zh-TW", "en-US"} {
			g, ok := Get(slug, lang)
			if !ok || g.Summary == "" || g.Suitable == "" || len(g.Before) < 4 || len(g.Steps) != 4 || len(g.FAQs) < 4 {
				t.Errorf("incomplete guide %s %s", slug, lang)
			}
			for _, s := range g.Steps {
				if s.Title == "" || s.Text == "" {
					t.Errorf("empty step %s %s", slug, lang)
				}
			}
			for _, q := range g.FAQs {
				if q.Q == "" || q.A == "" {
					t.Errorf("empty FAQ %s %s", slug, lang)
				}
			}
			rendered := HTML(g, lang)
			if strings.Contains(slug, "claude") && strings.Contains(rendered, `href="https://redeem.xuegaogx.cc/"`) {
				t.Error("Claude incorrectly sent to GPT redemption")
			}
			if g.Kind == "gpt" && !strings.Contains(rendered, `href="/guides/redeem"`) {
				t.Error("GPT guide entry missing")
			}
			for _, phrase := range []string{"会员即刻生效", "guaranteed instant activation"} {
				if strings.Contains(rendered, phrase) {
					t.Errorf("unverified guarantee %s", phrase)
				}
			}
		}
	}
	if _, ok := Get("unknown-product", "en-US"); ok {
		t.Fatal("guidance inferred for unknown product")
	}
}

func TestCallerCannotMutateSharedGuidance(t *testing.T) {
	slug := "upstream-1-3-1793979782658"
	g, _ := Get(slug, "zh-CN")
	g.Before[0] = "modified"
	g.Steps[0].Title = "modified"
	g.FAQs[0].Q = "modified"
	next, _ := Get(slug, "zh-CN")
	if next.Before[0] == "modified" || next.Steps[0].Title == "modified" || next.FAQs[0].Q == "modified" {
		t.Fatal("public guidance mutated across requests")
	}
}

func TestReadableURLsKeepExistingProductIdentity(t *testing.T) {
	source := "upstream-1-12-1793979783532"
	canonical := CanonicalSlug(source)
	if canonical != "gpt-pro-500-top-up" || SourceSlug(canonical) != source {
		t.Fatal("product identity changed")
	}
	a, _ := Get(source, "en-US")
	b, ok := Get(canonical, "en-US")
	if !ok || a.Summary != b.Summary {
		t.Fatal("alias has different guidance")
	}
	if SourceSlug("not-a-known-product") != "not-a-known-product" {
		t.Fatal("unknown slug remapped")
	}
}
