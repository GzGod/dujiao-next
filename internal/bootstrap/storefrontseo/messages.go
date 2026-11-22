package storefrontseo

import (
	"embed"
	"encoding/json"
	"github.com/dujiao-next/internal/shared/buyinghelp"
	"strings"
)

//go:embed messages.json
var messageFiles embed.FS
var messages = func() map[string]map[string]string {
	b, err := messageFiles.ReadFile("messages.json")
	if err != nil {
		panic(err)
	}
	var m map[string]map[string]string
	if err = json.Unmarshal(b, &m); err != nil {
		panic(err)
	}
	return m
}()

func localized(v interface{}, lang string) string {
	if m, ok := v.(map[string]interface{}); ok {
		if value, ok := m[lang].(string); ok && value != "" {
			return value
		}
	}
	return text(v)
}

func guideContent(cfg object, lang string, main bool) string {
	contact := obj(cfg["contact"])
	support := text(contact["telegram"])
	supportLink := ""
	if strings.HasPrefix(support, "https://t.me/") || strings.HasPrefix(support, "https://telegram.me/") {
		supportLink = link(support, messages[lang]["support"])
	}
	redeemLink := ""
	if main && strings.TrimRight(text(obj(cfg["brand"])["site_url"]), "/") == "https://xuegaogx.cc" {
		if supportLink == "" {
			supportLink = link("https://t.me/xuegaozhanshen", messages[lang]["support"])
		}
		redeemLink = "<p>" + link("https://redeem.xuegaogx.cc/", messages[lang]["redeem"]) + " · " + link("https://redeem.xuegaogx.cc/progress", messages[lang]["progress"]) + "</p>"
	}
	helpLink := ""
	if main && strings.TrimRight(text(obj(cfg["brand"])["site_url"]), "/") == "https://xuegaogx.cc" {
		helpLink = "<p>" + buyinghelp.IndexLink(lang) + "</p>"
	}
	return helpLink + `<p><a href="?lang=zh-CN">简体中文</a> · <a href="?lang=zh-TW">繁體中文</a> · <a href="?lang=en-US">English</a></p>` + redeemLink + messages[lang]["guide"] + link("/guest/orders", messages[lang]["guest"]) + " · " + link("/me/orders", messages[lang]["orders"]) + " · " + supportLink + "</p>"
}
