package contract

import (
	"context"
	"errors"
	domain "github.com/dujiao-next/internal/modules/fulfillment/domain"
	"time"
)

type TelegramDeliveryStore interface {
	SaveAdminSession(*domain.TelegramAdminSession) error
	AdvanceAdminSession(chat, nonce, stage string, next *domain.TelegramAdminSession) (bool, error)
	GetAdminSession(chat string) (*domain.TelegramAdminSession, error)
	ConsumeAdminSession(chat, nonce, stage string) (bool, error)
	AcceptTelegramUpdate(id int64, chat string, now time.Time) (bool, error)
	ListTelegramPendingOrders(includeNotified bool) ([]uint, error)
	ClaimAdminNotification(id uint) (bool, error)
	GetTelegramDeliveryByToken(hash string) (*domain.TelegramDelivery, error)
	GetTelegramDelivery(orderID uint) (*domain.TelegramDelivery, error)
	IssueTelegramToken(orderID uint, hash string, expires time.Time) error
	BindTelegramToken(hash, chat, username string, now time.Time) (*domain.TelegramDelivery, error)
	ClaimTelegramDelivery(orderID uint, hash string) (bool, error)
	ReserveTelegramDelivery(orderID, adminID uint, hash string) (bool, error)
	ReleaseTelegramDelivery(orderID uint) error
	SetTelegramDeliveryResult(orderID uint, state string, messageID int64) error
}

type TelegramButton struct {
	Text     string
	Callback string
}
type TelegramMessage struct {
	ChatID, Text, OrderURL, SupportURL string
	Buttons                            []TelegramButton
}
type TelegramDeliverySender interface {
	SendDelivery(context.Context, TelegramMessage) (int64, error)
}

var ErrTelegramRejected = errors.New("telegram explicitly rejected delivery")

type TelegramCallbackAcknowledger interface {
	AnswerCallback(context.Context, string) error
}
