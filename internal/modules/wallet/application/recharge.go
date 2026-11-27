package application

import (
	"fmt"
	"time"

	"github.com/dujiao-next/internal/constants"
	walletcontract "github.com/dujiao-next/internal/modules/wallet/contract"
	walletdomain "github.com/dujiao-next/internal/modules/wallet/domain"

	"github.com/shopspring/decimal"
)

func (s *Service) ApplyRechargePayment(
	tx walletcontract.Transaction,
	recharge *walletdomain.RechargeOrder,
) (*walletdomain.Transaction, error) {
	if tx == nil {
		return nil, walletcontract.ErrTransactionRequired
	}
	if recharge == nil || recharge.ID == 0 || recharge.UserID == 0 {
		return nil, walletcontract.ErrRechargeNotFound
	}
	if recharge.Amount.Decimal.Round(2).LessThanOrEqual(decimal.Zero) {
		return nil, walletcontract.ErrInvalidAmount
	}
	_, transaction, err := s.CreditInTransaction(tx, walletcontract.CreditInput{
		UserID: recharge.UserID, Amount: recharge.Amount, Currency: recharge.Currency,
		Type:      constants.WalletTxnTypeRecharge,
		Reference: fmt.Sprintf("recharge:%d:success", recharge.ID),
		Remark:    cleanRemark(recharge.Remark, "在线充值到账"),
	})
	return transaction, err
}

// ApproveRecharge releases a paid, held recharge exactly once.
func (s *Service) ApproveRecharge(paymentID uint) error {
	return s.transactions.WithinTransaction(func(tx walletcontract.Transaction) error {
		repo := tx.Wallets()
		recharge, err := repo.GetRechargeOrderByPaymentIDForUpdate(paymentID)
		if err != nil {
			return err
		}
		if recharge == nil {
			return walletcontract.ErrRechargeNotFound
		}
		if recharge.Status == constants.WalletRechargeStatusSuccess {
			return nil
		}
		if recharge.Status != constants.WalletRechargeStatusPendingReview || recharge.PaidAt == nil {
			return walletcontract.ErrRechargeStatusInvalid
		}
		if _, err := s.ApplyRechargePayment(tx, recharge); err != nil {
			return err
		}
		recharge.Status = constants.WalletRechargeStatusSuccess
		recharge.UpdatedAt = time.Now()
		return repo.UpdateRechargeOrder(recharge)
	})
}
