package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	orderapp "github.com/dujiao-next/internal/modules/order/application"
	ordercontract "github.com/dujiao-next/internal/modules/order/contract"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	riskcontract "github.com/dujiao-next/internal/modules/orderrisk/contract"
	paymentcontract "github.com/dujiao-next/internal/modules/payment/contract"
	procurementcontract "github.com/dujiao-next/internal/modules/procurement/contract"
	settingssecurity "github.com/dujiao-next/internal/modules/settings/schema/security"

	paymentdomain "github.com/dujiao-next/internal/modules/payment/domain"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/i18n"
	"github.com/dujiao-next/internal/logger"
	notificationcontract "github.com/dujiao-next/internal/modules/notification/contract"
	"github.com/dujiao-next/internal/shared/jsonmap"
)

// CapturePaymentInput 捕获支付输入。
type CapturePaymentInput struct {
	PaymentID uint
	Context   context.Context
}

func (s *PaymentService) CapturePayment(input CapturePaymentInput) (*paymentdomain.Payment, error) {
	if input.PaymentID == 0 {
		return nil, ErrPaymentInvalid
	}
	payment, err := s.paymentRepo.GetByID(input.PaymentID)
	if err != nil {
		return nil, ErrPaymentUpdateFailed
	}
	if payment == nil {
		return nil, ErrPaymentNotFound
	}
	if payment.Status == constants.PaymentStatusSuccess {
		return payment, nil
	}

	channel, err := s.channelRepo.GetByID(payment.ChannelID)
	if err != nil {
		return nil, ErrPaymentUpdateFailed
	}
	if channel == nil {
		return nil, ErrPaymentChannelNotFound
	}

	providerType := strings.ToLower(strings.TrimSpace(channel.ProviderType))
	if providerType != constants.PaymentProviderOfficial {
		return nil, ErrPaymentProviderNotSupported
	}
	if strings.TrimSpace(payment.ProviderRef) == "" {
		return nil, ErrPaymentInvalid
	}

	// 统一通过 Registry 路由。Registry.Lookup 会返回 channel 对应的 adapter,
	// 如果 adapter 不实现 Capturer,type assertion 失败,返回 ErrPaymentProviderNotSupported。
	// 因此无需在此显式检查 channel 是否支持 capture。
	return s.captureViaRegistry(input, payment, channel)
}

// captureViaRegistry 通过 PaymentProviderRegistry 路由调用 QueryPayment。
// stripe + paypal + wechat 实现了 paymentcontract.GatewayCapturer 接口,其它 channel
// (alipay / epay / epusdt / bepusdt / tokenpay / okpay) 仅实现 webhook 回调,
// type assertion 失败时返回 ErrPaymentProviderNotSupported。
func (s *PaymentService) captureViaRegistry(input CapturePaymentInput, payment *paymentdomain.Payment, channel *paymentdomain.PaymentChannel) (*paymentdomain.Payment, error) {
	logger.Infow("payment_capture_via_registry",
		"payment_id", payment.ID,
		"provider_type", channel.ProviderType,
		"channel_type", channel.ChannelType,
	)
	if s.paymentProviderRegistry == nil {
		return nil, ErrPaymentProviderNotSupported
	}
	p, ok := s.paymentProviderRegistry.Lookup(channel.ProviderType, channel.ChannelType)
	if !ok {
		return nil, ErrPaymentProviderNotSupported
	}
	capturer, ok := p.(paymentcontract.GatewayCapturer)
	if !ok {
		logger.Warnw("payment_provider_capture_not_implemented",
			"provider_type", channel.ProviderType,
			"channel_type", channel.ChannelType,
		)
		return nil, ErrPaymentProviderNotSupported
	}

	// 第二参数是 interactionMode,不是 channelType。stripe/wechat adapter
	// 会拒绝任何非法 mode,传 channelType 会导致  一律 ErrConfigInvalid。
	if err := capturer.ValidateConfig(channel.ConfigJSON, channel.InteractionMode); err != nil {
		return nil, mapProviderErrorToService(err)
	}

	ctx, cancel := detachOutboundRequestContext(input.Context)
	defer cancel()

	queryResult, err := capturer.QueryPayment(ctx, channel.ConfigJSON, payment.ProviderRef)
	if err != nil {
		return nil, mapProviderErrorToService(err)
	}

	payload := jsonmap.JSON{}
	if queryResult.Payload != nil {
		payload = queryResult.Payload
	}
	status := strings.TrimSpace(queryResult.Status)
	if status == "" {
		status = constants.PaymentStatusPending
	}

	callbackInput := PaymentCallbackInput{
		PaymentID:   payment.ID,
		ChannelID:   channel.ID,
		Status:      status,
		ProviderRef: pickFirstNonEmpty(queryResult.ProviderRef, payment.ProviderRef),
		Amount:      queryResult.Amount,
		Currency:    strings.ToUpper(strings.TrimSpace(queryResult.Currency)),
		PaidAt:      queryResult.PaidAt,
		Payload:     payload,
	}
	return s.HandleCallback(callbackInput)
}

// SettleSignedPaymentInput 付款人客户端提交签名载荷（如 x402 钱包签名）后由服务端结算的输入。
type SettleSignedPaymentInput struct {
	PaymentID uint
	Signed    jsonmap.JSON
	Context   context.Context
}

// SettleSignedPayment 把客户端签名载荷交给网关结算；结算成功后走与回调相同的事务路径确认支付。
// 金额、收款方以创建支付时保存的快照为准，网关结算失败时支付状态保持不变，可重新签名提交。
func (s *PaymentService) SettleSignedPayment(input SettleSignedPaymentInput) (*paymentdomain.Payment, error) {
	if input.PaymentID == 0 || len(input.Signed) == 0 {
		return nil, ErrPaymentInvalid
	}
	payment, err := s.paymentRepo.GetByID(input.PaymentID)
	if err != nil {
		return nil, ErrPaymentUpdateFailed
	}
	if payment == nil {
		return nil, ErrPaymentNotFound
	}
	if payment.Status == constants.PaymentStatusSuccess {
		return payment, nil
	}
	if payment.Status != constants.PaymentStatusPending {
		return nil, ErrPaymentStatusInvalid
	}
	if payment.ExpiredAt != nil && time.Now().After(*payment.ExpiredAt) {
		return nil, ErrPaymentStatusInvalid
	}
	channel, err := s.channelRepo.GetByID(payment.ChannelID)
	if err != nil {
		return nil, ErrPaymentUpdateFailed
	}
	if channel == nil {
		return nil, ErrPaymentChannelNotFound
	}
	if s.paymentProviderRegistry == nil {
		return nil, ErrPaymentProviderNotSupported
	}
	p, ok := s.paymentProviderRegistry.Lookup(channel.ProviderType, channel.ChannelType)
	if !ok {
		return nil, ErrPaymentProviderNotSupported
	}
	settler, ok := p.(paymentcontract.GatewayPayloadSettler)
	if !ok {
		return nil, ErrPaymentProviderNotSupported
	}

	ctx, cancel := detachOutboundRequestContext(input.Context)
	defer cancel()
	result, err := settler.SettlePayload(ctx, channel.ConfigJSON, payment.ProviderPayload, input.Signed)
	if err != nil {
		logger.Warnw("payment_settle_signed_failed",
			"payment_id", payment.ID,
			"provider_type", channel.ProviderType,
			"error", err,
		)
		return nil, mapProviderErrorToService(err)
	}
	logger.Infow("payment_settle_signed_success",
		"payment_id", payment.ID,
		"provider_type", channel.ProviderType,
		"provider_ref", result.ProviderRef,
	)
	return s.HandleCallback(PaymentCallbackInput{
		PaymentID:   payment.ID,
		ChannelID:   channel.ID,
		Status:      result.Status,
		ProviderRef: result.ProviderRef,
		Amount:      result.Amount,
		Currency:    result.Currency,
		PaidAt:      result.PaidAt,
		Payload:     result.Payload,
	})
}

// mapProviderErrorToService 把 provider.ErrXxx 转换为 service 层的 ErrPaymentXxx。
func mapProviderErrorToService(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, paymentcontract.ErrGatewayConfigInvalid):
		return fmt.Errorf("%w: %v", ErrPaymentChannelConfigInvalid, err)
	case errors.Is(err, paymentcontract.ErrGatewayRequestFailed), errors.Is(err, paymentcontract.ErrGatewayAuthFailed):
		return fmt.Errorf("%w: %v", ErrPaymentGatewayRequestFailed, err)
	case errors.Is(err, paymentcontract.ErrGatewayResponseInvalid), errors.Is(err, paymentcontract.ErrGatewaySignatureInvalid):
		return fmt.Errorf("%w: %v", ErrPaymentGatewayResponseInvalid, err)
	case errors.Is(err, paymentcontract.ErrGatewayUnsupportedChannel), errors.Is(err, paymentcontract.ErrGatewayProviderNotFound):
		return ErrPaymentProviderNotSupported
	default:
		return fmt.Errorf("%w: %v", ErrPaymentGatewayRequestFailed, err)
	}
}

// ManualConfirmPaymentInput 后台人工确认支付输入。
type ManualConfirmPaymentInput struct {
	OrderID     uint
	PaymentID   uint // 必填：明确要确认的那一条支付记录
	AdminID     uint
	ProviderRef string // 第三方支付流水号 / 链上交易 Hash（可选）
	Remark      string
}

// ManualConfirmPayment 管理员在支付渠道侧核实到账后，手动把订单恢复为已支付并继续履约。
//
// 它复用支付回调的事务路径（金额/币种校验、库存消耗、分销利润入账），
// 提交后同样触发自动发货、邮件、通知、采购等后续流程，不是简单改订单状态。
// 订单已被超时取消时，会在同一事务内重开订单并重新占用自动发货卡密，库存不足则整体回滚。
func (s *PaymentService) ManualConfirmPayment(input ManualConfirmPaymentInput) (*paymentdomain.Payment, error) {
	if input.OrderID == 0 || input.AdminID == 0 || input.PaymentID == 0 {
		return nil, ErrPaymentInvalid
	}
	order, err := s.orderRepo.GetByID(input.OrderID)
	if err != nil {
		return nil, orderapp.ErrOrderFetchFailed
	}
	if order == nil {
		return nil, orderapp.ErrOrderNotFound
	}
	if order.ParentID != nil || order.PaidAt != nil ||
		(order.Status != constants.OrderStatusPendingPayment && order.Status != constants.OrderStatusCanceled) {
		return nil, orderapp.ErrOrderStatusInvalid
	}

	payment, err := s.loadManualConfirmPayment(order.ID, input.PaymentID)
	if err != nil {
		return nil, err
	}

	ref := strings.TrimSpace(input.ProviderRef)
	callbackRef := ""
	if strings.TrimSpace(payment.ProviderRef) == "" {
		callbackRef = ref
	}
	now := time.Now()
	return s.HandleCallback(PaymentCallbackInput{
		PaymentID:   payment.ID,
		OrderNo:     order.OrderNo,
		ChannelID:   payment.ChannelID,
		Status:      constants.PaymentStatusSuccess,
		ProviderRef: callbackRef,
		Amount:      payment.Amount,
		Currency:    payment.Currency,
		Payload: jsonmap.JSON{
			"manual_confirm": jsonmap.JSON{
				"admin_id":                input.AdminID,
				"remark":                  strings.TrimSpace(input.Remark),
				"provider_ref":            ref,
				"previous_order_status":   order.Status,
				"previous_payment_status": payment.Status,
				"confirmed_at":            now.Format(time.RFC3339),
			},
		},
		manualConfirm: true,
	})
}

// loadManualConfirmPayment 取出管理员明确指定要确认的支付记录：必须属于该订单。
// 异常码为 underpaid_payment_succeeded 的记录，款项已转入用户余额，不能再拿来履约，否则会重复给付。
func (s *PaymentService) loadManualConfirmPayment(orderID, paymentID uint) (*paymentdomain.Payment, error) {
	payment, err := s.paymentRepo.GetByID(paymentID)
	if err != nil {
		return nil, ErrPaymentUpdateFailed
	}
	if payment == nil {
		return nil, ErrPaymentNotFound
	}
	if payment.OrderID != orderID {
		return nil, ErrPaymentInvalid
	}
	if payment.ExceptionCode == constants.PaymentExceptionUnderpaidSucceeded {
		return nil, ErrPaymentUnderpaidCredited
	}
	return payment, nil
}

// ReviewOrderRisk resolves a hold without replaying the original payment.
func (s *PaymentService) ReviewOrderRisk(orderID, adminID uint, approve bool) error {
	return s.ReviewOrderRiskWithList(orderID, adminID, approve, nil)
}

func (s *PaymentService) ReviewOrderRiskWithList(orderID, adminID uint, approve bool, kinds []string) error {
	if orderID == 0 || adminID == 0 || len(kinds) > 4 {
		return ErrPaymentInvalid
	}
	target := "rejected"
	if approve {
		target = "released"
	}
	var released *orderdomain.Order
	var payment *paymentdomain.Payment
	err := s.paymentRepo.WithinTransaction(func(tx paymentcontract.Transaction) error {
		order, err := tx.Orders().GetByIDForUpdateWithChildren(orderID)
		if err != nil {
			return err
		}
		if order == nil {
			return orderapp.ErrOrderNotFound
		}
		if order.ParentID != nil {
			return orderapp.ErrOrderStatusInvalid
		}
		if order.RiskHoldStatus == target {
			return nil
		}
		if order.RiskHoldStatus != "pending_review" || order.PaidAt == nil {
			return orderapp.ErrOrderStatusInvalid
		}
		if approve {
			if (order.Status != constants.OrderStatusPaid && order.Status != constants.OrderStatusFulfilling) ||
				order.RefundedAmount.Decimal.IsPositive() {
				return orderapp.ErrOrderStatusInvalid
			}
			payments, err := tx.Payments().ListByOrderID(order.ID)
			if err != nil {
				return err
			}
			for i := range payments {
				candidate := &payments[i]
				if candidate.ID == order.PaymentRiskPaymentID && candidate.Status == constants.PaymentStatusSuccess {
					payment = candidate
					break
				}
			}
			if payment == nil {
				return ErrPaymentNotFound
			}
		}
		entries, err := orderRiskReviewEntries(order, approve, kinds)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			if err := tx.AppendOrderRiskReviewEntries(entries); err != nil {
				return err
			}
		}
		now := time.Now()
		decision := mergeProviderPayload(order.PaymentRiskDecision, jsonmap.JSON{
			"review_admin_id": adminID, "reviewed_at": now.UTC().Format(time.RFC3339Nano), "review_status": target,
			"review_list_kinds": kinds,
		})
		updates := map[string]interface{}{
			"risk_hold_status": target, "payment_risk_decision": decision, "updated_at": now, "risk_alert_retry_at": nil,
			"risk_release_recovery_at": nil,
		}
		if approve {
			updates["risk_release_recovery_at"] = now.Add(2 * time.Minute)
		}
		if err := tx.Orders().UpdateFields(order.ID, updates); err != nil {
			return err
		}
		if !approve {
			orderIDs := []uint{order.ID}
			for _, child := range order.Children {
				orderIDs = append(orderIDs, child.ID)
			}
			if err := tx.CardSecrets().VoidByOrders(orderIDs, now); err != nil {
				return err
			}
		}
		order.RiskHoldStatus = target
		if approve {
			if s.resellerAccounting != nil {
				if err := s.resellerAccounting.PostOrderProfit(tx.ResellerAccounting(), order, payment); err != nil {
					return err
				}
			}
			released = order
		}
		return nil
	})
	if err != nil {
		return err
	}
	if released != nil {
		s.enqueueOrderPaidAsync(released, payment, paymentLogger("order_id", orderID))
	}
	return nil
}

func orderRiskReviewEntries(order *orderdomain.Order, allow bool, kinds []string) ([]settingssecurity.SettlementListEntry, error) {
	values := riskcontract.ReviewListValues(riskcontract.CheckInput{
		ClientIP: order.ClientIP, RiskDeviceHash: order.RiskDeviceHash, UserID: order.UserID,
	})
	entries := make([]settingssecurity.SettlementListEntry, 0, len(kinds))
	seen := make(map[string]bool, len(kinds))
	for _, kind := range kinds {
		value := values[kind]
		if value == "" || seen[kind] {
			return nil, ErrPaymentInvalid
		}
		seen[kind] = true
		entries = append(entries, settingssecurity.SettlementListEntry{Kind: kind, Value: value, Allow: allow})
	}
	return entries, nil
}

// RecoverReleasedFulfillment retries fulfillment, affiliate commission,
// and pending downstream callbacks without reposting reseller profit.
func (s *PaymentService) RecoverReleasedFulfillment(now time.Time) error {
	if s.queue == nil || !s.queue.Enabled() {
		return nil
	}
	orders, _, err := s.orderRepo.ListAdmin(ordercontract.ListFilter{
		RiskReleaseRecoveryDue: &now, Page: 1, PageSize: 100,
	})
	if err != nil {
		return err
	}
	var failures []error
	for i := range orders {
		order := &orders[i]
		var recoveryErr error
		if !order.RiskBlocksFulfillment() && !order.RefundedAmount.Decimal.IsPositive() &&
			(order.Status == constants.OrderStatusPaid || order.Status == constants.OrderStatusFulfilling || order.Status == constants.OrderStatusPartiallyDelivered) {
			targets := order.Children
			if len(targets) == 0 {
				targets = []orderdomain.Order{*order}
			}
			for j := range targets {
				child := &targets[j]
				if child.Status == constants.OrderStatusPaid && !child.RiskBlocksFulfillment() && shouldAutoFulfill(child) {
					recoveryErr = errors.Join(recoveryErr, s.queue.EnqueueOrderAutoFulfill(child.ID))
				}
			}
			if s.procurementSvc != nil {
				if err := s.procurementSvc.CreateForOrder(order.ID); err != nil && !errors.Is(err, procurementcontract.ErrExists) {
					recoveryErr = errors.Join(recoveryErr, err)
				}
			}
		}
		if !order.RiskBlocksFulfillment() && order.Status != constants.OrderStatusCanceled {
			if s.affiliateSvc != nil && order.Status != constants.OrderStatusRefunded {
				recoveryErr = errors.Join(recoveryErr, s.affiliateSvc.HandleOrderPaid(order.ID))
			}
			if callbacks, ok := s.downstreamCallbackSvc.(interface{ RecoverPendingCallback(uint) error }); ok {
				recoveryErr = errors.Join(recoveryErr, callbacks.RecoverPendingCallback(order.ID))
			}
		}
		var next *time.Time
		if recoveryErr != nil {
			retryAt := now.Add(5 * time.Minute)
			next = &retryAt
			failures = append(failures, fmt.Errorf("recover released order %d: %w", order.ID, recoveryErr))
		}
		if err := s.orderRepo.UpdateFields(order.ID, map[string]interface{}{"risk_release_recovery_at": next}); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (s *PaymentService) dispatchRiskReviewAlert(order *orderdomain.Order) error {
	locale := s.notificationTemplateLocale()
	label := i18n.T(locale, "notification.order_risk_review_title")
	data := jsonmap.JSON{
		"alert_type": label, "alert_type_key": "order_risk_review", "alert_level": "warning",
		"alert_value": fmt.Sprint(order.PaymentRiskDecision["score"]), "alert_threshold": "-",
		"message": fmt.Sprintf(i18n.T(locale, "notification.order_risk_review_message"), order.OrderNo),
	}
	if s.SendRiskTelegram != nil {
		detail, err := s.orderRepo.GetByID(order.ID)
		if err != nil {
			return err
		}
		if detail == nil {
			return orderapp.ErrOrderNotFound
		}
		fields := s.buildOrderNotificationPayload(detail, nil)
		message := fmt.Sprintf("已付款订单待核验\n订单号：%s\n金额：%s %s\n下单邮箱：%s\n命中规则：%v\n商品：%s", detail.OrderNo, fields["amount"], fields["currency"], fields["customer_email"], detail.PaymentRiskDecision["hard_rules"], fields["items_summary"])
		if err := s.SendRiskTelegram(message); err != nil {
			return err
		}
	} else if err := s.notificationSvc.Enqueue(notificationcontract.EnqueueInput{
		EventType: constants.NotificationEventExceptionAlert, BizType: constants.NotificationBizTypeOrder,
		BizID: order.ID, Locale: locale, Data: data,
	}); err != nil {
		return err
	}
	return s.orderRepo.UpdateFields(order.ID, map[string]interface{}{"risk_alert_retry_at": nil})
}

// RecoverRiskAlerts re-enqueues pending alerts after transient queue failures.
func (s *PaymentService) RecoverRiskAlerts(now time.Time) error {
	var failures []error
	if store, ok := s.paymentRepo.(interface {
		ClaimQRAlerts(time.Time) ([]paymentdomain.QRSaveEvent, error)
		AckQRAlert(uint, string) error
	}); ok && s.SendRiskTelegram != nil {
		events, err := store.ClaimQRAlerts(now)
		if err != nil {
			failures = append(failures, err)
		}
		for _, e := range events {
			order, err := s.orderRepo.GetByID(e.OrderID)
			if err == nil && order == nil {
				err = orderapp.ErrOrderNotFound
			}
			if err == nil {
				fields := s.buildOrderNotificationPayload(order, nil)
				err = s.SendRiskTelegram(fmt.Sprintf("付款码风控提醒（补发）\n订单号：%s\n金额：%s %s\n下单邮箱：%s\n疑似保存付款码：3 次\n商品：%s", order.OrderNo, fields["amount"], fields["currency"], e.OrderEmail, fields["items_summary"]))
			}
			if err == nil {
				err = store.AckQRAlert(e.OrderID, e.EventID)
			}
			if err != nil {
				failures = append(failures, err)
			}
		}
	}
	if s.notificationSvc == nil && s.SendRiskTelegram == nil {
		return errors.Join(failures...)
	}
	orders, _, err := s.orderRepo.ListAdmin(ordercontract.ListFilter{
		RiskAlertDue: &now, SkipRelations: true, Page: 1, PageSize: 100,
	})
	if err != nil {
		return err
	}

	for i := range orders {
		order := &orders[i]
		if err := s.dispatchRiskReviewAlert(order); err != nil {
			failures = append(failures, fmt.Errorf("enqueue risk review alert for order %d: %w", order.ID, err))
			if err := s.orderRepo.UpdateFields(order.ID, map[string]interface{}{"risk_alert_retry_at": now.Add(5 * time.Minute)}); err != nil {
				failures = append(failures, err)
			}
		}
	}
	return errors.Join(failures...)
}
