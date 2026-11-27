package gormuow

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/dujiao-next/internal/constants"
	apicontract "github.com/dujiao-next/internal/modules/apicredential/contract"
	apidomain "github.com/dujiao-next/internal/modules/apicredential/domain"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	leveldomain "github.com/dujiao-next/internal/modules/memberlevel/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

// Approver commits the agent grade and API credential together; no service calls or HTTP occur in the transaction.
type Approver struct{ DB *gorm.DB }

func (a *Approver) Approve(id, levelID uint) (*apidomain.ApiCredential, error) {
	key := make([]byte, 32)
	secret := make([]byte, 64)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	var result apidomain.ApiCredential
	err := a.DB.Transaction(func(tx *gorm.DB) error {
		var snapshot apidomain.ApiCredential
		if err := tx.Where("id = ? AND deleted_at IS NULL", id).First(&snapshot).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apicontract.ErrNotFound
			}
			return err
		}
		var user userdomain.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL AND status = ?", snapshot.UserID, "active").First(&user).Error; err != nil {
			return apicontract.ErrAgentApprovalInvalid
		}
		var cred apidomain.ApiCredential
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", id).First(&cred).Error; err != nil {
			return err
		}
		var level leveldomain.MemberLevel
		q := tx.Where("deleted_at IS NULL AND is_active = ? AND requires_approval = ?", true, true)
		if levelID > 0 {
			q = q.Where("id = ?", levelID)
		}
		if err := q.Order("sort_order ASC, id ASC").First(&level).Error; err != nil {
			return apicontract.ErrAgentApprovalInvalid
		}
		if cred.Status == constants.ApiCredentialStatusApproved && cred.IsActive && user.MemberLevelID == level.ID {
			result = cred
			return nil
		}
		if cred.Status != constants.ApiCredentialStatusPendingReview {
			return apicontract.ErrAgentApprovalInvalid
		}
		// Never silently downgrade an existing higher agent grade.
		if user.MemberLevelID > 0 && user.MemberLevelID != level.ID {
			var current leveldomain.MemberLevel
			err := tx.Where("id = ? AND deleted_at IS NULL", user.MemberLevelID).First(&current).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil && current.RequiresApproval && current.SortOrder > level.SortOrder {
				return apicontract.ErrAgentApprovalInvalid
			}
		}
		now := time.Now()
		cred.ApiKey = hex.EncodeToString(key)
		cred.ApiSecret = hex.EncodeToString(secret)
		cred.Status = constants.ApiCredentialStatusApproved
		cred.IsActive = true
		cred.ApprovedAt = &now
		cred.RejectReason = ""
		if err := tx.Model(&userdomain.User{}).Where("id = ?", user.ID).Updates(map[string]interface{}{"member_level_id": level.ID, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Save(&cred).Error; err != nil {
			return err
		}
		result = cred
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}
