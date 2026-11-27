package application

import (
	"fmt"
	"net/netip"
	"strings"
	"time"

	productcontract "github.com/dujiao-next/internal/modules/catalog/product/contract"
	orderapp "github.com/dujiao-next/internal/modules/order/application"
	ordercontract "github.com/dujiao-next/internal/modules/order/contract"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	riskcontract "github.com/dujiao-next/internal/modules/orderrisk/contract"
	paymentcontract "github.com/dujiao-next/internal/modules/payment/contract"
	paymentdomain "github.com/dujiao-next/internal/modules/payment/domain"
	settingssecurity "github.com/dujiao-next/internal/modules/settings/schema/security"
	walletcontract "github.com/dujiao-next/internal/modules/wallet/contract"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/money"

	"github.com/shopspring/decimal"
)

// PaymentCallbackInput 支付回调输入
type PaymentCallbackInput struct {
	PaymentID   uint
	OrderNo     string
	ChannelID   uint
	Status      string
	ProviderRef string
	Amount      money.Amount
	Currency    string
	PaidAt      *time.Time
	Payload     jsonmap.JSON

	// verifiedLegacyDujiaoPayCurrency 只能由已验签的 DujiaoPay webhook 入口设置。
	// 它允许升级前创建且没有法币快照标记的在途支付采纳网关签名币种；
	// 普通调用方无法开启该兼容分支。
	verifiedLegacyDujiaoPayCurrency string

	// manualConfirm 只能由后台"人工确认支付"入口设置，表示管理员已在支付渠道侧核实到账。
	// 它允许把仍未支付（待支付或已被超时取消）的订单恢复为已支付并继续履约，
	// 同时保留金额/币种校验，且欠付时直接拒绝而不是悄悄转入余额。
	manualConfirm bool
}

func (s *PaymentService) HandleCallback(input PaymentCallbackInput) (*paymentdomain.Payment, error) {
	if input.PaymentID == 0 {
		return nil, ErrPaymentInvalid
	}
	status := normalizePaymentStatus(input.Status)
	if !isPaymentStatusValid(status) {
		return nil, ErrPaymentStatusInvalid
	}

	log := paymentLogger(
		"payment_id", input.PaymentID,
		"target_status", status,
		"callback_channel_id", input.ChannelID,
		"callback_order_no", strings.TrimSpace(input.OrderNo),
		"callback_provider_ref", strings.TrimSpace(input.ProviderRef),
		"callback_currency", strings.ToUpper(strings.TrimSpace(input.Currency)),
		"callback_amount", input.Amount.String(),
	)
	log.Infow("payment_callback_received")

	payment, err := s.paymentRepo.GetByID(input.PaymentID)
	if err != nil {
		log.Errorw("payment_callback_payment_fetch_failed", "error", err)
		return nil, ErrPaymentUpdateFailed
	}
	if payment == nil {
		log.Warnw("payment_callback_payment_not_found")
		return nil, ErrPaymentNotFound
	}
	if payment.OrderID == 0 {
		log.Infow("payment_callback_wallet_recharge_flow")
		return s.handleWalletRechargeCallback(payment, status, input)
	}

	order, err := s.orderRepo.GetByID(payment.OrderID)
	if err != nil {
		log.Errorw("payment_callback_order_fetch_failed", "order_id", payment.OrderID, "error", err)
		return nil, orderapp.ErrOrderFetchFailed
	}
	if order == nil {
		log.Warnw("payment_callback_order_not_found", "order_id", payment.OrderID)
		return nil, orderapp.ErrOrderNotFound
	}

	if input.ChannelID != 0 && input.ChannelID != payment.ChannelID {
		log.Warnw("payment_callback_channel_mismatch",
			"stored_channel_id", payment.ChannelID,
			"callback_channel_id", input.ChannelID,
		)
		return nil, ErrPaymentInvalid
	}
	if !matchesBusinessOrderNo(input.OrderNo, order.OrderNo, payment) {
		log.Warnw("payment_callback_order_no_mismatch",
			"stored_order_no", order.OrderNo,
			"stored_gateway_order_no", payment.GatewayOrderNo,
			"callback_order_no", input.OrderNo,
		)
		return nil, ErrPaymentInvalid
	}
	if err := validateCallbackPaymentFacts(payment, order.OrderNo, status, input); err != nil {
		return nil, err
	}

	// 幂等处理：已成功的不再回退状态。
	// 人工确认需要继续进入事务，由 applyPaymentUpdate 在锁内判断订单是否仍需恢复。
	if payment.Status == constants.PaymentStatusSuccess && !input.manualConfirm {
		log.Infow("payment_callback_idempotent_success",
			"current_status", payment.Status,
		)
		return s.updateCallbackMeta(payment, constants.PaymentStatusSuccess, input)
	}
	if payment.Status == status && !input.manualConfirm {
		log.Infow("payment_callback_idempotent_same_status",
			"current_status", payment.Status,
		)
		return s.updateCallbackMeta(payment, status, input)
	}

	previousStatus := payment.Status
	now := time.Now()
	updated, processedOrder, orderPaid, err := s.applyPaymentUpdate(payment, order, status, input, now)
	if err != nil {
		log.Errorw("payment_callback_apply_failed",
			"order_id", order.ID,
			"order_no", order.OrderNo,
			"current_status", payment.Status,
			"error", err,
		)
		return nil, err
	}
	if orderPaid {
		s.enqueueOrderPaidAsync(processedOrder, updated, log)
	}
	log.Infow("payment_callback_processed",
		"order_id", processedOrder.ID,
		"order_no", processedOrder.OrderNo,
		"previous_status", previousStatus,
		"new_status", updated.Status,
		"order_paid", orderPaid,
		"exception_code", updated.ExceptionCode,
	)
	return updated, nil
}

func validateCallbackPaymentFacts(payment *paymentdomain.Payment, businessOrderNo, status string, input PaymentCallbackInput) error {
	if payment == nil {
		return ErrPaymentNotFound
	}
	if input.ChannelID != 0 && input.ChannelID != payment.ChannelID {
		return ErrPaymentInvalid
	}
	if !matchesBusinessOrderNo(input.OrderNo, businessOrderNo, payment) {
		return ErrPaymentInvalid
	}
	currency := strings.TrimSpace(input.Currency)
	if status == constants.PaymentStatusSuccess {
		if currency == "" {
			return ErrPaymentCurrencyMismatch
		}
		if !input.Amount.Decimal.IsPositive() {
			return ErrPaymentAmountMismatch
		}
	}
	if currency != "" &&
		!strings.EqualFold(currency, strings.TrimSpace(payment.Currency)) &&
		!canAdoptVerifiedLegacyDujiaoPayCurrency(payment, status, input) {
		return ErrPaymentCurrencyMismatch
	}
	if !input.Amount.Decimal.IsZero() && input.Amount.Decimal.Cmp(payment.Amount.Decimal) != 0 {
		return ErrPaymentAmountMismatch
	}
	return nil
}

// canAdoptVerifiedLegacyDujiaoPayCurrency 只接受已经通过 DujiaoPay webhook
// 验签入口标记的升级前支付。新版支付带有创建时法币快照，仍执行严格币种一致性校验。
func canAdoptVerifiedLegacyDujiaoPayCurrency(payment *paymentdomain.Payment, status string, input PaymentCallbackInput) bool {
	if payment == nil ||
		payment.ProviderType != constants.PaymentProviderDujiaoPay ||
		payment.Status == constants.PaymentStatusSuccess ||
		status != constants.PaymentStatusSuccess {
		return false
	}
	verifiedCurrency := strings.ToUpper(strings.TrimSpace(input.verifiedLegacyDujiaoPayCurrency))
	callbackCurrency := strings.ToUpper(strings.TrimSpace(input.Currency))
	if verifiedCurrency == "" || verifiedCurrency != callbackCurrency {
		return false
	}
	if !input.Amount.Decimal.IsPositive() || input.Amount.Decimal.Cmp(payment.Amount.Decimal) != 0 {
		return false
	}
	if payment.ProviderPayload != nil {
		if _, hasSnapshot := payment.ProviderPayload[paymentcontract.GatewayPayloadFiatCurrencySent]; hasSnapshot {
			return false
		}
	}
	return true
}

func adoptVerifiedLegacyDujiaoPayCurrency(payment *paymentdomain.Payment, status string, input PaymentCallbackInput) bool {
	if !canAdoptVerifiedLegacyDujiaoPayCurrency(payment, status, input) {
		return false
	}
	currency := strings.ToUpper(strings.TrimSpace(input.verifiedLegacyDujiaoPayCurrency))
	payment.Currency = currency
	if payment.ProviderPayload == nil {
		payment.ProviderPayload = jsonmap.JSON{}
	}
	payment.ProviderPayload[paymentcontract.GatewayPayloadFiatCurrencySent] = currency
	return true
}

func (s *PaymentService) updateCallbackMeta(payment *paymentdomain.Payment, status string, input PaymentCallbackInput) (*paymentdomain.Payment, error) {
	return updateCallbackMetaWithRepo(s.paymentRepo, payment, status, input)
}

func updateCallbackMetaWithRepo(repo paymentcontract.Store, payment *paymentdomain.Payment, status string, input PaymentCallbackInput) (*paymentdomain.Payment, error) {
	updated := adoptVerifiedLegacyDujiaoPayCurrency(payment, status, input)
	if input.ProviderRef != "" && payment.ProviderRef == "" {
		payment.ProviderRef = input.ProviderRef
		updated = true
	}
	if input.Payload != nil {
		payment.ProviderPayload = mergeProviderPayload(payment.ProviderPayload, input.Payload)
		updated = true
	}
	if status != "" && payment.Status != status {
		payment.Status = status
		updated = true
	}
	if payment.Status == constants.PaymentStatusSuccess && payment.PaidAt == nil && input.PaidAt != nil {
		payment.PaidAt = input.PaidAt
		payment.ProviderPaidAtKnown = true
		updated = true
	}
	if updated {
		now := time.Now()
		payment.CallbackAt = &now
		payment.UpdatedAt = now
		if err := repo.Update(payment); err != nil {
			return nil, ErrPaymentUpdateFailed
		}
	}
	return payment, nil
}

func (s *PaymentService) applyPaymentUpdate(payment *paymentdomain.Payment, order *orderdomain.Order, status string, input PaymentCallbackInput, now time.Time) (*paymentdomain.Payment, *orderdomain.Order, bool, error) {
	returnVal := payment
	processedOrder := order
	orderPaid := false
	riskConfig, err := s.settingService.GetOrderRiskControlConfig()
	if err != nil {
		return nil, nil, false, err
	}

	err = s.paymentRepo.WithinTransaction(func(tx paymentcontract.Transaction) error {
		paymentRepo := tx.Payments()
		// 先订单后支付，与创建支付、钱包支付的加锁顺序一致，避免 PostgreSQL 死锁。
		lockedOrder, err := tx.Orders().GetByIDForUpdateWithChildren(order.ID)
		if err != nil {
			return orderapp.ErrOrderFetchFailed
		}
		if lockedOrder == nil {
			return orderapp.ErrOrderNotFound
		}
		lockedPayment, err := paymentRepo.GetByIDForUpdate(payment.ID)
		if err != nil {
			return ErrPaymentUpdateFailed
		}
		if lockedPayment == nil {
			return ErrPaymentNotFound
		}
		if err := validateCallbackPaymentFacts(lockedPayment, lockedOrder.OrderNo, status, input); err != nil {
			return err
		}
		adoptVerifiedLegacyDujiaoPayCurrency(lockedPayment, status, input)
		returnVal = lockedPayment
		processedOrder = lockedOrder

		// 人工确认：订单必须仍未支付（待支付或已取消）。支付记录可能已是 success
		// （迟到回调只记账不履约），此时仍要继续恢复订单，不能走下面的幂等短路。
		manualRecover := false
		reopened := false
		if input.manualConfirm {
			if status != constants.PaymentStatusSuccess || lockedOrder.PaidAt != nil ||
				(lockedOrder.Status != constants.OrderStatusPendingPayment && lockedOrder.Status != constants.OrderStatusCanceled) {
				return orderapp.ErrOrderStatusInvalid
			}
			// 锁内再校验一次：欠付记录的款项已转入余额，不能再履约。
			if lockedPayment.ExceptionCode == constants.PaymentExceptionUnderpaidSucceeded {
				return ErrPaymentUnderpaidCredited
			}
			manualRecover = true
			if lockedOrder.Status == constants.OrderStatusCanceled {
				if err := orderapp.ReopenCanceledOrderInTx(tx, lockedOrder, now); err != nil {
					return err
				}
				reopened = true
			}
		}

		if lockedPayment.Status == constants.PaymentStatusSuccess && !manualRecover {
			_, err := updateCallbackMetaWithRepo(paymentRepo, lockedPayment, constants.PaymentStatusSuccess, input)
			return err
		}
		if lockedPayment.Status == status && !manualRecover {
			_, err := updateCallbackMetaWithRepo(paymentRepo, lockedPayment, status, input)
			return err
		}

		orderOpen := lockedOrder.Status == constants.OrderStatusPendingPayment && lockedOrder.PaidAt == nil
		// The expiry worker may still be queued. A successful receipt only
		// qualifies for automatic delivery within the original order window.
		if orderOpen && !manualRecover && lockedOrder.ExpiresAt != nil {
			receivedAt := now
			if input.PaidAt != nil && !input.PaidAt.IsZero() && !input.PaidAt.After(now) &&
				!input.PaidAt.Before(lockedPayment.CreatedAt.Truncate(time.Second)) {
				receivedAt = *input.PaidAt
			}
			orderOpen = receivedAt.Before(*lockedOrder.ExpiresAt)
		}

		// 金额守恒：一笔支付只有覆盖订单当前的在线应付额才允许履约。
		// 混合支付切换渠道会退回余额并抬高在线应付额，而旧链接在网关侧依然可付，
		// 缺少这道校验就能用旧链接的小额付款换到整单商品。
		requiredOnlineAmount := normalizeOrderAmount(lockedOrder.TotalAmount.Decimal.Sub(lockedOrder.WalletPaidAmount.Decimal))
		coveredOnlineAmount := paymentCoveredOrderAmount(lockedPayment, input.Amount.Decimal)
		underpaid := status == constants.PaymentStatusSuccess && orderOpen &&
			coveredOnlineAmount.LessThan(requiredOnlineAmount)

		if manualRecover && underpaid {
			// 已取消的混合支付订单：取消时余额部分已退回，重开后应付额变成全额，必然"欠付"。
			// 这不是渠道到账有问题，给出专门的错误，提示走退款流程。
			if reopened {
				return ErrPaymentCanceledOrderUnderpaid
			}
			// 人工确认不能把欠付的钱转进余额后让订单继续挂起，直接回滚让管理员核对金额。
			return ErrPaymentAmountMismatch
		}

		canFulfillOrder := status == constants.PaymentStatusSuccess && orderOpen && !underpaid
		if manualRecover && !canFulfillOrder {
			return orderapp.ErrOrderStatusInvalid
		}
		switch status {
		case constants.PaymentStatusSuccess:
			paidAt := now
			if input.PaidAt != nil {
				paidAt = *input.PaidAt
			}
			// 迟到回调已记录网关侧真实支付时间，人工恢复订单时不要覆盖它。
			if !(manualRecover && lockedPayment.PaidAt != nil) {
				lockedPayment.PaidAt = &paidAt
				lockedPayment.ProviderPaidAtKnown = input.PaidAt != nil
			}
			switch {
			case underpaid:
				lockedPayment.ExceptionCode = constants.PaymentExceptionUnderpaidSucceeded
			case lockedPayment.SupersededAt != nil:
				lockedPayment.ExceptionCode = constants.PaymentExceptionSupersededSucceeded
			case !canFulfillOrder:
				if lockedOrder.PaidAt != nil {
					lockedPayment.ExceptionCode = constants.PaymentExceptionDuplicateSucceeded
				} else {
					lockedPayment.ExceptionCode = constants.PaymentExceptionClosedOrderSucceeded
				}
			}
		case constants.PaymentStatusExpired:
			lockedPayment.ExpiredAt = &now
		}

		if manualRecover && canFulfillOrder {
			// 订单已被人工恢复履约，之前"迟到/关闭订单成功"的待处理异常标记随之解除。
			lockedPayment.ExceptionCode = ""
		}

		lockedPayment.Status = status
		lockedPayment.CallbackAt = &now
		lockedPayment.UpdatedAt = now
		if input.ProviderRef != "" {
			lockedPayment.ProviderRef = input.ProviderRef
		}
		if input.Payload != nil {
			lockedPayment.ProviderPayload = mergeProviderPayload(lockedPayment.ProviderPayload, input.Payload)
		}
		if err := paymentRepo.Update(lockedPayment); err != nil {
			return ErrPaymentUpdateFailed
		}

		if status == constants.PaymentStatusSuccess {
			if _, err := paymentRepo.ExpirePendingByOrderIDs([]uint{lockedOrder.ID}, now); err != nil {
				return ErrPaymentUpdateFailed
			}
		}
		if canFulfillOrder {
			if err := s.markOrderPaid(tx, lockedOrder, lockedPayment, now, riskConfig); err != nil {
				return err
			}
			if s.resellerAccounting != nil {
				if err := s.resellerAccounting.PostOrderProfit(tx.ResellerAccounting(), lockedOrder, lockedPayment); err != nil {
					return err
				}
			}
			orderPaid = true
		}
		// CNY orders and fiat receipts must not turn underpayment into wallet
		// credit, even when the order and the gateway use different currencies.
		manualReconcile := strings.EqualFold(strings.TrimSpace(lockedOrder.Currency), "CNY") ||
			strings.EqualFold(strings.TrimSpace(lockedPayment.Currency), "CNY") ||
			strings.EqualFold(strings.TrimSpace(lockedPayment.ChannelType), constants.PaymentChannelTypeAlipay)
		if underpaid && !manualReconcile {
			if err := s.creditUnderpaidToWallet(tx, lockedOrder, lockedPayment, coveredOnlineAmount, requiredOnlineAmount); err != nil {
				return err
			}
		}
		// 已被替代或已有更新尝试时，钱包占款归更新的尝试所有，旧链接的失败/过期回调不能退还。
		if (status == constants.PaymentStatusFailed || status == constants.PaymentStatusExpired) && lockedOrder.Status == constants.OrderStatusPendingPayment && s.walletSvc != nil && lockedPayment.SupersededAt == nil {
			payments, err := paymentRepo.ListByOrderID(lockedOrder.ID)
			if err != nil {
				return ErrPaymentUpdateFailed
			}
			if newerActivePaymentID(payments, lockedPayment.ID) == 0 {
				if _, err := orderapp.ReleaseWalletBalance(s.walletSvc, tx, lockedOrder, constants.WalletTxnTypeOrderRefund, "在线支付失败，退回余额"); err != nil {
					return err
				}
			}
		}

		return nil
	})
	if err != nil {
		return nil, nil, false, err
	}
	return returnVal, processedOrder, orderPaid, nil
}

// creditUnderpaidToWallet 把"支付成功但金额不足以履约订单"的款项转入用户余额。
//
// 订单保持待支付，用户可以用这笔余额补齐后重新发起支付；商家既不发货也不吞款。
// 以支付 ID 为幂等键，重复回调不会重复入账。游客订单没有钱包账户，只留异常码等待人工处理。
func (s *PaymentService) creditUnderpaidToWallet(
	tx paymentcontract.Transaction,
	order *orderdomain.Order,
	payment *paymentdomain.Payment,
	coveredAmount decimal.Decimal,
	requiredAmount decimal.Decimal,
) error {
	if order == nil || payment == nil {
		return nil
	}
	log := paymentLogger(
		"payment_id", payment.ID,
		"order_id", order.ID,
		"order_no", order.OrderNo,
		"covered_amount", coveredAmount.String(),
		"required_amount", requiredAmount.String(),
	)
	if s.walletSvc == nil || order.UserID == 0 || !coveredAmount.IsPositive() {
		log.Warnw("payment_callback_underpaid_credit_skipped", "user_id", order.UserID)
		return nil
	}
	orderID := order.ID
	if _, _, err := s.walletSvc.CreditInTransaction(tx.Wallets(), walletcontract.CreditInput{
		UserID:    order.UserID,
		Amount:    money.FromDecimal(coveredAmount),
		Currency:  order.Currency,
		Type:      constants.WalletTxnTypeOrderUnderpaidCredit,
		Reference: fmt.Sprintf("payment:%d:underpaid_credit", payment.ID),
		Remark:    "支付金额不足以完成订单，款项已转入余额",
		OrderID:   &orderID,
	}); err != nil {
		log.Errorw("payment_callback_underpaid_credit_failed", "user_id", order.UserID, "error", err)
		return err
	}
	log.Warnw("payment_callback_underpaid_credited_to_wallet", "user_id", order.UserID)
	return nil
}

// mergeProviderPayload 合并第三方回调原文，同时保留创建支付阶段写入的展示快照等元数据。
// 普通回调字段更新同名值；服务端创建时记录的换汇快照只能由本地代码写入。
func mergeProviderPayload(existing jsonmap.JSON, incoming jsonmap.JSON) jsonmap.JSON {
	if incoming == nil {
		return existing
	}
	merged := make(jsonmap.JSON, len(existing)+len(incoming))
	for key, value := range existing {
		merged[key] = value
	}
	for key, value := range incoming {
		switch key {
		case "exchange_rate", "original_amount", "original_currency", paymentcontract.GatewayPayloadFiatCurrencySent:
			continue
		}
		merged[key] = value
	}
	return merged
}

// markOrderPaid 在事务内将订单更新为已支付并处理库存
func (s *PaymentService) markOrderPaid(tx paymentcontract.Transaction, order *orderdomain.Order, payment *paymentdomain.Payment, now time.Time, riskConfig settingssecurity.OrderRiskControlConfig) error {
	if order == nil {
		return orderapp.ErrOrderNotFound
	}
	if !orderapp.IsTransitionAllowed(order.Status, constants.OrderStatusPaid) {
		return orderapp.ErrOrderStatusInvalid
	}
	cnyAmount, canScore := order.TotalAmount.Decimal, strings.EqualFold(order.Currency, "CNY")
	if !canScore && payment != nil && strings.EqualFold(payment.Currency, "CNY") {
		if rate, ok := paymentExchangeRate(payment); ok {
			cnyAmount, canScore = order.TotalAmount.Decimal.Mul(rate), true
		}
	}
	// Match the stored currency precision and bound the integer conversion.
	fen := cnyAmount.Shift(2).Round(0)
	if fen.GreaterThan(decimal.NewFromInt(1_000_000_000_000_000)) {
		fen = decimal.NewFromInt(1_000_000_000_000_000)
	}
	riskMode := riskConfig.SettlementMode
	identityInput := riskcontract.CheckInput{
		UserID: order.UserID, ClientIP: order.ClientIP, RiskDeviceHash: order.RiskDeviceHash, GuestEmail: order.GuestEmail,
	}
	orderRepo := tx.Orders()
	if err := orderRepo.LockRiskKeys(riskcontract.SettlementLockKeys(identityInput)); err != nil {
		return err
	}
	denied, trusted := riskcontract.MatchSettlementList(identityInput, riskConfig, now)
	facts := riskcontract.SettlementFacts{Blacklisted: denied}
	var riskDecision jsonmap.JSON
	var riskCandidate bool
	// Both full-wallet checkout and online settlement use this path.
	if s.walletSvc != nil && order.UserID != 0 && order.WalletPaidAmount.Decimal.IsPositive() {
		since := now.Add(-24 * time.Hour)
		walletRepo := tx.Wallets().Wallets()
		_, recharges, err := walletRepo.ListRechargeOrdersAdmin(walletcontract.RechargeListFilter{
			UserID: order.UserID, ChannelType: constants.PaymentChannelTypeAlipay,
			CreditedFrom: &since, Page: 1, PageSize: 1,
		})
		if err != nil {
			return err
		}
		_, underpaidCredits, err := walletRepo.ListTransactions(walletcontract.TransactionListFilter{
			UserID: order.UserID, Type: constants.WalletTxnTypeOrderUnderpaidCredit,
			CreatedFrom: &since, Page: 1, PageSize: 1,
		})
		if err != nil {
			return err
		}
		facts.RecentMonitoredWalletFunding = recharges > 0 || underpaidCredits > 0
	}
	if canScore {
		facts.Amount = fen.IntPart()
	}
	delayKnown := false
	if payment != nil && payment.ProviderPaidAtKnown && payment.PaidAt != nil && payment.InteractionMode == constants.PaymentInteractionQR {
		ready := payment.CreatedAt
		if payment.CheckoutReadyAt != nil {
			ready = *payment.CheckoutReadyAt
		}
		if !ready.IsZero() && !payment.PaidAt.Before(ready) && !payment.PaidAt.After(now) {
			delayKnown = true
			facts.PaymentDelay = payment.PaidAt.Sub(ready)
		}
	}
	since := now.Add(-24 * time.Hour)
	identity := ordercontract.ListFilter{UserID: order.UserID, ExcludeID: order.ID, Page: 1, PageSize: 100, SkipRelations: true}
	if order.UserID == 0 {
		identity.GuestEmail = strings.TrimSpace(order.GuestEmail)
		identity.RiskContactHash = riskcontract.GuestContactHash(identity.GuestEmail)
	}
	var recent []orderdomain.Order
	if identity.UserID != 0 || identity.GuestEmail != "" {
		oldest := identity
		oldest.PageSize, oldest.SortBy, oldest.SortOrder = 1, "created_at", "asc"
		history, total, err := orderRepo.ListAdmin(oldest)
		if err != nil {
			return err
		}
		facts.NewContact = total == 0
		if len(history) > 0 {
			facts.CustomerAge = now.Sub(history[0].CreatedAt)
		}
		oldest.RiskSuccessfulOnly = true
		_, paid, err := orderRepo.ListAdmin(oldest)
		if err != nil {
			return err
		}
		facts.HistoricalSuccessfulOrders = int(paid)
		identity.RiskActivityFrom = &since
		recent, _, err = orderRepo.ListAdmin(identity)
		if err != nil {
			return err
		}
	}
	seenRecent := make(map[uint]bool, len(recent))
	for _, previous := range recent {
		seenRecent[previous.ID] = true
	}
	shortWindow := now.Add(-30 * time.Minute)
	if order.RiskIP != "" {
		ipHistory := ordercontract.ListFilter{RiskIP: order.RiskIP, ExcludeID: order.ID, Page: 1, PageSize: 1, SkipRelations: true}
		_, total, err := orderRepo.ListAdmin(ipHistory)
		if err != nil {
			return err
		}
		facts.NewIP = total == 0
		if total > 0 && identity.GuestEmail != "" {
			ipHistory.PageSize, ipHistory.CreatedFrom = 50, &since
			neighbors, _, err := orderRepo.ListAdmin(ipHistory)
			if err != nil {
				return err
			}
			for _, previous := range neighbors {
				if previous.GuestEmail == "" || previous.GuestEmail == identity.GuestEmail {
					continue
				}
				same, similar := riskcontract.CompareGuestContacts(identity.GuestEmail, previous.GuestEmail)
				facts.ApproximateContact = facts.ApproximateContact || similar
				if same {
					facts.NewContact = false
					if !seenRecent[previous.ID] {
						recent = append(recent, previous)
						seenRecent[previous.ID] = true
					}
				} else if !previous.CreatedAt.Before(shortWindow) {
					facts.DifferentContactSameIP = true
				}
			}
		}
	}
	if facts.Amount >= 15000 && order.RiskIP != "" {
		subnet := ordercontract.ListFilter{ExcludeID: order.ID, PaidFrom: &shortWindow, Page: 1, PageSize: 50, SkipRelations: true}
		if ip, err := netip.ParseAddr(order.RiskIP); err == nil && ip.Is4() {
			masked := netip.PrefixFrom(ip, 24).Masked().Addr().String()
			subnet.RiskIPv4Prefix = masked[:strings.LastIndexByte(masked, '.')+1]
		} else {
			// IPv6 risk IPs already store a normalized /64.
			subnet.RiskIP = order.RiskIP
		}
		neighbors, _, err := orderRepo.ListAdmin(subnet)
		if err != nil {
			return err
		}
		for _, previous := range neighbors {
			if previous.PaymentRiskAmountFen >= 15000 ||
				strings.EqualFold(previous.Currency, "CNY") && previous.TotalAmount.Decimal.GreaterThanOrEqual(decimal.NewFromInt(150)) {
				facts.SubnetBurst = true
				break
			}
		}
	}
	if order.RiskDeviceHash != "" {
		rows, _, err := orderRepo.ListAdmin(ordercontract.ListFilter{
			RiskDeviceHash: order.RiskDeviceHash, ExcludeID: order.ID, RiskActivityFrom: &since,
			Page: 1, PageSize: 50, SkipRelations: true,
		})
		if err != nil {
			return err
		}
		for _, previous := range rows {
			if !seenRecent[previous.ID] {
				recent = append(recent, previous)
				seenRecent[previous.ID] = true
			}
			if order.UserID != previous.UserID {
				facts.DeviceSwitch = true
			}
			if identity.GuestEmail != "" && previous.GuestEmail != "" && previous.GuestEmail != identity.GuestEmail {
				same, similar := riskcontract.CompareGuestContacts(identity.GuestEmail, previous.GuestEmail)
				facts.DeviceSwitch = facts.DeviceSwitch || !same
				facts.ApproximateContact = facts.ApproximateContact || similar
			}
		}
	}
	for _, previous := range recent {
		if previous.PaidAt != nil && !previous.PaidAt.Before(since) {
			amount := previous.PaymentRiskAmountFen
			if amount == 0 && strings.EqualFold(previous.Currency, "CNY") {
				amount = decimal.Min(previous.TotalAmount.Decimal.Shift(2).Round(0), decimal.NewFromInt(1_000_000_000_000_000)).IntPart()
			}
			if amount <= 0 && !previous.TotalAmount.Decimal.IsPositive() {
				continue
			}
			if amount < 0 {
				amount = 0
			}
			facts.PaidCount24h++
			facts.PaidAmount24h += amount
			if !previous.PaidAt.Before(shortWindow) {
				facts.PaidCount30m++
				facts.PaidAmount30m += amount
			}
		} else if previous.PaidAt == nil && !previous.CreatedAt.Before(since) && (previous.Status == constants.OrderStatusPendingPayment || previous.Status == constants.OrderStatusCanceled) {
			facts.Unpaid24h++
			if !previous.CreatedAt.Before(shortWindow) {
				facts.Unpaid30m++
			}
		}
	}
	if reader, ok := orderRepo.(interface {
		HasUnusedRedemption([]uint) (bool, error)
	}); ok {
		eligible := make([]uint, 0, len(recent))
		for _, previous := range recent {
			if previous.PaidAt == nil || previous.PaidAt.Before(now.Add(-2*time.Hour)) {
				continue
			}
			if previous.PaymentRiskAmountFen >= 15000 ||
				previous.PaymentRiskAmountFen == 0 && strings.EqualFold(previous.Currency, "CNY") &&
					previous.TotalAmount.Decimal.GreaterThanOrEqual(decimal.NewFromInt(150)) {
				eligible = append(eligible, previous.ID)
			}
		}
		unused, err := reader.HasUnusedRedemption(eligible)
		if err != nil {
			return err
		}
		facts.UnredeemedHighOrder = unused
	}
	facts.RiskyEmail = riskcontract.SettlementRiskyEmail(order.GuestEmail, riskConfig)
	decision := riskcontract.EvaluateSettlement(facts, riskConfig)
	if trusted && !facts.Blacklisted && !facts.RecentMonitoredWalletFunding {
		decision.Hold, decision.Exempt = false, true
	}
	riskCandidate = decision.Hold
	riskDecision = jsonmap.JSON{
		"mode": riskMode, "score": decision.Score, "exempt": decision.Exempt,
		"hard_rules": decision.HardRules, "signals": decision.Signals,
		"recent_wallet_funding": facts.RecentMonitoredWalletFunding, "cny_amount_known": canScore,
		"pay_delay_known": delayKnown,
		"threshold":       riskConfig.SettlementThreshold,
		"features": jsonmap.JSON{
			"paid_count_30m": facts.PaidCount30m, "paid_count_24h": facts.PaidCount24h,
			"paid_amount_fen_30m": facts.PaidAmount30m, "paid_amount_fen_24h": facts.PaidAmount24h,
			"unpaid_count_30m": facts.Unpaid30m, "unpaid_count_24h": facts.Unpaid24h,
			"new_contact": facts.NewContact, "new_ip": facts.NewIP, "device_switch": facts.DeviceSwitch,
			"approximate_contact": facts.ApproximateContact, "unredeemed_high_order": facts.UnredeemedHighOrder,
			"subnet_burst": facts.SubnetBurst, "different_contact_same_ip": facts.DifferentContactSameIP,
			"risky_email": facts.RiskyEmail, "blacklisted": facts.Blacklisted,
			"historical_successful_orders": facts.HistoricalSuccessfulOrders,
			"customer_age_hours":           int64(facts.CustomerAge / time.Hour),
			"matched_allowlist":            trusted,
		},
	}
	if delayKnown {
		riskDecision["pay_delay_seconds"] = int64(facts.PaymentDelay / time.Second)
	}
	if decision.Hold || decision.Score > 0 || facts.RecentMonitoredWalletFunding {
		paymentLogger("order_id", order.ID).Infow("payment_risk_shadow_settlement",
			"hold_candidate", decision.Hold, "score", decision.Score, "hard_rules", decision.HardRules,
			"signals", decision.Signals, "recent_wallet_funding", facts.RecentMonitoredWalletFunding)
	}
	productRepo := tx.Products()
	var productSKURepo productcontract.SKURepository
	if s.productSKURepo != nil {
		productSKURepo = tx.ProductSKUs()
	}

	onlineAmount := normalizeOrderAmount(order.TotalAmount.Decimal.Sub(order.WalletPaidAmount.Decimal))
	orderUpdates := map[string]interface{}{
		"paid_at":            now,
		"online_paid_amount": money.FromDecimal(onlineAmount),
		"updated_at":         now,
	}
	if riskDecision != nil {
		orderUpdates["payment_risk_evaluated_at"] = now
		orderUpdates["payment_risk_amount_fen"] = facts.Amount
		if payment != nil {
			orderUpdates["payment_risk_payment_id"] = payment.ID
		}
		orderUpdates["payment_risk_candidate"] = riskCandidate
		orderUpdates["payment_risk_decision"] = riskDecision
		if riskCandidate && (riskMode == settingssecurity.SettlementModeHoldNotice || riskMode == settingssecurity.SettlementModeSilentTrap) {
			orderUpdates["risk_hold_status"] = "pending_review"
			if s.notificationSvc != nil {
				orderUpdates["risk_alert_retry_at"] = now.Add(2 * time.Minute)
			}
		}
	}
	if err := orderRepo.UpdateStatus(order.ID, constants.OrderStatusPaid, orderUpdates); err != nil {
		return orderapp.ErrOrderUpdateFailed
	}
	order.Status = constants.OrderStatusPaid
	if riskDecision != nil {
		order.PaymentRiskDecision = riskDecision
		order.PaymentRiskCandidate = riskCandidate
		order.PaymentRiskEvaluatedAt = &now
	}
	if hold, ok := orderUpdates["risk_hold_status"].(string); ok {
		order.RiskHoldStatus = hold
	}
	order.PaidAt = &now
	order.OnlinePaidAmount = money.FromDecimal(onlineAmount)
	order.UpdatedAt = now

	if len(order.Children) > 0 {
		for idx := range order.Children {
			child := &order.Children[idx]
			childStatus := constants.OrderStatusPaid
			if shouldMarkFulfilling(child) {
				childStatus = constants.OrderStatusFulfilling
			}
			if err := orderRepo.UpdateStatus(child.ID, childStatus, map[string]interface{}{
				"paid_at":    now,
				"updated_at": now,
			}); err != nil {
				return orderapp.ErrOrderUpdateFailed
			}
			if err := orderapp.ConsumeManualStockByItems(productRepo, productSKURepo, child.Items); err != nil {
				return err
			}
			child.Status = childStatus
			child.PaidAt = &now
			child.UpdatedAt = now
		}
		parentStatus := orderapp.CalcParentStatus(order.Children, constants.OrderStatusPaid)
		if parentStatus != "" && parentStatus != constants.OrderStatusPaid {
			if err := orderRepo.UpdateStatus(order.ID, parentStatus, map[string]interface{}{
				"online_paid_amount": money.FromDecimal(onlineAmount),
				"updated_at":         now,
			}); err != nil {
				return orderapp.ErrOrderUpdateFailed
			}
			order.Status = parentStatus
		}
		return nil
	}

	if err := orderapp.ConsumeManualStockByItems(productRepo, productSKURepo, order.Items); err != nil {
		return err
	}
	return nil
}
