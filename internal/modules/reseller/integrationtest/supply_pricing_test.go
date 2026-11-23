package integrationtest

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dujiao-next/internal/config"
	app "github.com/dujiao-next/internal/modules/reseller/application"
	domain "github.com/dujiao-next/internal/modules/reseller/domain"
	store "github.com/dujiao-next/internal/modules/reseller/infrastructure/gormstore"
	"github.com/shopspring/decimal"
)

func TestSupplyPricingPersistsPerSiteAndControlsPreviewWithoutLeakingCost(t *testing.T) {
	db := openResellerProductSettingServiceTestDB(t)
	user, profile, product, skus := seedResellerProductSettingServiceData(t, db)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	repository := store.New(db)
	management := app.NewManagementService(repository, config.ResellerConfig{})
	markup := "3"
	supply := &domain.SupplyPricing{DefaultMarkupPercent: &markup, Overrides: []domain.SupplyPriceRule{{ProductID: product.ID, SKUID: skus[0].ID, Mode: "fixed", Value: "90"}}}
	input := app.ResellerProfileUpdateInput{DefaultMarkupPercent: profile.DefaultMarkupPercent.Decimal, MaxMarkupPercent: profile.MaxMarkupPercent.Decimal, SupplyPricing: supply}
	if _, err := management.UpdateProfileOperationalConfig(1, profile.ID, input); err != nil {
		t.Fatal(err)
	}
	loaded, err := repository.GetProfileByID(profile.ID)
	if err != nil || loaded.SupplyPricing == nil || *loaded.SupplyPricing.DefaultMarkupPercent != "3" {
		t.Fatalf("not persisted: %+v %v", loaded, err)
	}
	raw, _ := json.Marshal(loaded)
	if strings.Contains(string(raw), "supply_pricing") || strings.Contains(string(raw), "cost_markup") {
		t.Fatalf("raw profile leaked supply rules: %s", raw)
	}
	detail, err := newResellerProductSettingServiceForTest(db).GetUserProductSetting(user.ID, product.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Product.SKUs[0].PriceAmount.String() != "90.00" || detail.Product.SKUs[1].PriceAmount.String() != "123.60" {
		t.Fatalf("wrong supply prices: %+v", detail.Product.SKUs)
	}
	if !detail.EffectiveBySKUID[skus[0].ID].Equal(decimal.RequireFromString("99")) {
		t.Fatalf("resale does not use supply: %v", detail.EffectiveBySKUID)
	}
	other := &domain.Profile{}
	base, err := app.ResolveSupplyUnitAmount(other, product.ID, skus[0].ID, decimal.NewFromInt(100), decimal.NewFromInt(80))
	if err != nil || !base.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("other site changed: %s %v", base, err)
	}
	// An unrelated edit must preserve supply rules. An empty object explicitly resets inheritance.
	input.SupplyPricing = nil
	if _, err := management.UpdateProfileOperationalConfig(1, profile.ID, input); err != nil {
		t.Fatal(err)
	}
	loaded, _ = repository.GetProfileByID(profile.ID)
	if loaded.SupplyPricing == nil {
		t.Fatal("unrelated update cleared supply")
	}
	input.SupplyPricing = &domain.SupplyPricing{}
	if _, err := management.UpdateProfileOperationalConfig(1, profile.ID, input); err != nil {
		t.Fatal(err)
	}
	loaded, _ = repository.GetProfileByID(profile.ID)
	base, err = app.ResolveSupplyUnitAmount(loaded, product.ID, skus[0].ID, decimal.NewFromInt(100), decimal.NewFromInt(80))
	if err != nil || !base.Equal(decimal.NewFromInt(100)) {
		t.Fatal("reset did not restore retail")
	}
}

func TestSupplyPricingRejectsInvalidOrBelowCostOverrides(t *testing.T) {
	db := openResellerProductSettingServiceTestDB(t)
	_, profile, product, skus := seedResellerProductSettingServiceData(t, db)
	management := app.NewManagementService(store.New(db), config.ResellerConfig{})
	for _, rules := range [][]domain.SupplyPriceRule{
		{{ProductID: product.ID, SKUID: skus[0].ID, Mode: "fixed", Value: "79"}},
		{{ProductID: product.ID, SKUID: 999999, Mode: "fixed", Value: "90"}},
		{{ProductID: 999999, Mode: "fixed", Value: "90"}},
		{{ProductID: product.ID, Mode: "fixed", Value: "90"}}, // below SKU B cost
		{{ProductID: product.ID, SKUID: skus[0].ID, Mode: "fixed", Value: "90"}, {ProductID: product.ID, SKUID: skus[0].ID, Mode: "fixed", Value: "91"}},
	} {
		_, err := management.UpdateProfileOperationalConfig(1, profile.ID, app.ResellerProfileUpdateInput{SupplyPricing: &domain.SupplyPricing{Overrides: rules}})
		if err == nil {
			t.Fatalf("accepted invalid rules: %+v", rules)
		}
	}
	markup := "3"
	site := &domain.Profile{SupplyPricing: &domain.SupplyPricing{DefaultMarkupPercent: &markup}}
	base, err := app.ResolveSupplyUnitAmount(site, product.ID, skus[0].ID, decimal.NewFromInt(100), decimal.Zero)
	if err != nil || !base.Equal(decimal.NewFromInt(100)) {
		t.Fatal("unknown cost should inherit retail")
	}
	site.SupplyPricing.Overrides = []domain.SupplyPriceRule{{ProductID: product.ID, SKUID: skus[0].ID, Mode: "fixed", Value: "90"}}
	if _, err := app.ResolveSupplyUnitAmount(site, product.ID, skus[0].ID, decimal.NewFromInt(100), decimal.NewFromInt(91)); err == nil {
		t.Fatal("cost increase must block loss")
	}
}
