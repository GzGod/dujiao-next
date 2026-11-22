package storefrontseo

import (
	"encoding/json"
	"html"
	"net/url"
	"strings"

	"github.com/dujiao-next/internal/htmltext"
	reseller "github.com/dujiao-next/internal/modules/reseller/contract"
	"github.com/dujiao-next/internal/web"
	"github.com/gin-gonic/gin"
)

type object map[string]interface{}

func text(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	if m, ok := v.(map[string]interface{}); ok {
		if s, ok := m["zh-CN"].(string); ok {
			return s
		}
	}
	return ""
}
func plain(v interface{}) string { return strings.TrimSpace(htmltext.StripToPlainText(text(v))) }
func esc(s string) string        { return html.EscapeString(s) }
func obj(v interface{}) object {
	if m, ok := v.(map[string]interface{}); ok {
		return object(m)
	}
	return object{}
}
func link(path, label string) string { return `<a href="` + esc(path) + `">` + esc(label) + `</a>` }

// New returns an HTML projection for anonymous, public pages only. It deliberately
// does not use User-Agent sniffing or render protected order/account routes.
func New(config, products, product, post gin.HandlerFunc) web.PageRenderer {
	readConfig, readProducts, readProduct, readPost := newReader(config), newReader(products), newReader(product), newReader(post)
	return func(c *gin.Context) (*web.Page, error) {
		lang := c.Query("lang")
		if lang != "en-US" && lang != "zh-TW" {
			lang = "zh-CN"
		}
		tr := func(key string) string { return messages[lang][key] }
		text := func(v interface{}) string { return localized(v, lang) }
		plain := func(v interface{}) string { return strings.TrimSpace(htmltext.StripToPlainText(text(v))) }
		tenant := reseller.TenantOrMain(c.Request.Context())
		if tenant.Unavailable {
			return &web.Page{Status: 404, Language: lang, Title: tr("m0")}, nil
		}
		path := c.Request.URL.Path
		detail := strings.HasPrefix(path, "/products/") && strings.Count(path, "/") == 2
		article := strings.HasPrefix(path, "/blog/") && strings.Count(path, "/") == 2
		guide := path == "/guides/redeem"
		if path != "/" && path != "/products" && !detail && !article && !guide {
			return nil, nil
		}
		raw, _, err := readConfig(c, "")
		if err != nil {
			return nil, err
		}
		var cfg object
		if err = json.Unmarshal(raw, &cfg); err != nil {
			return nil, err
		}
		brand := obj(cfg["brand"])
		seo := obj(cfg["seo"])
		name := text(brand["site_name"])
		base := strings.TrimRight(text(brand["site_url"]), "/")
		if !tenant.IsMain && tenant.PrimaryDomain != "" {
			base = "https://" + tenant.PrimaryDomain
		}
		if base == "" {
			return nil, nil
		} // Never construct canonical URLs from untrusted Host headers.
		page := &web.Page{Language: lang, Canonical: base + path, Title: name, Description: plain(seo["description"])}
		content := ""
		if guide {
			page.Title = tr("m7") + name
			page.Description = tr("m8")
			page.Standalone = true
			content = guideContent(cfg, lang, tenant.IsMain)
		} else if detail || article {
			handler := readProduct
			if article {
				handler = readPost
			}
			slug := strings.TrimPrefix(path, "/products/")
			if article {
				slug = strings.TrimPrefix(path, "/blog/")
			}
			raw, status, e := handler(c, slug)
			if e != nil {
				return nil, e
			}
			if status == 404 {
				page.Status = 404
				page.Title = tr("m0") + " - " + name
				content = "<h1>" + tr("m0") + "</h1>"
			} else {
				var item object
				if e = json.Unmarshal(raw, &item); e != nil {
					return nil, e
				}
				title := plain(item["title"])
				page.Title = title + " - " + name
				page.Description = plain(item["description"])
				if article {
					page.Description = plain(item["summary"])
				}
				if page.Description == "" {
					page.Description = plain(obj(item["seo_meta"])["description"])
				}
				if page.Description == "" {
					page.Description = plain(item["content"])
					r := []rune(page.Description)
					if len(r) > 160 {
						page.Description = string(r[:160])
					}
				}
				content = "<h1>" + esc(title) + "</h1><p>" + esc(page.Description) + "</p>"
				if detail {
					price := text(item["price_amount"])
					currency := text(cfg["currency"])
					if currency == "" {
						currency = "CNY"
					}
					content += "<p>" + esc(currency+" "+price) + "</p>"
					availability := "https://schema.org/InStock"
					if item["is_sold_out"] == true || text(item["stock_status"]) == "out_of_stock" {
						availability = "https://schema.org/OutOfStock"
					}
					data := object{"@context": "https://schema.org", "@type": "Product", "name": title, "description": page.Description, "url": page.Canonical, "offers": object{"@type": "Offer", "price": price, "priceCurrency": currency, "availability": availability, "url": page.Canonical}}
					b, _ := json.Marshal(data)
					page.JSONLD = string(b)
				}
				content += "<section style=\"white-space:pre-line\">" + esc(plain(item["content"])) + "</section>"
				content += "<p>" + link("/guides/redeem", tr("m1")) + "</p>"
			}
		} else {
			page.Title = plain(seo["title"])
			if page.Title == "" {
				page.Title = tr("homeTitle") + " - " + name
			}
			if page.Description == "" {
				page.Description = tr("homeDescription")
			}
			content = "<h1>" + esc(page.Title) + "</h1><p>" + esc(page.Description) + "</p>"
			raw, _, e := readProducts(c, "")
			if e != nil {
				return nil, e
			}
			var items []object
			if e = json.Unmarshal(raw, &items); e != nil {
				return nil, e
			}
			content += "<h2>" + tr("m2") + "</h2><ul>"
			for _, item := range items {
				content += "<li>" + link("/products/"+url.PathEscape(text(item["slug"])), plain(item["title"])) + " — " + esc(text(item["price_amount"])) + "</li>"
			}
			content += "</ul><h2>" + tr("m3") + "</h2><p>" + tr("m4") + "</p><p>" + link("/guides/redeem", tr("m5")) + "</p>"
		}
		page.Content = `<main class="seo-document"><nav>` + link("/", name) + " · " + link("/products", tr("m6")) + `</nav>` + content + `</main><style>.seo-document{max-width:900px;margin:40px auto;padding:24px;font:16px/1.8 system-ui;color:#202124;background:#fff;border-radius:20px}.dark .seo-document{background:#0b0b0b;color:#eee}.dark .seo-document a{color:#86b7ff}.seo-document h1{font-size:30px;line-height:1.4}.seo-document h2{margin-top:30px}.seo-document a{color:#1764c0;text-decoration:underline}.seo-document li{margin:12px 0}</style>`
		return page, nil
	}
}
