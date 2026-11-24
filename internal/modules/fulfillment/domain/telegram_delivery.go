package domain

import "time"

// TelegramDelivery binds one order to one private Telegram recipient. Tokens are
// hashed; payloads remain in the existing protected fulfillment record.
type TelegramDelivery struct {
	OrderID        uint       `gorm:"primaryKey" json:"order_id"`
	TokenHash      string     `gorm:"size:64;uniqueIndex" json:"-"`
	TokenExpiresAt time.Time  `json:"-"`
	ChatID         string     `gorm:"size:32" json:"-"`
	Username       string     `gorm:"size:64" json:"username"`
	BoundAt        *time.Time `json:"bound_at"`
	State          string     `gorm:"size:32" json:"state"`
	MessageID      int64      `json:"message_id"`
	PayloadHash    string     `gorm:"size:64" json:"-"`
	SentAt         *time.Time `json:"sent_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (TelegramDelivery) TableName() string { return "fulfillment_telegram_deliveries" }
