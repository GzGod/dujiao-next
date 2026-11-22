package storefrontseo

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	reseller "github.com/dujiao-next/internal/modules/reseller/contract"
	"github.com/gin-gonic/gin"
)

func TestProjectionUsesAnonymousTenantScopedPublicReads(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config := func(c *gin.Context) {
		c.JSON(200, gin.H{"status_code": 0, "data": gin.H{"brand": gin.H{"site_name": "Shop", "site_url": "https://main.test"}, "currency": "CNY"}})
	}
	catalog := func(c *gin.Context) {
		tenant := reseller.TenantOrMain(c.Request.Context())
		if tenant.ResellerID == nil || *tenant.ResellerID != 7 {
			t.Error("tenant context lost")
		}
		if c.GetHeader("Authorization") != "" || c.GetHeader("Cookie") != "" {
			t.Error("caller credentials forwarded")
		}
		c.JSON(200, gin.H{"status_code": 0, "data": []gin.H{{"slug": "plus", "title": gin.H{"zh-CN": "Public plan"}, "price_amount": "119.00"}}})
	}
	detail := func(c *gin.Context) {
		if c.Param("slug") == "missing" {
			c.JSON(200, gin.H{"status_code": 404})
			return
		}
		c.JSON(200, gin.H{"status_code": 0, "data": gin.H{"title": gin.H{"zh-CN": "<script>alert(1)</script>"}, "description": gin.H{"zh-CN": "Public description"}, "content": gin.H{"zh-CN": "<p>Details</p>"}, "price_amount": "119.00", "is_sold_out": true}})
	}
	renderer := New(config, catalog, detail, detail)
	for _, path := range []string{"/", "/products/plus", "/products/missing", "/guides/redeem", "/orders/private"} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", path, nil)
		c.Request.Header.Set("Authorization", "Bearer sensitive")
		c.Request.Header.Set("Cookie", "session=sensitive")
		c.Request = c.Request.WithContext(reseller.WithTenantContext(c.Request.Context(), reseller.ResellerTenantContext("sub.test", 7, 9, "sub.test")))
		page, err := renderer(c)
		if err != nil {
			t.Fatal(err)
		}
		if path == "/orders/private" {
			if page != nil {
				t.Error("private route projected")
			}
			continue
		}
		if page.Canonical != "https://sub.test"+path {
			t.Errorf("wrong tenant canonical: %s", page.Canonical)
		}
		if strings.Contains(page.Content, "<script>") {
			t.Error("unsafe public content")
		}
		if path == "/products/missing" && page.Status != 404 {
			t.Error("missing product is soft 404")
		}
		if path == "/products/plus" && !strings.Contains(page.JSONLD, "OutOfStock") {
			t.Error("stock incorrectly projected")
		}
	}
}

func TestProjectionFailureIsNotAnEmptyIndexablePage(t *testing.T) {
	failed := func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status_code": 500}) }
	renderer := New(failed, failed, failed, failed)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	if _, err := renderer(c); err == nil {
		t.Fatal("public read failure swallowed")
	}
}
