// Package buyinghelp provides public, user-accessible decision guides. The same
// documents are served to people and crawlers; no User-Agent variants are used.
package buyinghelp

import (
	_ "embed"
	"encoding/json"
	"html"
	"net/url"
	"sort"
	"strings"
)

//go:embed content.json
var raw []byte

type Section struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}
type Document struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Sections    []Section `json:"sections"`
}

var content = func() struct {
	Pages  map[string]map[string]Document `json:"pages"`
	Labels map[string]map[string]string   `json:"labels"`
} { var c struct {
	Pages  map[string]map[string]Document `json:"pages"`
	Labels map[string]map[string]string   `json:"labels"`
}; if err := json.Unmarshal(raw, &c); err != nil {
	panic(err)
}; return c }()

func Paths() []string {
	paths := []string{"/guides"}
	for p := range content.Pages {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}
func Has(path string) bool {
	if path == "/guides" {
		return true
	}
	_, ok := content.Pages[path]
	return ok
}
func locale(lang string) string {
	if _, ok := content.Labels[lang]; !ok {
		return "zh-CN"
	}
	return lang
}
func Get(path, lang string) (Document, bool) {
	lang = locale(lang)
	if path == "/guides" {
		l := content.Labels[lang]
		return Document{Title: l["indexTitle"], Description: l["indexDescription"]}, true
	}
	m, ok := content.Pages[path]
	if !ok {
		return Document{}, false
	}
	d := m[lang]
	d.Sections = append([]Section{}, d.Sections...)
	return d, true
}
func anchor(path, label, lang string) string {
	if strings.HasPrefix(path, "/guides") && lang != "zh-CN" {
		path += "?lang=" + url.QueryEscape(lang)
	}
	return `<a href="` + html.EscapeString(path) + `">` + html.EscapeString(label) + `</a>`
}
func IndexLink(lang string) string {
	lang = locale(lang)
	return anchor("/guides", content.Labels[lang]["indexTitle"], lang)
}

// HTML uses escaped editorial text and fixed public links only. It never reads
// query credentials, orders, codes, sessions, customer data, prices or stock.
func HTML(path, lang string) string {
	lang = locale(lang)
	d, ok := Get(path, lang)
	if !ok {
		return ""
	}
	l := content.Labels[lang]
	esc := html.EscapeString
	var b strings.Builder
	b.WriteString(`<nav aria-label="Language"><a href="?lang=zh-CN">简体中文</a> · <a href="?lang=zh-TW">繁體中文</a> · <a href="?lang=en-US">English</a></nav><h1>` + esc(d.Title) + "</h1><p>" + esc(d.Description) + "</p><small>" + esc(l["updated"]) + "</small>")
	for _, s := range d.Sections {
		b.WriteString("<section><h2>" + esc(s.Question) + "</h2><p>" + esc(s.Answer) + "</p></section>")
	}
	b.WriteString("<h2>" + esc(l["related"]) + "</h2><ul>")
	for _, p := range Paths() {
		if p == path {
			continue
		}
		doc, _ := Get(p, lang)
		b.WriteString("<li>" + anchor(p, doc.Title, lang) + "</li>")
	}
	b.WriteString("<li>" + anchor("/guides/redeem", l["redeem"], lang) + "</li></ul><p>" + anchor("/products", l["products"], lang) + " · " + anchor("https://t.me/xuegaozhanshen", l["support"], lang) + "</p>")
	b.WriteString("<h2>" + esc(l["sources"]) + "</h2><ul>")
	for _, s := range []struct{ URL, Title string }{{"https://help.openai.com/en/articles/6950777-what-is-chatgpt-plus", "OpenAI · ChatGPT Plus"}, {"https://help.openai.com/en/articles/9039756-billing-settings-in-chatgpt-vs-platform", "OpenAI · ChatGPT / API billing"}, {"https://help.openai.com/en/articles/10421635-multicurrency-billing", "OpenAI · Currencies and payment methods"}} {
		b.WriteString("<li>" + anchor(s.URL, s.Title, lang) + "</li>")
	}
	b.WriteString("</ul>")
	return b.String()
}
