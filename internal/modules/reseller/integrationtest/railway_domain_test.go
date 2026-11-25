package integrationtest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	app "github.com/dujiao-next/internal/modules/reseller/application"
	domain "github.com/dujiao-next/internal/modules/reseller/domain"
	store "github.com/dujiao-next/internal/modules/reseller/infrastructure/gormstore"
	railway "github.com/dujiao-next/internal/modules/reseller/infrastructure/railwayapi"
)

type domainTransport func(*http.Request) (*http.Response, error)

func (f domainTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRailwayDomainTimeoutRecoversWithoutDuplicateCreate(t *testing.T) {
	exists := false
	creates := 0
	client := &railway.Client{Token: "test-private-token", ProjectID: "project", EnvironmentID: "env", ServiceID: "service", Port: 8080}
	client.HTTP = &http.Client{Transport: domainTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://backboard.railway.com/graphql/v2" || r.Header.Get("Project-Access-Token") != "test-private-token" {
			t.Fatal("unexpected endpoint or auth")
		}
		var body struct {
			Query     string                 `json:"query"`
			Variables map[string]interface{} `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(body.Query, "mutation") {
			creates++
			exists = true
			return nil, errors.New("network timeout, token must not escape")
		}
		rows := "[]"
		if exists {
			rows = `[{"id":"provider-id","domain":"shop.example.test","targetPort":8080,"status":{"verified":true,"certificateStatus":"CERTIFICATE_STATUS_TYPE_VALID","verificationToken":"public-txt","verificationDnsHost":"_railway-verify.shop","dnsRecords":[{"recordType":"DNS_RECORD_TYPE_CNAME","hostlabel":"shop","requiredValue":"target.up.railway.app","status":"DNS_RECORD_STATUS_PROPAGATED"}]}}]`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":{"domains":{"customDomains":` + rows + `}}}`)), Header: http.Header{}}, nil
	})}
	if _, _, err := client.EnsureDomain(context.Background(), "shop.example.test"); err == nil || strings.Contains(err.Error(), "token") {
		t.Fatal("timeout not sanitized")
	}
	id, dns, err := client.EnsureDomain(context.Background(), "shop.example.test")
	if err != nil || id != "provider-id" || dns.Status != "ready" || creates != 1 {
		t.Fatalf("recovery failed: %s %+v %v creates=%d", id, dns, err, creates)
	}
}

type domainProviderStub struct {
	run func(context.Context, string) (string, *domain.DomainDNS, error)
}

func (p domainProviderStub) EnsureDomain(ctx context.Context, host string) (string, *domain.DomainDNS, error) {
	return p.run(ctx, host)
}

func TestAutomaticDomainChecksLeaseAndActivation(t *testing.T) {
	db := openResellerManagementServiceTestDB(t)
	repo := store.New(db)
	user := seedResellerManagementUser(t, db, "autodns@example.test")
	profile := domain.Profile{UserID: user.ID, Status: domain.ProfileStatusActive}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	row := domain.Domain{ResellerID: profile.ID, Domain: "shop.example.test", Type: domain.DomainTypeCustom, Status: domain.DomainStatusPendingReview, VerificationStatus: domain.DomainVerificationPending}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	claimed, err := repo.ClaimDomainChecks(now, "lease-a", 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim failed %v %v", claimed, err)
	}
	other, err := repo.ClaimDomainChecks(now, "lease-b", 10)
	if err != nil || len(other) != 0 {
		t.Fatal("double lease")
	}
	dns := &domain.DomainDNS{CNAME: "target.example", Status: "ready", UpdatedAt: now}
	if changed, err := repo.CompleteDomainCheck(row.ID, "wrong-lease", "id", dns); err != nil || changed {
		t.Fatal("stale worker wrote result")
	}
	if changed, err := repo.CompleteDomainCheck(row.ID, "lease-a", "id", dns); err != nil || !changed {
		t.Fatal(err)
	}
	loaded, err := repo.GetDomainByID(row.ID)
	if err != nil || loaded.Status != domain.DomainStatusActive || loaded.VerificationStatus != domain.DomainVerificationVerified || !loaded.IsPrimary {
		t.Fatal("not activated")
	}
	// A disabled domain cannot be reactivated by a delayed provider response.
	db.Model(&row).Updates(map[string]interface{}{"status": domain.DomainStatusDisabled, "check_lease": "lease-c"})
	if changed, err := repo.CompleteDomainCheck(row.ID, "lease-c", "id", dns); err != nil || changed {
		t.Fatal("disabled domain reactivated")
	}
	// HTTP provider code can query the SQLite connection: no transaction surrounds HTTP.
	db.Model(&row).Updates(map[string]interface{}{"status": domain.DomainStatusPendingReview, "check_lease": "", "next_check_at": now.Add(-time.Minute)})
	worker := app.DomainProvisioningService{Store: repo, Provider: domainProviderStub{run: func(context.Context, string) (string, *domain.DomainDNS, error) {
		if _, err := repo.GetProfileByID(profile.ID); err != nil {
			return "", nil, err
		}
		return "", nil, errors.New("private provider error")
	}}}
	if err := worker.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	loaded, _ = repo.GetDomainByID(row.ID)
	if loaded.DNS.ErrorCode != "provider_unavailable" || loaded.DNS.Status != "error" || loaded.Status != domain.DomainStatusPendingReview {
		t.Fatal("unsafe failure handling")
	}
}
