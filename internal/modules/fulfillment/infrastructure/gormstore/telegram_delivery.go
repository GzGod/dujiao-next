package gormstore

import (
	"errors"
	domain "github.com/dujiao-next/internal/modules/fulfillment/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

func (r *Store) GetTelegramDelivery(id uint) (*domain.TelegramDelivery, error) {
	var row domain.TelegramDelivery
	err := r.db.First(&row, "order_id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}
func (r *Store) IssueTelegramToken(id uint, hash string, expires time.Time) error {
	row := domain.TelegramDelivery{OrderID: id, TokenHash: hash, TokenExpiresAt: expires, State: "unbound", UpdatedAt: time.Now()}
	return r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}
func (r *Store) BindTelegramToken(hash, chat, user string, now time.Time) (*domain.TelegramDelivery, error) {
	var row domain.TelegramDelivery
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "token_hash = ?", hash).Error; err != nil {
			return err
		}
		if row.ChatID != "" {
			if row.ChatID == chat {
				return nil
			}
			return errors.New("binding unavailable")
		}
		if !now.Before(row.TokenExpiresAt) {
			return errors.New("binding expired")
		}
		result := tx.Model(&domain.TelegramDelivery{}).Where("order_id = ? AND chat_id = ?", row.OrderID, "").Updates(map[string]interface{}{"chat_id": chat, "username": user, "bound_at": now, "state": "bound", "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("binding unavailable")
		}
		row.ChatID = chat
		row.Username = user
		row.BoundAt = &now
		row.State = "bound"
		return nil
	})
	return &row, err
}
func (r *Store) ClaimTelegramDelivery(id uint, hash string) (bool, error) {
	result := r.db.Model(&domain.TelegramDelivery{}).Where("order_id = ? AND state = ? AND chat_id <> ?", id, "bound", "").Updates(map[string]interface{}{"state": "sending", "payload_hash": hash, "updated_at": time.Now()})
	return result.RowsAffected == 1, result.Error
}
func (r *Store) SetTelegramDeliveryResult(id uint, state string, messageID int64) error {
	fields := map[string]interface{}{"state": state, "updated_at": time.Now()}
	if messageID > 0 {
		fields["message_id"] = messageID
		fields["sent_at"] = time.Now()
	}
	return r.db.Model(&domain.TelegramDelivery{}).Where("order_id = ?", id).Updates(fields).Error
}
