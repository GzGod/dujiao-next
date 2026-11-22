package productguide

import (
	"html"
	"strings"
)

// HTML renders the same guidance visible in the storefront. The output contains
// public editorial text only; it does not include static prices or inventory.
func HTML(g Guide, lang string) string {
	labels := Labels(lang)
	var b strings.Builder
	esc := html.EscapeString
	b.WriteString("<section><h2>" + esc(labels["heading"]) + "</h2><p>" + esc(g.Summary) + "</p><h3>" + esc(labels["suitable"]) + "</h3><p>" + esc(g.Suitable) + "</p><h3>" + esc(labels["before"]) + "</h3><ul>")
	for _, item := range g.Before {
		b.WriteString("<li>" + esc(item) + "</li>")
	}
	b.WriteString("</ul><h3>" + esc(labels["steps"]) + "</h3><ol>")
	for _, step := range g.Steps {
		b.WriteString("<li><strong>" + esc(step.Title) + "</strong><p>" + esc(step.Text) + "</p></li>")
	}
	b.WriteString("</ol><h3>" + esc(labels["faq"]) + "</h3>")
	for _, faq := range g.FAQs {
		b.WriteString("<h4>" + esc(faq.Q) + "</h4><p>" + esc(faq.A) + "</p>")
	}
	if g.Kind == "gpt" {
		b.WriteString(`<p><a href="/guides/redeem">` + esc(labels["guide"]) + `</a> · <a href="https://redeem.xuegaogx.cc/">` + esc(labels["redeem"]) + `</a></p>`)
	}
	b.WriteString(`<p><a href="https://t.me/xuegaozhanshen">` + esc(labels["support"]) + `</a></p></section>`)
	return b.String()
}
