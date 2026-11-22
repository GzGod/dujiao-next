package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

func TestPublicHTMLProjectionAndMissingStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	index := []byte(`<html lang="en"><head><title></title></head><body><div id="app"></div><script type="module" src="/assets/a.js"></script></body></html>`)
	page := &Page{Title: `Safe <title>`, Description: `A "quote"`, Canonical: "https://shop.test/products/a", Language: "zh-CN", Content: "<h1>Readable content</h1>", JSONLD: `{"name":"</script><script>alert(1)</script>"}`}
	renderer := func(c *gin.Context) (*Page, error) {
		p := *page
		if c.Request.URL.Path == "/products/missing" {
			p.Status = 404
		}
		if c.Request.URL.Path == "/guides/redeem" {
			p.Standalone = true
		}
		return &p, nil
	}
	r := gin.New()
	if err := RegisterUser(r, fstest.MapFS{"index.html": &fstest.MapFile{Data: index}}, renderer); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/products/a", "/products/missing", "/guides/redeem"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		body := w.Body.String()
		for _, want := range []string{`lang="zh-CN"`, "Readable content", "Safe &lt;title&gt;", "data-seo-bootstrap"} {
			if !strings.Contains(body, want) {
				t.Errorf("missing %s", want)
			}
		}
		if strings.Contains(body, `</script><script>alert`) {
			t.Fatal("structured data permits script breakout")
		}
		if path == "/products/missing" && (w.Code != http.StatusNotFound || !strings.Contains(body, "noindex")) {
			t.Fatal("soft 404")
		}
		if path == "/guides/redeem" && strings.Contains(body, `src="/assets/a.js"`) {
			t.Fatal("standalone guide replaced by Vue")
		}
	}
}
