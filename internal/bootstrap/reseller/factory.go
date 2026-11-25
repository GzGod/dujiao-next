package resellerbootstrap

import (
	"context"
	"github.com/dujiao-next/internal/app/container"
	resellerapp "github.com/dujiao-next/internal/modules/reseller/application"
	railwayapi "github.com/dujiao-next/internal/modules/reseller/infrastructure/railwayapi"
	adminhttp "github.com/dujiao-next/internal/modules/reseller/transport/http/admin"
	userhttp "github.com/dujiao-next/internal/modules/reseller/transport/http/user"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Handlers struct {
	User                *userhttp.UserHandler
	UserProductSetting  *userhttp.UserProductSettingHandler
	UserFinance         *userhttp.UserFinanceHandler
	UserOrder           *userhttp.UserOrderHandler
	AdminManagement     *adminhttp.AdminManagementHandler
	AdminProfileDetail  *adminhttp.AdminProfileDetailHandler
	AdminSiteConfig     *adminhttp.AdminSiteConfigHandler
	AdminProductSetting *adminhttp.AdminProductSettingHandler
	AdminOperations     *adminhttp.AdminOperationsHandler
	AdminFinance        *adminhttp.AdminFinanceHandler
}

func New(c *container.Container) Handlers {
	user := userhttp.NewUserHandler(
		c.ResellerManagementService,
		c.ResellerSiteConfigService,
		c.UploadService,
	)
	user.SetFrontendService(resellerapp.NewFrontendService(c.ResellerStore))
	return Handlers{
		User:               user,
		UserProductSetting: userhttp.NewUserProductSettingHandler(c.ResellerProductSettingService),
		UserFinance: userhttp.NewUserFinanceHandler(
			c.ResellerAccountingQuery,
			c.ResellerAccountingWithdraw,
		),
		UserOrder: userhttp.NewUserOrderHandler(c.ResellerOrderService),
		AdminManagement: adminhttp.NewAdminManagementHandler(
			c.ResellerManagementService, c.ResellerStore, c.AuthzAuditService,
		),
		AdminProfileDetail: adminhttp.NewAdminProfileDetailHandler(
			c.ResellerStore, c.ResellerProductSettingService, c.ResellerAccountingQuery, c.ResellerOrderService,
		),
		AdminSiteConfig: adminhttp.NewAdminSiteConfigHandler(
			c.ResellerSiteConfigService, c.ResellerStore, c.AuthzAuditService,
		),
		AdminProductSetting: adminhttp.NewAdminProductSettingHandler(
			c.ResellerProductSettingService, c.AuthzAuditService,
		),
		AdminOperations: adminhttp.NewAdminOperationsHandler(c.ResellerOperationsService),
		AdminFinance: adminhttp.NewAdminFinanceHandler(
			c.ResellerAccountingQuery,
			c.ResellerAccountingWithdraw,
		),
	}
}

var provisioningOnce sync.Once

// StartDomainProvisioning requires a project token rather than a CLI/account token.
func StartDomainProvisioning(c *container.Container) {
	token := strings.TrimSpace(os.Getenv("RESELLER_RAILWAY_TOKEN"))
	project := os.Getenv("RAILWAY_PROJECT_ID")
	environment := os.Getenv("RAILWAY_ENVIRONMENT_ID")
	service := os.Getenv("RAILWAY_SERVICE_ID")
	if token == "" || project == "" || environment == "" || service == "" || !c.Config.Reseller.Enabled {
		return
	}
	provisioningOnce.Do(func() {
		port, _ := strconv.Atoi(os.Getenv("PORT"))
		if port == 0 {
			port = 8080
		}
		provider := &railwayapi.Client{Token: token, ProjectID: project, EnvironmentID: environment, ServiceID: service, Port: port}
		worker := &resellerapp.DomainProvisioningService{Store: c.ResellerStore, Provider: provider}
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				_ = worker.Sync(ctx)
				cancel()
				<-ticker.C
			}
		}()
	})
}
