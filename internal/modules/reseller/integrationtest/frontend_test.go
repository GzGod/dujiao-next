package integrationtest

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	app "github.com/dujiao-next/internal/modules/reseller/application"
	contract "github.com/dujiao-next/internal/modules/reseller/contract"
	domain "github.com/dujiao-next/internal/modules/reseller/domain"
	store "github.com/dujiao-next/internal/modules/reseller/infrastructure/gormstore"
)

func frontendZIP(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for name, data := range files {
		f, e := w.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		io.WriteString(f, data)
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestFrontendArchiveValidation(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"traversal":       {"index.html": "<html></html>", "../evil.js": "bad"},
		"absolute":        {"index.html": "<html></html>", "/evil.js": "bad"},
		"hidden":          {"index.html": "<html></html>", ".env": "secret"},
		"server code":     {"index.html": "<html></html>", "app.php": "bad"},
		"missing index":   {"dist/index.html": "<html></html>"},
		"reserved":        {"index.html": "<html></html>", "api/index.html": "bad"},
		"oversized entry": {"index.html": "<html></html>", "assets/large.js": strings.Repeat("a", (5<<20)+1)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, e := app.ValidateFrontendArchive(frontendZIP(t, files)); e == nil {
				t.Fatal("invalid ZIP accepted")
			}
		})
	}
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, _ := w.Create("index.html")
	io.WriteString(f, "<html></html>")
	header := &zip.FileHeader{Name: "assets/link.js"}
	header.SetMode(os.ModeSymlink | 0777)
	f, _ = w.CreateHeader(header)
	io.WriteString(f, "/etc/passwd")
	w.Close()
	if _, e := app.ValidateFrontendArchive(b.Bytes()); e == nil {
		t.Fatal("symlink accepted")
	}
	if _, e := app.ValidateFrontendArchive(frontendZIP(t, map[string]string{"index.html": "<html><head></head></html>", "assets/app.js": "console.log(1)"})); e != nil {
		t.Fatal(e)
	}
}
func TestFrontendDraftPublishTenantIsolationAndDurableRestore(t *testing.T) {
	db := openResellerProductSettingServiceTestDB(t)
	if e := db.AutoMigrate(&domain.FrontendBundle{}, &domain.Domain{}); e != nil {
		t.Fatal(e)
	}
	user, profile, _, _ := seedResellerProductSettingServiceData(t, db)
	if e := db.Create(&domain.Domain{ResellerID: profile.ID, Domain: "shop.test", Type: domain.DomainTypeCustom, Status: domain.DomainStatusActive, VerificationStatus: domain.DomainVerificationVerified}).Error; e != nil {
		t.Fatal(e)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	repository := store.New(db)
	service := app.NewFrontendService(repository)
	archive := frontendZIP(t, map[string]string{"index.html": "<html><head></head><script src=\"/assets/app.js\"></script></html>", "assets/app.js": "const endpoint='/api/v1/public/products';"})
	uploaded, e := service.Upload(user.ID, archive)
	if e != nil {
		t.Fatal(e)
	}
	tenant := contract.ResellerTenantContext("shop.test", profile.ID, user.ID, "shop.test")
	asset, e := service.Asset(tenant, "/", "")
	if e != nil || asset != nil {
		t.Fatal("draft changed live frontend")
	}
	asset, e = service.Asset(tenant, "/", uploaded.PreviewToken)
	if e != nil || !asset.Preview || !bytes.Contains(asset.Data, []byte("/__frontend_preview/")) {
		t.Fatalf("preview failed: %v", e)
	}
	asset, e = service.Asset(tenant, "/assets/app.js", uploaded.PreviewToken)
	if e != nil || !bytes.Contains(asset.Data, []byte("/api/v1/public/products")) {
		t.Fatal("preview rewrote API path")
	}
	if _, e = service.Asset(tenant, "/", "wrong-token"); e == nil {
		t.Fatal("invalid preview token accepted")
	}
	if e = service.Publish(user.ID, "stale"); e == nil {
		t.Fatal("stale publish accepted")
	}
	if e = service.Publish(user.ID, uploaded.DraftHash); e != nil {
		t.Fatal(e)
	}
	// New service instance reads the persisted release, without relying on a container filesystem.
	service = app.NewFrontendService(store.New(db))
	asset, e = service.Asset(tenant, "/nested/spa", "")
	if e != nil || !bytes.Contains(asset.Data, []byte("<html>")) {
		t.Fatal("published SPA unavailable")
	}
	main := contract.MainTenantContext("main.test")
	if a, e := service.Asset(main, "/", ""); e != nil || a != nil {
		t.Fatal("custom frontend leaked to main")
	}
	other := contract.ResellerTenantContext("other.test", profile.ID+100, user.ID+100, "other.test")
	if a, e := service.Asset(other, "/", uploaded.PreviewToken); e != nil || a != nil {
		t.Fatal("custom frontend leaked to another tenant")
	}
	next, e := service.Upload(user.ID, frontendZIP(t, map[string]string{"index.html": "<html>new draft</html>"}))
	if e != nil {
		t.Fatal(e)
	}
	live, _ := service.Asset(tenant, "/", "")
	if bytes.Contains(live.Data, []byte("new draft")) {
		t.Fatal("upload replaced published archive")
	}
	if e = service.Publish(user.ID, uploaded.DraftHash); e == nil {
		t.Fatal("old draft hash published newer upload")
	}
	if e = service.Disable(user.ID); e != nil {
		t.Fatal(e)
	}
	live, e = service.Asset(tenant, "/", "")
	if e != nil || live != nil {
		t.Fatal("restore failed")
	}
	status, e := service.Status(user.ID)
	if e != nil || status.DraftHash != next.DraftHash || status.PublishedHash != "" {
		t.Fatal("restore removed draft")
	}
	db.Model(&domain.FrontendBundle{}).Where("reseller_id = ?", profile.ID).Update("preview_expires_at", time.Now().Add(-time.Minute))
	if _, e = service.Asset(tenant, "/", next.PreviewToken); e == nil {
		t.Fatal("expired preview accepted")
	}
	db.Model(&domain.Profile{}).Where("id = ?", profile.ID).Update("status", domain.ProfileStatusDisabled)
	if _, e = service.Upload(user.ID, archive); e == nil {
		t.Fatal("disabled owner uploaded")
	}
	if live, e = service.Asset(tenant, "/", ""); e != nil || live != nil {
		t.Fatal("disabled site served frontend")
	}
}
func TestResellerThemeOverlayAndValidation(t *testing.T) {
	db := openResellerProductSettingServiceTestDB(t)
	if e := db.AutoMigrate(&domain.SiteConfig{}, &domain.Domain{}); e != nil {
		t.Fatal(e)
	}
	user, profile, _, _ := seedResellerProductSettingServiceData(t, db)
	repository := store.New(db)
	service := app.NewSiteConfigService(repository)
	for _, theme := range []app.ResellerThemeInput{{Template: "evil"}, {Template: "xgai", PrimaryColor: "red;display:none"}, {PrimaryColor: "#gg0000"}} {
		if _, e := service.UpdateUserSiteConfig(context.Background(), user.ID, app.ResellerSiteConfigInput{Theme: &theme}); e == nil {
			t.Fatal("invalid theme accepted")
		}
	}
	theme := app.ResellerThemeInput{Template: "xgai", PrimaryColor: "#8844cc"}
	if _, e := service.UpdateUserSiteConfig(context.Background(), user.ID, app.ResellerSiteConfigInput{SiteName: "Own shop", Theme: &theme}); e != nil {
		t.Fatal(e)
	}
	// Older brand forms omit theme; editing them must retain a previously saved appearance.
	if _, e := service.UpdateUserSiteConfig(context.Background(), user.ID, app.ResellerSiteConfigInput{SiteName: "Renamed"}); e != nil {
		t.Fatal(e)
	}
	tenant := contract.ResellerTenantContext("shop.test", profile.ID, user.ID, "shop.test")
	base := map[string]interface{}{"storefront_template": "classic"}
	out, e := service.ApplyPublicConfigOverlay(context.Background(), tenant, base)
	if e != nil || out["storefront_template"] != "xgai" || out["reseller_theme"] == nil {
		t.Fatalf("overlay failed: %v %v", out, e)
	}
	if base["storefront_template"] != "classic" {
		t.Fatal("main config mutated")
	}
	main, e := service.ApplyPublicConfigOverlay(context.Background(), contract.MainTenantContext("main.test"), base)
	if e != nil || main["reseller_theme"] != nil {
		t.Fatal("theme leaked to main")
	}
}

func TestFrontendRejectsPublishWithoutVerifiedDomain(t *testing.T) {
	db := openResellerProductSettingServiceTestDB(t)
	if e := db.AutoMigrate(&domain.FrontendBundle{}, &domain.Domain{}); e != nil {
		t.Fatal(e)
	}
	user, _, _, _ := seedResellerProductSettingServiceData(t, db)
	service := app.NewFrontendService(store.New(db))
	status, e := service.Upload(user.ID, frontendZIP(t, map[string]string{"index.html": "<html>draft</html>"}))
	if e != nil {
		t.Fatal(e)
	}
	if e = service.Publish(user.ID, status.DraftHash); e == nil {
		t.Fatal("publish without domain accepted")
	}
	if _, e = service.PreviewToken(user.ID); e == nil {
		t.Fatal("preview without domain accepted")
	}
}

func TestRedemptionSiteAutoAccessAndRevocation(t *testing.T) {
	db := openResellerProductSettingServiceTestDB(t)
	if err := db.AutoMigrate(&domain.Domain{}, &domain.SiteConfig{}); err != nil {
		t.Fatal(err)
	}
	_, profile, _, _ := seedResellerProductSettingServiceData(t, db)
	row := domain.Domain{ResellerID: profile.ID, Domain: "agent.example", Type: domain.DomainTypeCustom, Status: domain.DomainStatusActive, VerificationStatus: domain.DomainVerificationVerified}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	service := app.NewSiteConfigService(store.New(db))
	site, err := service.RedemptionSite("agent.example")
	if err != nil || site == nil || site["enabled"] != true || site["origin"] != "https://agent.example" {
		t.Fatalf("active verified site not authorized: %v %v", site, err)
	}
	for _, host := range []string{"missing.example", "agent.example:443", "https://agent.example", "agent.example/path"} {
		site, err = service.RedemptionSite(host)
		if err != nil || site != nil {
			t.Fatalf("invalid host authorized: %s", host)
		}
	}
	if err := db.Model(&row).Update("verification_status", "pending").Error; err != nil {
		t.Fatal(err)
	}
	if site, err = service.RedemptionSite("agent.example"); err != nil || site != nil {
		t.Fatal("unverified domain authorized")
	}
	if err := db.Model(&row).Update("verification_status", domain.DomainVerificationVerified).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&profile).Update("status", "disabled").Error; err != nil {
		t.Fatal(err)
	}
	if site, err = service.RedemptionSite("agent.example"); err != nil || site != nil {
		t.Fatal("disabled profile authorized")
	}
}
