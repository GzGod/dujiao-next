package gormuow

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dujiao-next/internal/constants"
	apidomain "github.com/dujiao-next/internal/modules/apicredential/domain"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	leveldomain "github.com/dujiao-next/internal/modules/memberlevel/domain"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"strings"
	"sync"
	"testing"
	"time"
)

func approvalFixture(t *testing.T) (*gorm.DB, *Approver, userdomain.User, apidomain.ApiCredential, leveldomain.MemberLevel) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:agent-review-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	if err := db.AutoMigrate(&userdomain.User{}, &apidomain.ApiCredential{}, &leveldomain.MemberLevel{}); err != nil {
		t.Fatal(err)
	}
	level := leveldomain.MemberLevel{NameJSON: jsonmap.JSON{"zh-CN": "初级代理"}, Slug: "basic-agent", IsActive: true, RequiresApproval: true, SortOrder: 1}
	user := userdomain.User{Email: "review@example.test", PasswordHash: "hash", Status: "active"}
	if err := db.Create(&level).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	cred := apidomain.ApiCredential{UserID: user.ID, ApiKey: "pending-key", Status: constants.ApiCredentialStatusPendingReview}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatal(err)
	}
	return db, &Approver{DB: db}, user, cred, level
}

func TestAgentApprovalAtomicIdempotentAndConcurrent(t *testing.T) {
	db, service, user, cred, level := approvalFixture(t)
	var wg sync.WaitGroup
	results := make(chan *apidomain.ApiCredential, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Go(func() { r, e := service.Approve(cred.ID, 0); results <- r; errs <- e })
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	secret := ""
	for result := range results {
		if result == nil || !result.IsActive || result.Status != constants.ApiCredentialStatusApproved || len(result.ApiSecret) != 128 {
			t.Fatal("credential not enabled")
		}
		if secret != "" && result.ApiSecret != secret {
			t.Fatal("retry rotated secret")
		}
		secret = result.ApiSecret
		encoded, _ := json.Marshal(result)
		if strings.Contains(string(encoded), secret) {
			t.Fatal("secret exposed in response")
		}
	}
	db.First(&user, user.ID)
	if user.MemberLevelID != level.ID {
		t.Fatal("grade not assigned")
	}
}

func TestAgentApprovalRollsBackGradeWhenCredentialWriteFails(t *testing.T) {
	db, service, user, cred, level := approvalFixture(t)
	db.Callback().Update().Before("gorm:update").Register("fail_api_approval", func(tx *gorm.DB) {
		if tx.Statement.Table == "api_credentials" {
			tx.AddError(errors.New("write failed"))
		}
	})
	if _, err := service.Approve(cred.ID, level.ID); err == nil {
		t.Fatal("expected failure")
	}
	db.First(&user, user.ID)
	db.First(&cred, cred.ID)
	if user.MemberLevelID != 0 || cred.Status != constants.ApiCredentialStatusPendingReview || cred.ApiSecret != "" {
		t.Fatal("approval was partially committed")
	}
}

func TestAgentApprovalRejectsDisabledAndDowngrade(t *testing.T) {
	db, service, user, cred, level := approvalFixture(t)
	high := leveldomain.MemberLevel{NameJSON: jsonmap.JSON{"zh-CN": "高级代理"}, Slug: "high-agent", IsActive: true, RequiresApproval: true, SortOrder: 9}
	db.Create(&high)
	db.Model(&user).Update("member_level_id", high.ID)
	if _, err := service.Approve(cred.ID, level.ID); err == nil {
		t.Fatal("silently downgraded agent")
	}
	db.Model(&user).Updates(map[string]interface{}{"member_level_id": 0, "status": "disabled"})
	if _, err := service.Approve(cred.ID, level.ID); err == nil {
		t.Fatal("approved disabled user")
	}
	db.Model(&user).Update("status", "active")
	db.Model(&cred).Update("status", constants.ApiCredentialStatusDisabled)
	if _, err := service.Approve(cred.ID, level.ID); err == nil {
		t.Fatal("re-enabled revoked credential")
	}
}
