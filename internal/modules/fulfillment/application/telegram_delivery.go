package application

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/crypto"
	contract "github.com/dujiao-next/internal/modules/fulfillment/contract"
	domain "github.com/dujiao-next/internal/modules/fulfillment/domain"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/dujiao-next/internal/i18n"
	orderapp "github.com/dujiao-next/internal/modules/order/application"
)

var ErrTelegramDeliveryUnavailable = errors.New("telegram delivery unavailable")
var ErrTelegramDeliveryUncertain = errors.New("telegram delivery requires review")

type TelegramDeliveryService struct {
	AdminChatID                                   string
	AuthorizeAdmin                                func() (uint, error)
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
	prior, err := s.Store.GetTelegramDeliveryByToken(hashDelivery(token))
	if err != nil || prior == nil {
		return ErrTelegramDeliveryUnavailable
	}
	order, err := s.Fulfillment.orderStore.GetByID(prior.OrderID)
	if err != nil || !manualPaid(order) {
		return ErrTelegramDeliveryUnavailable
	}
	if prior.ChatID != "" {
		if prior.ChatID == chat {
			return nil
		}
		return ErrTelegramDeliveryUnavailable
	}
	bound, err := s.Store.BindTelegramToken(hashDelivery(token), chat, username, time.Now())
	if err != nil {
		return ErrTelegramDeliveryUnavailable
	}
	if !bound.NewBinding {
		return nil
	}
	_, err = s.Sender.SendDelivery(ctx, contract.TelegramMessage{ChatID: chat, Text: botText(order, "telegram.delivery_bound", order.OrderNo), SupportURL: s.SupportURL})
	// Binding is already committed. Notify only the configured, currently
	// authorized administrator; a notification failure must not undo binding.
	if _, authErr := s.adminIdentity(s.AdminChatID); authErr == nil {
		orders := []*orderdomain.Order{order}
		if len(order.Children) > 0 {
			orders = nil
			for i := range order.Children {
				orders = append(orders, &order.Children[i])
			}
		}
		for _, leaf := range orders {
			if !manualPaid(leaf) {
				continue
			}
			_ = s.sendAdminOrderNotice(ctx, leaf.ID, fmt.Sprintf(i18n.T("zh-CN", "telegram.admin_bound"), leaf.OrderNo, bound.Username, bound.ChatID),
				contract.TelegramButton{Text: i18n.T("zh-CN", "telegram.admin_21"), Callback: fmt.Sprintf("deliver:%d", leaf.ID)},
				contract.TelegramButton{Text: i18n.T("zh-CN", "telegram.mark_done"), Callback: fmt.Sprintf("markdelivered:%d", leaf.ID)})
		}
	}
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
	if err != nil || binding == nil || binding.ChatID == "" || binding.State == "review" || binding.State == "delivered" {
		return nil, ErrTelegramDeliveryUnavailable
	}
	// Re-read immediately before claiming the send slot so a refund/cancel
	// handled by another request cannot turn a stale paid order into delivery.
	latest, err := s.Fulfillment.orderStore.GetByID(orderID)
	if err != nil || !manualPaid(latest) || latest.Status == constants.OrderStatusCanceled {
		return nil, ErrOrderStatusInvalid
	}
	order = latest
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
	claimed, err := s.Store.ReserveTelegramDelivery(order.ID, adminID, hash)
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
			_ = s.Store.ReleaseTelegramDelivery(order.ID)
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
			if releaseErr := s.Store.ReleaseTelegramDelivery(order.ID); releaseErr != nil {
				return nil, ErrTelegramDeliveryUncertain
			}
			return nil, err
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
	result, err := s.Fulfillment.CreateManual(CreateManualInput{OrderID: order.ID, AdminID: adminID, Payload: payload, TelegramConfirmed: true})
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

func (s *TelegramDeliveryService) AdminEnabled() bool {
	return s.Enabled() && s.AdminChatID != "" && s.AuthorizeAdmin != nil
}
func (s *TelegramDeliveryService) adminIdentity(chat string) (uint, error) {
	if !s.AdminEnabled() || chat != s.AdminChatID {
		return 0, ErrTelegramDeliveryUnavailable
	}
	return s.AuthorizeAdmin()
}
func (s *TelegramDeliveryService) adminReply(ctx context.Context, chat, text string, buttons ...contract.TelegramButton) error {
	_, err := s.Sender.SendDelivery(ctx, contract.TelegramMessage{ChatID: chat, Text: text, Buttons: buttons})
	return err
}
func newDeliveryNonce() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func (s *TelegramDeliveryService) AdminAction(ctx context.Context, chat, data, text string) error {
	admin, err := s.adminIdentity(chat)
	if err != nil {
		return err
	}
	if data == "" && text == "/start" {
		_, err := s.Sender.SendDelivery(ctx, contract.TelegramMessage{ChatID: chat, Text: i18n.T("zh-CN", "telegram.admin_1"), AdminMenu: true})
		return err
	}
	if data == "" && (text == "/orders" || text == "/pending" || text == i18n.T("zh-CN", "telegram.pending_menu")) {
		return s.listAdminPending(ctx, 0)
	}
	if strings.HasPrefix(data, "pendingback:") {
		cursor, err := strconv.ParseUint(strings.TrimPrefix(data, "pendingback:"), 10, 64)
		if err != nil {
			return ErrFulfillmentInvalid
		}
		previous, err := s.Store.PreviousTelegramPendingCursor(uint(cursor))
		if err != nil {
			return err
		}
		return s.listAdminPending(ctx, previous)
	}
	if strings.HasPrefix(data, "pending:") {
		after, err := strconv.ParseUint(strings.TrimPrefix(data, "pending:"), 10, 64)
		if err != nil {
			return ErrFulfillmentInvalid
		}
		return s.listAdminPending(ctx, uint(after))
	}
	if strings.HasPrefix(data, "markdelivered:") {
		id, err := strconv.ParseUint(strings.TrimPrefix(data, "markdelivered:"), 10, 64)
		if err != nil || id == 0 {
			return ErrFulfillmentInvalid
		}
		order, err := s.Fulfillment.orderStore.GetByID(uint(id))
		if err != nil || !manualPaid(order) || len(order.Children) > 0 {
			return s.adminReply(ctx, chat, i18n.T("zh-CN", "telegram.admin_2"))
		}
		nonce := newDeliveryNonce()
		if nonce == "" {
			return ErrTelegramDeliveryUnavailable
		}
		session := &domain.TelegramAdminSession{ChatID: chat, OrderID: order.ID, AdminID: admin, Nonce: nonce, Stage: "mark_delivered", ExpiresAt: time.Now().Add(15 * time.Minute)}
		if err := s.Store.SaveAdminSession(session); err != nil {
			return err
		}
		return s.adminReply(ctx, chat, fmt.Sprintf(i18n.T("zh-CN", "telegram.mark_confirm"), order.OrderNo), contract.TelegramButton{Text: i18n.T("zh-CN", "telegram.mark_done"), Callback: "markconfirm:" + nonce}, contract.TelegramButton{Text: i18n.T("zh-CN", "telegram.cancel_prepare"), Callback: "cancel:" + nonce})
	}
	if strings.HasPrefix(data, "deliver:") {
		id64, err := strconv.ParseUint(strings.TrimPrefix(data, "deliver:"), 10, 64)
		if err != nil {
			return ErrFulfillmentInvalid
		}
		order, err := s.Fulfillment.orderStore.GetByID(uint(id64))
		if err != nil || !manualPaid(order) || len(order.Children) > 0 {
			return s.adminReply(ctx, chat, i18n.T("zh-CN", "telegram.admin_2"))
		}
		binding, err := s.Status(order)
		if err != nil || binding == nil || binding.ChatID == "" {
			return s.adminReply(ctx, chat, i18n.T("zh-CN", "telegram.admin_3"))
		}
		nonce := newDeliveryNonce()
		if nonce == "" {
			return ErrTelegramDeliveryUnavailable
		}
		stage := "awaiting"
		record, _ := s.Store.GetTelegramDelivery(order.ID)
		if record != nil && (record.State == "review" || record.State == "sent" || record.State == "sending") {
			if record.State == "sending" && time.Since(record.UpdatedAt) < time.Minute {
				return s.adminReply(ctx, chat, i18n.T("zh-CN", "telegram.admin_9"))
			}
			stage = "awaiting_review"
		}
		session := &domain.TelegramAdminSession{ChatID: chat, OrderID: order.ID, AdminID: admin, Nonce: nonce, Stage: stage, ExpiresAt: time.Now().Add(15 * time.Minute)}
		if err := s.Store.SaveAdminSession(session); err != nil {
			return err
		}
		return s.adminReply(ctx, chat, i18n.T("zh-CN", "telegram.admin_4")+order.OrderNo+i18n.T("zh-CN", "telegram.admin_5")+binding.Username+" · "+binding.ChatID+i18n.T("zh-CN", "telegram.admin_6"), contract.TelegramButton{Text: i18n.T("zh-CN", "telegram.cancel_prepare"), Callback: "cancel:" + nonce})
	}
	session, err := s.Store.GetAdminSession(chat)
	if err != nil || session == nil || session.AdminID != admin || time.Now().After(session.ExpiresAt) {
		return s.adminReply(ctx, chat, i18n.T("zh-CN", "telegram.admin_7"))
	}
	if data == "cancel:"+session.Nonce {
		ok, err := s.Store.ConsumeAdminSession(chat, session.Nonce, session.Stage)
		if err != nil {
			return err
		}
		if ok {
			return s.adminReply(ctx, chat, i18n.T("zh-CN", "telegram.admin_8"))
		}
		return nil
	}
	if session.Stage == "mark_delivered" && data == "markconfirm:"+session.Nonce {
		ok, err := s.Store.ConsumeAdminSession(chat, session.Nonce, "mark_delivered")
		if err != nil || !ok {
			return ErrTelegramDeliveryUnavailable
		}
		order, err := s.Fulfillment.orderStore.GetByID(session.OrderID)
		if err != nil || !manualPaid(order) || len(order.Children) > 0 {
			return ErrOrderStatusInvalid
		}
		_, err = s.Fulfillment.CreateManual(CreateManualInput{OrderID: order.ID, AdminID: admin, Payload: i18n.T(order.GuestLocale, "telegram.mark_record")})
		if err != nil {
			return s.adminReply(ctx, chat, i18n.T("zh-CN", "telegram.mark_failed"))
		}
		if order.ParentID != nil {
			_, _ = orderapp.SyncParentStatus(s.Fulfillment.orderStore, *order.ParentID, time.Now())
		}
		return s.adminReply(ctx, chat, i18n.T("zh-CN", "telegram.admin_10"))
	}
	if session.Stage == "review" && (data == "received:"+session.Nonce || data == "unsent:"+session.Nonce) {
		ok, err := s.Store.ConsumeAdminSession(chat, session.Nonce, "review")
		if err != nil || !ok {
			return ErrTelegramDeliveryUnavailable
		}
		key := sha256.Sum256([]byte(s.TokenSecret))
		payload, err := crypto.Decrypt(key[:], session.Ciphertext)
		if err != nil {
			return err
		}
		row, err := s.Store.GetTelegramDelivery(session.OrderID)
		if err != nil || row == nil || row.PayloadHash != hashDelivery(payload) || (row.State != "review" && row.State != "sending" && row.State != "sent") {
			return ErrTelegramDeliveryUnavailable
		}
		if data == "received:"+session.Nonce {
			order, err := s.Fulfillment.orderStore.GetByID(session.OrderID)
			if err != nil {
				return err
			}
			_, err = s.finishTelegramDelivery(order, admin, payload, row.MessageID)
			if err != nil {
				return err
			}
			return s.adminReply(ctx, chat, i18n.T("zh-CN", "telegram.admin_10"))
		}
		if err := s.Store.ReleaseTelegramDelivery(session.OrderID); err != nil {
			return err
		}
		// Human confirmed no delivery. Require a fresh content/confirmation
		// cycle; never retry the network send from this recovery button.
		return s.AdminAction(ctx, chat, fmt.Sprintf("deliver:%d", session.OrderID), "")
	}
	if data == "confirm:"+session.Nonce && session.Stage == "confirm" {
		ok, err := s.Store.ConsumeAdminSession(chat, session.Nonce, "confirm")
		if err != nil || !ok {
			return ErrTelegramDeliveryUnavailable
		}
		key := sha256.Sum256([]byte(s.TokenSecret))
		payload, err := crypto.Decrypt(key[:], session.Ciphertext)
		if err != nil {
			return err
		}
		_, err = s.Send(ctx, session.OrderID, admin, payload)
		if err != nil {
			row, _ := s.Store.GetTelegramDelivery(session.OrderID)
			if row != nil && (row.State == "review" || row.State == "sending" || row.State == "sent") {
				previousNonce := session.Nonce
				session.Stage = "review"
				session.Nonce = newDeliveryNonce()
				session.ExpiresAt = time.Now().Add(24 * time.Hour)
				advanced, err := s.Store.AdvanceAdminSession(chat, previousNonce, "consumed", session)
				if err != nil {
					return err
				}
				if !advanced {
					return ErrTelegramDeliveryUncertain
				}
				return s.adminReply(ctx, chat, i18n.T("zh-CN", "telegram.admin_9"), contract.TelegramButton{Text: i18n.T("zh-CN", "telegram.review_received"), Callback: "received:" + session.Nonce}, contract.TelegramButton{Text: i18n.T("zh-CN", "telegram.review_unsent"), Callback: "unsent:" + session.Nonce})
			}
			return s.adminReply(ctx, chat, i18n.T("zh-CN", "telegram.admin_9"))
		}
		return s.adminReply(ctx, chat, i18n.T("zh-CN", "telegram.admin_10"))
	}
	if data != "" || (session.Stage != "awaiting" && session.Stage != "awaiting_review") {
		return nil
	}
	payload := strings.TrimSpace(text)
	if payload == "" || len(utf16.Encode([]rune(payload))) > 3000 {
		return s.adminReply(ctx, chat, i18n.T("zh-CN", "telegram.admin_11"))
	}
	key := sha256.Sum256([]byte(s.TokenSecret))
	encrypted, err := crypto.Encrypt(key[:], payload)
	if err != nil {
		return err
	}
	previousStage := session.Stage
	if previousStage == "awaiting_review" {
		record, err := s.Store.GetTelegramDelivery(session.OrderID)
		if err != nil || record == nil || record.PayloadHash != hashDelivery(payload) {
			return s.adminReply(ctx, chat, i18n.T("zh-CN", "telegram.admin_9"))
		}
	}
	previousNonce := session.Nonce
	session.Ciphertext = encrypted
	session.Stage = "confirm"
	if previousStage == "awaiting_review" {
		session.Stage = "review"
	}
	session.Nonce = newDeliveryNonce()
	if session.Nonce == "" {
		return ErrTelegramDeliveryUnavailable
	}
	advanced, err := s.Store.AdvanceAdminSession(chat, previousNonce, previousStage, session)
	if err != nil {
		return err
	}
	if !advanced {
		return ErrTelegramDeliveryUnavailable
	}
	order, err := s.Fulfillment.orderStore.GetByID(session.OrderID)
	if err != nil || !manualPaid(order) {
		return ErrOrderStatusInvalid
	}
	binding, err := s.Status(order)
	if err != nil || binding == nil {
		return ErrTelegramDeliveryUnavailable
	}
	if session.Stage == "review" {
		return s.adminReply(ctx, chat, i18n.T("zh-CN", "telegram.admin_9"), contract.TelegramButton{Text: i18n.T("zh-CN", "telegram.review_received"), Callback: "received:" + session.Nonce}, contract.TelegramButton{Text: i18n.T("zh-CN", "telegram.review_unsent"), Callback: "unsent:" + session.Nonce})
	}
	return s.adminReply(ctx, chat, i18n.T("zh-CN", "telegram.admin_12")+order.OrderNo+i18n.T("zh-CN", "telegram.admin_5")+binding.Username+" · "+binding.ChatID+"\n\n"+payload, contract.TelegramButton{Text: i18n.T("zh-CN", "telegram.admin_13"), Callback: "confirm:" + session.Nonce}, contract.TelegramButton{Text: i18n.T("zh-CN", "telegram.admin_14"), Callback: "cancel:" + session.Nonce})
}

// NotifyAdminOrders is invoked outside database transactions by the bootstrap
// poller. Only the configured private administrator chat can receive these.
func (s *TelegramDeliveryService) NotifyAdminOrders(ctx context.Context, force bool) error {
	if err := s.SyncAdminNotices(ctx); err != nil {
		return err
	}
	if _, err := s.adminIdentity(s.AdminChatID); err != nil {
		return err
	}
	ids, err := s.Store.ListTelegramPendingOrders(force)
	if err != nil {
		return err
	}
	if len(ids) == 0 && force {
		return s.adminReply(ctx, s.AdminChatID, i18n.T("zh-CN", "telegram.admin_15"))
	}
	for _, id := range ids {
		order, err := s.Fulfillment.orderStore.GetByID(id)
		if err != nil || !manualPaid(order) {
			continue
		}
		token := s.startToken(id)
		if err := s.Store.IssueTelegramToken(id, hashDelivery(token), time.Now().Add(7*24*time.Hour)); err != nil {
			continue
		}
		if !force {
			claimed, err := s.Store.ClaimAdminNotification(id)
			if err != nil || !claimed {
				continue
			}
		}
		var titles []string
		for _, item := range order.Items {
			if title, ok := item.TitleJSON["zh-CN"].(string); ok {
				titles = append(titles, title)
			}
		}
		binding, _ := s.Status(order)
		status := i18n.T("zh-CN", "telegram.admin_16")
		if binding != nil && binding.ChatID != "" {
			status = i18n.T("zh-CN", "telegram.admin_17") + binding.Username + " · " + binding.ChatID
		}
		text := i18n.T("zh-CN", "telegram.admin_18") + order.OrderNo + i18n.T("zh-CN", "telegram.admin_19") + strings.Join(titles, " / ") + i18n.T("zh-CN", "telegram.admin_20") + order.TotalAmount.String() + " " + order.Currency + "\n" + status
		if err := s.sendAdminOrderNotice(ctx, id, text, contract.TelegramButton{Text: i18n.T("zh-CN", "telegram.admin_21"), Callback: fmt.Sprintf("deliver:%d", id)}, contract.TelegramButton{Text: i18n.T("zh-CN", "telegram.mark_done"), Callback: fmt.Sprintf("markdelivered:%d", id)}); err != nil {
			return err
		}
	}
	return nil
}

func (s *TelegramDeliveryService) sendAdminOrderNotice(ctx context.Context, orderID uint, text string, buttons ...contract.TelegramButton) error {
	if _, err := s.adminIdentity(s.AdminChatID); err != nil {
		return err
	}
	id, err := s.Sender.SendDelivery(ctx, contract.TelegramMessage{ChatID: s.AdminChatID, Text: text, Buttons: buttons, Entities: s.adminCustomerLink(orderID, text)})
	if err != nil {
		return err
	}
	return s.Store.SaveAdminNotice(&domain.TelegramAdminNotice{OrderID: orderID, ChatID: s.AdminChatID, MessageID: id, UpdatedAt: time.Now()})
}

// SyncAdminNotices reconciles messages with website changes, including refunds
// performed outside the bot. No customer data or delivery payload is included.
func (s *TelegramDeliveryService) SyncAdminNotices(ctx context.Context) error {
	if _, err := s.adminIdentity(s.AdminChatID); err != nil {
		return err
	}
	editor, ok := s.Sender.(contract.TelegramMessageEditor)
	if !ok {
		return nil
	}
	rows, err := s.Store.ListAdminNotices(s.AdminChatID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		order, err := s.Fulfillment.orderStore.GetByID(row.OrderID)
		if err != nil || order == nil {
			continue
		}
		status := order.Status
		if order.RefundedAmount.Decimal.IsPositive() && status != "refunded" {
			status = "partially_refunded"
		}
		binding, _ := s.Status(order)
		identity := ""
		if binding != nil && binding.ChatID != "" {
			identity = binding.Username + " · " + binding.ChatID
		}
		fingerprint := status + "|" + identity
		terminal := status == "delivered" || status == "completed" || status == "refunded" || status == "canceled"
		if terminal {
			fingerprint = status
		}
		if row.Status == fingerprint {
			continue
		}
		label := i18n.T("zh-CN", "telegram.notice_"+status)
		if label == "telegram.notice_"+status {
			label = status
		}
		text := fmt.Sprintf("%s\n%s%s", label, i18n.T("zh-CN", "telegram.admin_18"), order.OrderNo)
		if identity != "" {
			text += "\n" + i18n.T("zh-CN", "telegram.admin_17") + identity
		}
		buttons := []contract.TelegramButton{}
		if manualPaid(order) && !terminal && status != "partially_refunded" {
			buttons = append(buttons, contract.TelegramButton{Text: i18n.T("zh-CN", "telegram.admin_21"), Callback: fmt.Sprintf("deliver:%d", order.ID)}, contract.TelegramButton{Text: i18n.T("zh-CN", "telegram.mark_done"), Callback: fmt.Sprintf("markdelivered:%d", order.ID)})
		}
		if err := editor.EditDelivery(ctx, row.MessageID, contract.TelegramMessage{ChatID: s.AdminChatID, Text: text, Buttons: buttons, Entities: s.adminCustomerLink(row.OrderID, text)}); err != nil {
			continue
		}
		if err := s.Store.UpdateAdminNotice(row.ID, fingerprint); err != nil {
			return err
		}
	}
	return nil
}

// listAdminPending uses a stable ID cursor, so delivery of earlier orders does
// not cause later pending orders to be skipped while paging.
func (s *TelegramDeliveryService) listAdminPending(ctx context.Context, after uint) error {
	if _, err := s.adminIdentity(s.AdminChatID); err != nil {
		return err
	}
	ids, err := s.Store.ListTelegramPendingPage(after)
	if err != nil {
		return err
	}
	more := len(ids) > 10
	if more {
		ids = ids[:10]
	}
	if len(ids) == 0 {
		return s.adminReply(ctx, s.AdminChatID, i18n.T("zh-CN", "telegram.admin_15"), contract.TelegramButton{Text: i18n.T("zh-CN", "telegram.pending_all"), Callback: "pending:0"})
	}
	for _, id := range ids {
		order, err := s.Fulfillment.orderStore.GetByID(id)
		if err != nil || !manualPaid(order) {
			continue
		}
		status := i18n.T("zh-CN", "telegram.admin_16")
		if binding, _ := s.Status(order); binding != nil && binding.ChatID != "" {
			status = i18n.T("zh-CN", "telegram.admin_17") + binding.Username + " · " + binding.ChatID
		}
		titles := []string{}
		for _, item := range order.Items {
			if title, ok := item.TitleJSON["zh-CN"].(string); ok {
				titles = append(titles, title)
			}
		}
		message := i18n.T("zh-CN", "telegram.notice_"+order.Status) + "\n" + order.OrderNo + "\n" + strings.Join(titles, " / ") + "\n" + order.TotalAmount.String() + " " + order.Currency + "\n" + status
		if err := s.sendAdminOrderNotice(ctx, id, message, contract.TelegramButton{Text: i18n.T("zh-CN", "telegram.admin_21"), Callback: fmt.Sprintf("deliver:%d", id)}, contract.TelegramButton{Text: i18n.T("zh-CN", "telegram.mark_done"), Callback: fmt.Sprintf("markdelivered:%d", id)}); err != nil {
			return err
		}
	}
	buttons := []contract.TelegramButton{{Text: i18n.T("zh-CN", "telegram.pending_refresh"), Callback: "pending:0"}}
	if after > 0 {
		buttons = append(buttons, contract.TelegramButton{Text: i18n.T("zh-CN", "telegram.pending_previous"), Callback: fmt.Sprintf("pendingback:%d", after)})
	}
	if more {
		buttons = append(buttons, contract.TelegramButton{Text: i18n.T("zh-CN", "telegram.pending_next"), Callback: fmt.Sprintf("pending:%d", ids[len(ids)-1])})
	}
	return s.adminReply(ctx, s.AdminChatID, i18n.T("zh-CN", "telegram.pending_footer"), buttons...)
}

// Telegram text entities use UTF-16 offsets; names remain plain text, never HTML.
func (s *TelegramDeliveryService) adminCustomerLink(orderID uint, text string) []contract.TelegramEntity {
	order, err := s.Fulfillment.orderStore.GetByID(orderID)
	if err != nil || order == nil {
		return nil
	}
	binding, err := s.Status(order)
	if err != nil || binding == nil {
		return nil
	}
	id, err := strconv.ParseInt(binding.ChatID, 10, 64)
	if err != nil || id <= 0 {
		return nil
	}
	label := binding.Username + " · " + binding.ChatID
	index := strings.LastIndex(text, label)
	if index < 0 {
		label = binding.ChatID
		index = strings.LastIndex(text, label)
	}
	if index < 0 {
		return nil
	}
	return []contract.TelegramEntity{{Type: "text_link", Offset: len(utf16.Encode([]rune(text[:index]))), Length: len(utf16.Encode([]rune(label))), URL: fmt.Sprintf("tg://user?id=%d", id)}}
}
