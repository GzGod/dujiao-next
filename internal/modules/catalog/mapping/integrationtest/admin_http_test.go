package integrationtest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mappingdomain "github.com/dujiao-next/internal/modules/catalog/mapping/domain"

	siteconnectiondomain "github.com/dujiao-next/internal/modules/siteconnection/domain"

	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"

	categoryapp "github.com/dujiao-next/internal/modules/catalog/category/application"
	categorydomain "github.com/dujiao-next/internal/modules/catalog/category/domain"

	"github.com/dujiao-next/internal/constants"
	categorygormstore "github.com/dujiao-next/internal/modules/catalog/category/infrastructure/gormstore"
	mappingapp "github.com/dujiao-next/internal/modules/catalog/mapping/application"
	mappingcontract "github.com/dujiao-next/internal/modules/catalog/mapping/contract"
	mappinggormstore "github.com/dujiao-next/internal/modules/catalog/mapping/infrastructure/gormstore"
	mappinghttp "github.com/dujiao-next/internal/modules/catalog/mapping/transport/http"
	productgormstore "github.com/dujiao-next/internal/modules/catalog/product/store/gormstore"
	"github.com/dujiao-next/internal/platform/http/response"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/money"
	"github.com/dujiao-next/internal/upstream"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type adminNoopLocalMediaRecorder struct{}

func (adminNoopLocalMediaRecorder) RecordLocalFile(context.Context, string, string) {}

type mappingConnectionProvider struct {
	db      *gorm.DB
	adapter upstream.Adapter
}

func (p *mappingConnectionProvider) GetByID(id uint) (*siteconnectiondomain.Connection, error) {
	var conn siteconnectiondomain.Connection
	if err := p.db.First(&conn, id).Error; err != nil {
		return nil, err
	}
	return &conn, nil
}

func (p *mappingConnectionProvider) GetAdapter(conn *siteconnectiondomain.Connection) (upstream.Adapter, error) {
	return p.adapter, nil
}

type mappingImportUoW struct {
	products    *productgormstore.ProductStore
	skus        *productgormstore.SKUStore
	mappings    *mappinggormstore.MappingStore
	skuMappings *mappinggormstore.SKUMappingStore
}

func (unit *mappingImportUoW) WithinTransaction(fn func(mappingcontract.ImportRepositories) error) error {
	if fn == nil {
		return nil
	}
	return unit.products.Transaction(func(tx *gorm.DB) error {
		return fn(mappingcontract.ImportRepositories{
			Products:    unit.products.BindTx(tx),
			SKUs:        unit.skus.BindTx(tx),
			Mappings:    unit.mappings.WithTx(tx),
			SKUMappings: unit.skuMappings.WithTx(tx),
		})
	})
}

func setupAdminHandlerTest(t *testing.T, upstreamHandler http.HandlerFunc) (*mappinghttp.AdminHandler, *gorm.DB, uint, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dsn := fmt.Sprintf("file:admin_product_mapping_handler_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
		&categorydomain.Category{},
		&productdomain.Product{},
		&productdomain.ProductSKU{},
		&siteconnectiondomain.Connection{},
		&mappingdomain.Mapping{},
		&mappingdomain.SKUMapping{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	server := httptest.NewServer(upstreamHandler)
	categoryRepo := categorygormstore.NewCategoryStore(db)
	categoryService := categoryapp.NewService(categoryRepo)

	conn := siteconnectiondomain.Connection{
		Name:      "upstream",
		BaseURL:   server.URL,
		ApiKey:    "test-key",
		ApiSecret: "test-secret",
		Protocol:  constants.ConnectionProtocolDujiaoNext,
		Status:    constants.ConnectionStatusActive,
	}
	if err := db.Create(&conn).Error; err != nil {
		t.Fatalf("create connection failed: %v", err)
	}

	adapter, err := upstream.NewAdapter(&conn, t.TempDir())
	if err != nil {
		t.Fatalf("create upstream adapter failed: %v", err)
	}

	productStore := productgormstore.NewProductStore(db)
	skuStore := productgormstore.NewSKUStore(db)
	mappingStore := mappinggormstore.NewMappingStore(db)
	skuMappingStore := mappinggormstore.NewSKUMappingStore(db)

	mappingService, err := mappingapp.NewService(mappingapp.Options{
		Mappings:    mappingStore,
		SKUMappings: skuMappingStore,
		Products:    productStore,
		SKUs:        skuStore,
		Categories:  categoryRepo,
		Connections: &mappingConnectionProvider{db: db, adapter: adapter},
		Media:       adminNoopLocalMediaRecorder{},
		Transactions: &mappingImportUoW{
			products:    productStore,
			skus:        skuStore,
			mappings:    mappingStore,
			skuMappings: skuMappingStore,
		},
	})
	if err != nil {
		t.Fatalf("create product mapping service: %v", err)
	}
	mappingService.SetCategoryCreator(categoryService)

	h := mappinghttp.NewAdminHandler(mappingService)
	return h, db, conn.ID, server.Close
}

func TestBatchImportUpstreamProductsAutoCreatesCategory(t *testing.T) {
	h, db, connID, cleanup := setupAdminHandlerTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/upstream/categories":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"categories": []upstream.UpstreamCategory{
					{ID: 9, Slug: "upstream-streaming", Name: jsonmap.JSON{"zh-CN": "流媒体"}},
				},
			})
		case "/api/v1/upstream/products/101":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"product": upstream.UpstreamProduct{
					ID:              101,
					CategoryID:      9,
					Title:           jsonmap.JSON{"zh-CN": "上游商品"},
					Description:     jsonmap.JSON{"zh-CN": "描述"},
					Content:         jsonmap.JSON{"zh-CN": "内容"},
					Images:          []string{},
					Tags:            []string{},
					PriceAmount:     "10.00",
					Currency:        "CNY",
					FulfillmentType: constants.FulfillmentTypeAuto,
					IsActive:        true,
					SKUs: []upstream.UpstreamSKU{
						{ID: 201, SKUCode: "SKU-A", SpecValues: jsonmap.JSON{"name": "A"}, PriceAmount: "10.00", IsActive: true},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	body := fmt.Sprintf(`{"connection_id":%d,"upstream_product_ids":[101],"auto_create_category":true}`, connID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/product-mappings/batch-import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	h.BatchImportUpstreamProducts(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", w.Code, w.Body.String())
	}

	var imported productdomain.Product
	if err := db.First(&imported).Error; err != nil {
		t.Fatalf("load imported product failed: %v", err)
	}
	if imported.CategoryID == 0 {
		t.Fatalf("expected imported product to be assigned to auto-created category")
	}

	var category categorydomain.Category
	if err := db.First(&category, imported.CategoryID).Error; err != nil {
		t.Fatalf("load auto-created category failed: %v", err)
	}
	if category.Slug != "upstream-streaming" {
		t.Fatalf("expected auto-created category slug upstream-streaming, got %q", category.Slug)
	}
}

func TestBatchImportUpstreamProductsRestoresSoftDeletedAutoCategory(t *testing.T) {
	h, db, connID, cleanup := setupAdminHandlerTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/upstream/categories":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"categories": []upstream.UpstreamCategory{
					{ID: 9, Slug: "upstream-streaming", Name: jsonmap.JSON{"zh-CN": "流媒体"}},
				},
			})
		case "/api/v1/upstream/products/101":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"product": upstream.UpstreamProduct{
					ID:              101,
					CategoryID:      9,
					Title:           jsonmap.JSON{"zh-CN": "上游商品"},
					PriceAmount:     "10.00",
					Currency:        "CNY",
					FulfillmentType: constants.FulfillmentTypeAuto,
					IsActive:        true,
					SKUs: []upstream.UpstreamSKU{
						{ID: 201, SKUCode: "SKU-A", SpecValues: jsonmap.JSON{"name": "A"}, PriceAmount: "10.00", IsActive: true},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	})
	defer cleanup()

	deletedCategory := categorydomain.Category{
		Slug:     "upstream-streaming",
		NameJSON: jsonmap.JSON{"zh-CN": "已删除分类"},
		IsActive: true,
	}
	if err := db.Create(&deletedCategory).Error; err != nil {
		t.Fatalf("create soft-delete target category failed: %v", err)
	}
	if err := categorygormstore.NewCategoryStore(db).Delete(fmt.Sprintf("%d", deletedCategory.ID)); err != nil {
		t.Fatalf("soft delete category failed: %v", err)
	}

	body := fmt.Sprintf(`{"connection_id":%d,"upstream_product_ids":[101],"auto_create_category":true}`, connID)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/product-mappings/batch-import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	h.BatchImportUpstreamProducts(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", w.Code, w.Body.String())
	}

	var imported productdomain.Product
	if err := db.First(&imported).Error; err != nil {
		t.Fatalf("load imported product failed: %v", err)
	}
	if imported.CategoryID != deletedCategory.ID {
		t.Fatalf("expected imported product category %d, got %d", deletedCategory.ID, imported.CategoryID)
	}

	var restored categorydomain.Category
	if err := db.First(&restored, deletedCategory.ID).Error; err != nil {
		t.Fatalf("expected category to be restored, got %v", err)
	}
	if restored.NameJSON["zh-CN"] != "流媒体" {
		t.Fatalf("expected restored category name to be refreshed, got %+v", restored.NameJSON)
	}
}

// seedMappedProduct 建一个已对接商品：上游价 100，本地 1 个 SKU。
func seedMappedProduct(t *testing.T, db *gorm.DB, connID uint) (mappingdomain.Mapping, productdomain.ProductSKU) {
	t.Helper()
	product := productdomain.Product{
		CategoryID: 1, Slug: fmt.Sprintf("p-%d", connID), TitleJSON: jsonmap.JSON{"zh-CN": "GPT Plus"},
		PriceAmount: money.FromDecimal(decimal.RequireFromString("110")), IsMapped: true, IsActive: true,
		FulfillmentType: constants.FulfillmentTypeManual,
	}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	sku := productdomain.ProductSKU{ProductID: product.ID, SKUCode: "DEFAULT", PriceAmount: product.PriceAmount, IsActive: true}
	if err := db.Create(&sku).Error; err != nil {
		t.Fatal(err)
	}
	mapping := mappingdomain.Mapping{ConnectionID: connID, LocalProductID: product.ID, UpstreamProductID: 101, IsActive: true}
	if err := db.Create(&mapping).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&mappingdomain.SKUMapping{
		ProductMappingID: mapping.ID, LocalSKUID: sku.ID, UpstreamSKUID: 201,
		UpstreamPrice: money.FromDecimal(decimal.RequireFromString("100")), UpstreamIsActive: true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	return mapping, sku
}

// putMarkup 返回业务码（项目统一 HTTP 200 + body.status_code）。
func putMarkup(t *testing.T, h interface{ UpdateProductMappingMarkup(*gin.Context) }, mappingID uint, body string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(mappingID)}}
	h.UpdateProductMappingMarkup(c)
	var resp struct {
		StatusCode int `json:"status_code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, w.Body.String())
	}
	return resp.StatusCode
}

func TestMappingMarkupOverridesConnectionAndRepricesNow(t *testing.T) {
	h, db, connID, cleanup := setupAdminHandlerTest(t, http.NotFound)
	defer cleanup()
	if err := db.Model(&siteconnectiondomain.Connection{}).Where("id = ?", connID).
		Updates(map[string]any{"price_markup_percent": 10, "exchange_rate": 1}).Error; err != nil {
		t.Fatal(err)
	}
	mapping, sku := seedMappedProduct(t, db, connID)

	if code := putMarkup(t, h, mapping.ID, `{"price_markup_percent":30}`); code != 0 {
		t.Fatalf("set markup status %d", code)
	}
	var got productdomain.ProductSKU
	db.First(&got, sku.ID)
	if !got.PriceAmount.Decimal.Equal(decimal.RequireFromString("130")) {
		t.Fatalf("expected 130 after 30%% markup, got %s", got.PriceAmount.Decimal)
	}
	if !got.CostPriceAmount.Decimal.Equal(decimal.RequireFromString("100")) {
		t.Fatalf("cost should be upstream price, got %s", got.CostPriceAmount.Decimal)
	}

	// 清空单品加价 → 回到连接统一的 10%
	if code := putMarkup(t, h, mapping.ID, `{"price_markup_percent":null}`); code != 0 {
		t.Fatalf("clear markup status %d", code)
	}
	db.First(&got, sku.ID)
	if !got.PriceAmount.Decimal.Equal(decimal.RequireFromString("110")) {
		t.Fatalf("expected connection markup 110, got %s", got.PriceAmount.Decimal)
	}

	if code := putMarkup(t, h, mapping.ID, `{"price_markup_percent":-150}`); code != response.CodeBadRequest {
		t.Fatalf("expected 400 for markup below -100%%, got %d", code)
	}
}

func TestSyncUsesPerMappingMarkupWhenUpstreamPriceChanges(t *testing.T) {
	var upstreamPrice = "200.00"
	h, db, connID, cleanup := setupAdminHandlerTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/upstream/products/101" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "product": upstream.UpstreamProduct{
			ID: 101, Title: jsonmap.JSON{"zh-CN": "GPT Plus"}, PriceAmount: upstreamPrice, Currency: "CNY",
			FulfillmentType: constants.FulfillmentTypeAuto, IsActive: true,
			SKUs: []upstream.UpstreamSKU{{ID: 201, SKUCode: "DEFAULT", PriceAmount: upstreamPrice, IsActive: true, StockQuantity: -1}},
		}})
	})
	defer cleanup()
	if err := db.Model(&siteconnectiondomain.Connection{}).Where("id = ?", connID).
		Updates(map[string]any{"price_markup_percent": 10, "exchange_rate": 1, "auto_sync_price": true}).Error; err != nil {
		t.Fatal(err)
	}
	mapping, sku := seedMappedProduct(t, db, connID)
	if code := putMarkup(t, h, mapping.ID, `{"price_markup_percent":25}`); code != 0 {
		t.Fatalf("set markup status %d", code)
	}

	// 上游涨价到 200，同步后应按单品 25% 而不是连接的 10%
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(mapping.ID)}}
	h.SyncProductMapping(c)
	if w.Code != http.StatusOK {
		t.Fatalf("sync status %d body=%s", w.Code, w.Body.String())
	}

	var got productdomain.ProductSKU
	db.First(&got, sku.ID)
	if !got.PriceAmount.Decimal.Equal(decimal.RequireFromString("250")) {
		t.Fatalf("expected 250 (200 × 1.25), got %s", got.PriceAmount.Decimal)
	}
	var product productdomain.Product
	db.First(&product, mapping.LocalProductID)
	if !product.PriceAmount.Decimal.Equal(decimal.RequireFromString("250")) {
		t.Fatalf("product price should follow sku, got %s", product.PriceAmount.Decimal)
	}
}
