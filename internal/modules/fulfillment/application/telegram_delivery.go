package application

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/dujiao-next/internal/constants"
	contract "github.com/dujiao-next/internal/modules/fulfillment/contract"
	domain "github.com/dujiao-next/internal/modules/fulfillment/domain"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/dujiao-next/internal/i18n"
	orderapp "github.com/dujiao-next/internal/modules/order/application"
)

var ErrTelegramDeliveryUnavailable = errors.New("telegram delivery unavailable")
var ErrTelegramDeliveryUncertain = errors.New("telegram delivery requires review")

type TelegramDeliveryService struct {
	Store                                         contract.TelegramDeliveryStore
	Sender                                        contract.TelegramDeliverySender
	Fulfillment                                   *Service
	BotUsername, TokenSecret, SiteURL, SupportURL string
}

func hashDelivery(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func (s *TelegramDeliveryService) Enabled() bool {
	return s != nil && s.Store != nil && s.Sender != nil && s.BotUsername != "" && len(s.TokenSecret) >= 32
}
func manualPaid(order *orderdomain.Order) bool {
	if order == nil || order.ResellerID != nil {
		return false
	}
	if order.Status != constants.OrderStatusPaid && order.Status != constants.OrderStatusFulfilling {
		return false
	}
	if len(order.Children) > 0 {
		for i := range order.Children {
			if manualPaid(&order.Children[i]) {
				return true
			}
		}
		return false
	}
	if len(order.Items) == 0 {
		return false
	}
	for _, item := range order.Items {
		if item.FulfillmentType != constants.FulfillmentTypeManual {
			return false
		}
	}
	return true
}
func (s *TelegramDeliveryService) startToken(id uint) string {
	mac := hmac.New(sha256.New, []byte(s.TokenSecret))
	fmt.Fprintf(mac, "delivery:%d", id)
	return "d_" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (s *TelegramDeliveryService) Link(order *orderdomain.Order) (string, error) {
	if order != nil && order.ParentID != nil {
		var err error
		order, err = s.Fulfillment.orderStore.GetByID(*order.ParentID)
		if err != nil {
			return "", err
		}
	}
	if !s.Enabled() || !manualPaid(order) {
		return "", ErrTelegramDeliveryUnavailable
	}
	token := s.startToken(order.ID)
	if err := s.Store.IssueTelegramToken(order.ID, hashDelivery(token), time.Now().Add(7*24*time.Hour)); err != nil {
		return "", err
	}
	row, err := s.Store.GetTelegramDelivery(order.ID)
	if err != nil {
		return "", err
	}
	if row == nil || row.ChatID != "" {
		return "https://t.me/" + s.BotUsername, nil
	}
	if time.Now().After(row.TokenExpiresAt) {
		return "", ErrTelegramDeliveryUnavailable
	}
	return "https://t.me/" + s.BotUsername + "?start=" + token, nil
}
func (s *TelegramDeliveryService) Bind(ctx context.Context, token, chat, username string) error {
	if !s.Enabled() || !strings.HasPrefix(token, "d_") {
		return ErrTelegramDeliveryUnavailable
	}
	row, err := s.Store.BindTelegramToken(hashDelivery(token), chat, username, time.Now())
	if err != nil {
		return ErrTelegramDeliveryUnavailable
	}
	order, err := s.Fulfillment.orderStore.GetByID(row.OrderID)
	if err != nil || !manualPaid(order) {
		return ErrTelegramDeliveryUnavailable
	}
	_, err = s.Sender.SendDelivery(ctx, contract.TelegramMessage{ChatID: chat, Text: botText(order, "telegram.delivery_bound", order.OrderNo), SupportURL: s.SupportURL})
	return err
}
func (s *TelegramDeliveryService) Status(order *orderdomain.Order) (*domain.TelegramDelivery, error) {
	if !s.Enabled() || order == nil {
		return nil, nil
	}
	id := order.ID
	if order.ParentID != nil {
		id = *order.ParentID
	}
	row, err := s.Store.GetTelegramDelivery(order.ID)
	if err != nil || row != nil && row.ChatID != "" {
		return row, err
	}
	return s.Store.GetTelegramDelivery(id)
}
func (s *TelegramDeliveryService) Send(ctx context.Context, orderID, adminID uint, payload string) (*domain.Fulfillment, error) {
	if !s.Enabled() || adminID == 0 {
		return nil, ErrTelegramDeliveryUnavailable
	}
	payload = strings.TrimSpace(payload)
	// Keep account delivery in a single atomic Telegram message.
	if payload == "" || len(utf16.Encode([]rune(payload))) > 3000 {
		return nil, ErrFulfillmentInvalid
	}
	order, err := s.Fulfillment.orderStore.GetByID(orderID)
	if err != nil || !manualPaid(order) || len(order.Children) > 0 {
		return nil, ErrOrderStatusInvalid
	}
	binding, err := s.Status(order)
	if err != nil || binding == nil || binding.ChatID == "" {
		return nil, ErrTelegramDeliveryUnavailable
	}
	// Each leaf order has its own send claim, even when the parent was bound.
	if order.ParentID != nil {
		token := s.startToken(order.ID)
		if err := s.Store.IssueTelegramToken(order.ID, hashDelivery(token), time.Now().Add(time.Hour)); err != nil {
			return nil, err
		}
		if _, err := s.Store.BindTelegramToken(hashDelivery(token), binding.ChatID, binding.Username, time.Now()); err != nil {
			return nil, err
		}
	}
	row, err := s.Store.GetTelegramDelivery(order.ID)
	if err != nil || row == nil {
		return nil, ErrTelegramDeliveryUnavailable
	}
	hash := hashDelivery(payload)
	if row.State == "sent" && row.PayloadHash == hash {
		return s.finishTelegramDelivery(order, adminID, payload, row.MessageID)
	}
	claimed, err := s.Store.ClaimTelegramDelivery(order.ID, hash)
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, ErrTelegramDeliveryUncertain
	}
	orderURL := s.SiteURL + "/guest/orders/" + order.OrderNo
	if order.UserID > 0 {
		orderURL = s.SiteURL + "/orders/" + order.OrderNo
	}
	if order.ParentID != nil {
		parent, err := s.Fulfillment.orderStore.GetByID(*order.ParentID)
		if err != nil || parent == nil {
			_ = s.Store.SetTelegramDeliveryResult(order.ID, "bound", 0)
			return nil, ErrOrderFetchFailed
		}
		orderURL = s.SiteURL + "/guest/orders/" + parent.OrderNo
		if order.UserID > 0 {
			orderURL = s.SiteURL + "/orders/" + parent.OrderNo
		}
	}
	messageID, err := s.Sender.SendDelivery(ctx, contract.TelegramMessage{ChatID: binding.ChatID, Text: botText(order, "telegram.delivery_content", order.OrderNo) + "\n\n" + payload, OrderURL: orderURL, SupportURL: s.SupportURL})
	if err != nil {
		state := "review"
		if errors.Is(err, contract.ErrTelegramRejected) {
			state = "bound"
		}
		_ = s.Store.SetTelegramDeliveryResult(order.ID, state, 0)
		return nil, ErrTelegramDeliveryUncertain
	}
	if err := s.Store.SetTelegramDeliveryResult(order.ID, "sent", messageID); err != nil {
		return nil, ErrTelegramDeliveryUncertain
	}
	return s.finishTelegramDelivery(order, adminID, payload, messageID)
}
func (s *TelegramDeliveryService) finishTelegramDelivery(order *orderdomain.Order, adminID uint, payload string, messageID int64) (*domain.Fulfillment, error) {
	result, err := s.Fulfillment.CreateManual(CreateManualInput{OrderID: order.ID, AdminID: adminID, Payload: payload})
	if err == nil {
		_ = s.Store.SetTelegramDeliveryResult(order.ID, "delivered", messageID)
		if order.ParentID != nil {
			_, _ = orderapp.SyncParentStatus(s.Fulfillment.orderStore, *order.ParentID, time.Now())
		}
	}
	return result, err
}

func botText(order *orderdomain.Order, key, no string) string {
	return fmt.Sprintf(i18n.T(order.GuestLocale, key), no)
}

// StartHelp gives customers a useful response without disclosing order data.
func (s *TelegramDeliveryService) StartHelp(ctx context.Context, chat string) {
	if !s.Enabled() {
		return
	}
	_, _ = s.Sender.SendDelivery(ctx, contract.TelegramMessage{ChatID: chat, Text: i18n.T("zh-CN", "telegram.delivery_help"), SupportURL: s.SupportURL})
}
