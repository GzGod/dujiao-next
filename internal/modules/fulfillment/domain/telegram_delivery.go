package domain

import "time"

// TelegramDelivery binds one order to one private Telegram recipient. Tokens are
// hashed; payloads remain in the existing protected fulfillment record.
type TelegramDelivery struct {
	NewBinding      bool       `gorm:"-" json:"-"`
	AdminNotifiedAt *time.Time `json:"-"`
	OrderID         uint       `gorm:"primaryKey" json:"order_id"`
	TokenHash       string     `gorm:"size:64;uniqueIndex" json:"-"`
	TokenExpiresAt  time.Time  `json:"-"`
	ChatID          string     `gorm:"size:32" json:"-"`
	Username        string     `gorm:"size:64" json:"username"`
	BoundAt         *time.Time `json:"bound_at"`
	State           string     `gorm:"size:32" json:"state"`
	MessageID       int64      `json:"message_id"`
	PayloadHash     string     `gorm:"size:64" json:"-"`
	SentAt          *time.Time `json:"sent_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (TelegramDelivery) TableName() string { return "fulfillment_telegram_deliveries" }

// TelegramAdminSession stores only encrypted delivery drafts; callback nonces
// expire and are scoped to the authenticated administrator's private chat.
type TelegramAdminSession struct {
	ChatID     string `gorm:"primaryKey;size:32"`
	OrderID    uint
	AdminID    uint
	Nonce      string `gorm:"size:32"`
	Ciphertext string `gorm:"type:text"`
	Stage      string `gorm:"size:20"`
	ExpiresAt  time.Time
}

func (TelegramAdminSession) TableName() string { return "fulfillment_telegram_admin_sessions" }

type TelegramUpdateReceipt struct {
	UpdateID  int64     `gorm:"primaryKey"`
	CreatedAt time.Time `gorm:"index"`
}

func (TelegramUpdateReceipt) TableName() string { return "fulfillment_telegram_updates" }
