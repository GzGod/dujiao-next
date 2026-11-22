package buyinghelp

import (
	"strings"
	"testing"
)

func TestPublicDocumentsCompleteAndConnected(t *testing.T) {
	if len(Paths()) != 6 {
		t.Fatal("missing topics")
	}
	for _, path := range Paths() {
		for _, lang := range []string{"zh-CN", "zh-TW", "en-US"} {
			d, ok := Get(path, lang)
			if !ok || d.Title == "" || d.Description == "" {
				t.Fatalf("incomplete %s %s", path, lang)
			}
			if path != "/guides" && len(d.Sections) < 5 {
				t.Fatal("thin topic")
			}
			for _, s := range d.Sections {
				if s.Question == "" || s.Answer == "" {
					t.Fatal("empty answer")
				}
			}
			h := HTML(path, lang)
			if !strings.Contains(h, `href="/products"`) || !strings.Contains(h, "/guides/redeem") {
				t.Fatal("dead-end page")
			}
			if strings.Contains(h, "<script") {
				t.Fatal("unexpected executable content")
			}
		}
	}
	if Has("/orders/private") || Has("/guides/missing") {
		t.Fatal("unregistered route included")
	}
}
func TestReturnedCopyAndLanguageFallback(t *testing.T) {
	d, _ := Get("/guides/gpt-top-up", "en-US")
	d.Sections[0].Answer = "changed"
	next, _ := Get("/guides/gpt-top-up", "en-US")
	if next.Sections[0].Answer == "changed" {
		t.Fatal("shared content mutated")
	}
	fallback, _ := Get("/guides", "unknown")
	if fallback.Title == "" {
		t.Fatal("fallback absent")
	}
}
