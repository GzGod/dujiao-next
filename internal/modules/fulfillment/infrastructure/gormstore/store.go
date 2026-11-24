package gormstore

import (
	"errors"
	"time"

	fulfillmentcontract "github.com/dujiao-next/internal/modules/fulfillment/contract"
	fulfillmentdomain "github.com/dujiao-next/internal/modules/fulfillment/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Store 是交付记录端口的 GORM 实现。
type Store struct{ db *gorm.DB }

var _ fulfillmentcontract.Store = (*Store)(nil)

func New(db *gorm.DB) *Store { return &Store{db: db} }

// Create 创建交付记录
func (r *Store) Create(fulfillment *fulfillmentdomain.Fulfillment) error {
	return r.db.Create(fulfillment).Error
}

// GetByOrderID 根据订单 ID 获取交付记录(不存在返回 nil, nil),不加锁。
func (r *Store) GetByOrderID(orderID uint) (*fulfillmentdomain.Fulfillment, error) {
	var existing fulfillmentdomain.Fulfillment
	if err := r.db.Where("deleted_at IS NULL AND order_id = ?", orderID).First(&existing).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &existing, nil
}

// FindByOrderIDForUpdate 用于事务内的存在性检查,加 SELECT ... FOR UPDATE 行锁防止并发双重交付。
// 返回 (record, found, err)。
func (r *Store) FindByOrderIDForUpdate(orderID uint) (*fulfillmentdomain.Fulfillment, bool, error) {
	var existing fulfillmentdomain.Fulfillment
	err := r.db.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("deleted_at IS NULL AND order_id = ?", orderID).
		First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &existing, true, nil
}

func (r *Store) FinalizeTelegramDelivery(id, admin uint, payload string, now time.Time) error {
	res := r.db.Model(&fulfillmentdomain.Fulfillment{}).Where("order_id = ? AND status = ? AND delivered_by = ? AND payload = ?", id, "pending", admin, "").Updates(map[string]interface{}{"status": "delivered", "payload": payload, "logistics_json": nil, "delivered_at": now, "updated_at": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return errors.New("delivery reservation mismatch")
	}
	return nil
}
