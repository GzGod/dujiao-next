package contract

import "time"
import fulfillmentdomain "github.com/dujiao-next/internal/modules/fulfillment/domain"

// Store 是交付记录持久化端口。
type Store interface {
	Create(fulfillment *fulfillmentdomain.Fulfillment) error
	FinalizeTelegramDelivery(id, adminID uint, payload string, now time.Time) error
	GetByOrderID(orderID uint) (*fulfillmentdomain.Fulfillment, error)
	FindByOrderIDForUpdate(orderID uint) (*fulfillmentdomain.Fulfillment, bool, error)
}
