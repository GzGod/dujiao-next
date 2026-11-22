package web

import (
	"bytes"
	"html"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

// Page is a public, request-scoped HTML projection. Never include account data.
type Page struct {
	Title, Description, Canonical, Language, Content string
	JSONLD                                           string
	Status                                           int
	Standalone                                       bool
}

type PageRenderer func(*gin.Context) (*Page, error)

var titleElement = regexp.MustCompile(`(?s)<title>.*?</title>`)
var languageAttribute = regexp.MustCompile(`<html[^>]*>`)
var moduleScript = regexp.MustCompile(`(?s)<script[^>]*type="module"[^>]*>.*?</script>`)

func renderPage(index []byte, page *Page) []byte {
	if page == nil {
		return index
	}
	body := titleElement.ReplaceAllLiteral(index, []byte("<title>"+html.EscapeString(page.Title)+"</title>"))
	body = languageAttribute.ReplaceAllLiteral(body, []byte(`<html lang="`+html.EscapeString(page.Language)+`">`))
	meta := `<meta name="description" content="` + html.EscapeString(page.Description) + `"><link rel="canonical" href="` + html.EscapeString(page.Canonical) + `"><meta property="og:title" content="` + html.EscapeString(page.Title) + `"><meta property="og:description" content="` + html.EscapeString(page.Description) + `"><meta property="og:url" content="` + html.EscapeString(page.Canonical) + `">`
	if page.Status == http.StatusNotFound {
		meta += `<meta name="robots" content="noindex">`
	}
	if page.JSONLD != "" {
		meta += `<script type="application/ld+json">` + strings.ReplaceAll(page.JSONLD, "<", `\u003c`) + `</script>`
	}
	meta = strings.NewReplacer("<meta ", "<meta data-seo-bootstrap ", "<link ", "<link data-seo-bootstrap ", "<script ", "<script data-seo-bootstrap ").Replace(meta)
	body = bytes.Replace(body, []byte("</head>"), []byte(meta+"</head>"), 1)
	body = bytes.Replace(body, []byte(`<div id="app"></div>`), []byte(`<div id="app">`+page.Content+`</div>`), 1)
	if page.Standalone {
		body = moduleScript.ReplaceAll(body, nil)
	}
	return body
}

func serveUserIndex(c *gin.Context, index []byte, renderer PageRenderer) {
	if renderer == nil {
		serveIndex(c, index)
		return
	}
	page, err := renderer(c)
	if err != nil {
		c.Header("Cache-Control", "no-store")
		c.String(http.StatusServiceUnavailable, "Service temporarily unavailable")
		return
	}
	if page == nil {
		serveIndex(c, index)
		return
	}
	status := page.Status
	if status == 0 {
		status = http.StatusOK
	}
	c.Header("Cache-Control", "no-store")
	c.Data(status, "text/html; charset=utf-8", renderPage(index, page))
}
