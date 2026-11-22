// Package storefrontseo composes indexable HTML from the same anonymous public
// handlers as the storefront, preserving tenant pricing and listing rules.
package storefrontseo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
)

type captureWriter struct {
	http.ResponseWriter
	body    bytes.Buffer
	headers http.Header
	status  int
}

func (w *captureWriter) Header() http.Header         { return w.headers }
func (w *captureWriter) WriteHeader(s int)           { w.status = s }
func (w *captureWriter) Write(b []byte) (int, error) { return w.body.Write(b) }

// publicData invokes a public read handler without outbound HTTP or caller
// credentials. Only the request context (including resolved tenant) is retained.
func newReader(handler gin.HandlerFunc) func(*gin.Context, string) (json.RawMessage, int, error) {
	engine := gin.New()
	engine.GET("/", handler)
	engine.GET("/:slug", handler)
	return func(parent *gin.Context, slug string) (json.RawMessage, int, error) {
		w := &captureWriter{headers: make(http.Header), status: 200}
		request := parent.Request.Clone(parent.Request.Context())
		request.Method = http.MethodGet
		request.Body = nil
		request.GetBody = nil
		request.ContentLength = 0
		request.Trailer = nil
		query := url.Values{"page": {"1"}, "page_size": {"100"}}
		for _, key := range []string{"category_id", "type"} {
			if value := parent.Query(key); value != "" {
				query.Set(key, value)
			}
		}
		request.URL = &url.URL{Path: "/" + slug, RawQuery: query.Encode()}
		request.Header = make(http.Header)
		request.Header.Set("Accept-Language", "zh-CN")
		engine.ServeHTTP(w, request)
		var envelope struct {
			Status int             `json:"status_code"`
			Data   json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(w.body.Bytes(), &envelope); err != nil {
			return nil, 0, err
		}
		if envelope.Status != 0 && envelope.Status != 200 {
			if envelope.Status == 404 {
				return nil, 404, nil
			}
			return nil, 0, fmt.Errorf("public SEO projection failed: %d", envelope.Status)
		}
		return envelope.Data, 200, nil
	}

}
