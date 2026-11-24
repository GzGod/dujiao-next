package gormstore

import (
	"errors"
	"fmt"
	domain "github.com/dujiao-next/internal/modules/fulfillment/domain"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	"github.com/dujiao-next/internal/shared/jsonmap"
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
		var lookup domain.TelegramDelivery
		if err := tx.First(&lookup, "token_hash = ?", hash).Error; err != nil {
			return err
		}
		var initial orderdomain.Order
		if err := tx.First(&initial, "id = ? AND deleted_at IS NULL", lookup.OrderID).Error; err != nil {
			return err
		}
		root := initial.ID
		if initial.ParentID != nil {
			root = *initial.ParentID
		}
		var order orderdomain.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, "id = ? AND deleted_at IS NULL", root).Error; err != nil {
			return err
		}
		if (order.Status != "paid" && order.Status != "fulfilling") || order.RefundedAmount.Decimal.IsPositive() || order.ResellerID != nil {
			return errors.New("order unavailable")
		}
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
		row.NewBinding = true
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

func (r *Store) GetTelegramDeliveryByToken(hash string) (*domain.TelegramDelivery, error) {
	var row domain.TelegramDelivery
	err := r.db.First(&row, "token_hash = ?", hash).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *Store) SaveAdminSession(s *domain.TelegramAdminSession) error { return r.db.Save(s).Error }
func (r *Store) GetAdminSession(chat string) (*domain.TelegramAdminSession, error) {
	var s domain.TelegramAdminSession
	err := r.db.First(&s, "chat_id = ?", chat).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &s, err
}
func (r *Store) ConsumeAdminSession(chat, nonce, stage string) (bool, error) {
	res := r.db.Model(&domain.TelegramAdminSession{}).Where("chat_id = ? AND nonce = ? AND stage = ? AND expires_at > ?", chat, nonce, stage, time.Now()).Update("stage", "consumed")
	return res.RowsAffected == 1, res.Error
}
func (r *Store) AcceptTelegramUpdate(id int64, chat string, now time.Time) (bool, error) {
	if id <= 0 {
		return false, nil
	}
	accepted := false
	err := r.db.Transaction(func(tx *gorm.DB) error {
		row := domain.TelegramUpdateReceipt{UpdateID: id, CreatedAt: now}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		if chat == "" {
			accepted = true
			return nil
		}
		// Per-chat, one bot action per second. Share the dedupe table, avoiding an
		// in-memory limiter that resets or diverges across deployment replicas.
		var prior domain.TelegramUpdateReceipt
		res = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&prior, "update_id = ?", -chatKey(chat))
		if res.Error != nil && !errors.Is(res.Error, gorm.ErrRecordNotFound) {
			return res.Error
		}
		if res.Error == nil && now.Sub(prior.CreatedAt) < time.Second {
			return nil
		}
		prior = domain.TelegramUpdateReceipt{UpdateID: -chatKey(chat), CreatedAt: now}
		if err := tx.Save(&prior).Error; err != nil {
			return err
		}
		accepted = true
		return tx.Where("created_at < ?", now.Add(-48*time.Hour)).Delete(&domain.TelegramUpdateReceipt{}).Error
	})
	return accepted, err
}
func chatKey(chat string) int64 {
	var id int64
	_, _ = fmt.Sscan(chat, &id)
	if id <= 0 {
		return 1
	}
	return id
}
func (r *Store) ListTelegramPendingOrders(includeNotified bool) ([]uint, error) {
	if err := r.db.Where("expires_at < ?", time.Now()).Delete(&domain.TelegramAdminSession{}).Error; err != nil {
		return nil, err
	}
	var ids []uint
	query := r.db.Model(&orderdomain.Order{}).Where("orders.deleted_at IS NULL AND orders.reseller_id IS NULL AND orders.status IN ?", []string{"paid", "fulfilling"}).Where("NOT EXISTS (SELECT 1 FROM orders c WHERE c.parent_id = orders.id AND c.deleted_at IS NULL)").Where("EXISTS (SELECT 1 FROM order_items oi WHERE oi.order_id = orders.id AND oi.fulfillment_type = ?)", "manual")
	if !includeNotified {
		query = query.Where("NOT EXISTS (SELECT 1 FROM fulfillment_telegram_deliveries td WHERE td.order_id = orders.id AND td.admin_notified_at IS NOT NULL)")
	}
	err := query.Order("orders.id ASC").Limit(30).Pluck("orders.id", &ids).Error
	return ids, err
}
func (r *Store) ClaimAdminNotification(id uint) (bool, error) {
	res := r.db.Model(&domain.TelegramDelivery{}).Where("order_id = ? AND admin_notified_at IS NULL", id).Update("admin_notified_at", time.Now())
	return res.RowsAffected == 1, res.Error
}

func (r *Store) ReserveTelegramDelivery(id, admin uint, hash string) (bool, error) {
	claimed := false
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var initial orderdomain.Order
		if err := tx.First(&initial, "id = ? AND deleted_at IS NULL", id).Error; err != nil {
			return err
		}
		root := id
		if initial.ParentID != nil {
			root = *initial.ParentID
		}
		var parent orderdomain.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&parent, "id = ? AND deleted_at IS NULL", root).Error; err != nil {
			return err
		}
		var order orderdomain.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Items", "deleted_at IS NULL").First(&order, "id = ? AND deleted_at IS NULL", id).Error; err != nil {
			return err
		}
		if (parent.Status != "paid" && parent.Status != "fulfilling") || (order.Status != "paid" && order.Status != "fulfilling") || order.RefundedAmount.Decimal.IsPositive() || parent.RefundedAmount.Decimal.IsPositive() || order.ResellerID != nil {
			return errors.New("order unavailable")
		}
		if len(order.Items) == 0 {
			return errors.New("order unavailable")
		}
		for _, item := range order.Items {
			if item.FulfillmentType != "manual" {
				return errors.New("order not manual")
			}
		}
		var count int64
		if err := tx.Model(&domain.Fulfillment{}).Where("order_id = ? AND deleted_at IS NULL", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		ok, err := New(tx).ClaimTelegramDelivery(id, hash)
		if err != nil || !ok {
			return err
		}
		record := domain.Fulfillment{OrderID: id, Type: "manual", Status: "pending", Payload: "", LogisticsJSON: jsonmap.JSON{"telegram_payload_hash": hash}, DeliveredBy: &admin, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		claimed = true
		return nil
	})
	return claimed, err
}
func (r *Store) ReleaseTelegramDelivery(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("order_id = ? AND status = ?", id, "pending").Delete(&domain.Fulfillment{}).Error; err != nil {
			return err
		}
		return New(tx).SetTelegramDeliveryResult(id, "bound", 0)
	})
}

func (r *Store) AdvanceAdminSession(chat, nonce, stage string, next *domain.TelegramAdminSession) (bool, error) {
	res := r.db.Model(&domain.TelegramAdminSession{}).Where("chat_id = ? AND nonce = ? AND stage = ? AND expires_at > ?", chat, nonce, stage, time.Now()).Updates(map[string]interface{}{"nonce": next.Nonce, "ciphertext": next.Ciphertext, "stage": next.Stage, "expires_at": next.ExpiresAt})
	return res.RowsAffected == 1, res.Error
}
