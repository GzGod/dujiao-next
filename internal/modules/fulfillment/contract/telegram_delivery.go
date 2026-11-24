package contract

import (
	"context"
	"errors"
	domain "github.com/dujiao-next/internal/modules/fulfillment/domain"
	"time"
)

type TelegramDeliveryStore interface {
	GetTelegramDelivery(orderID uint) (*domain.TelegramDelivery, error)
	IssueTelegramToken(orderID uint, hash string, expires time.Time) error
	BindTelegramToken(hash, chat, username string, now time.Time) (*domain.TelegramDelivery, error)
	ClaimTelegramDelivery(orderID uint, hash string) (bool, error)
	SetTelegramDeliveryResult(orderID uint, state string, messageID int64) error
}

type TelegramMessage struct{ ChatID, Text, OrderURL, SupportURL string }
type TelegramDeliverySender interface {
	SendDelivery(context.Context, TelegramMessage) (int64, error)
}

var ErrTelegramRejected = errors.New("telegram explicitly rejected delivery")
