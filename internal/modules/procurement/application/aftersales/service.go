package aftersales

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	procurementcontract "github.com/dujiao-next/internal/modules/procurement/contract"
	procurementdomain "github.com/dujiao-next/internal/modules/procurement/domain"
)

var ErrConflict = errors.New("aftersale state conflict")
var ErrUnsupported = errors.New("aftersale operation unsupported")

// Repository atomically reserves one operation per procurement order and replaces
// the persisted delivery only after the remote operation has been confirmed.
type Repository interface {
	GetByID(uint) (*procurementdomain.Order, error)
	CheckRedemptionAccess(context.Context, uint) error
	ObserveRedemptionProgress(context.Context, uint, string, string) error
	BeginAfterSale(uint, string, string, string, int) (bool, error)
	CompleteReplacement(uint, string, string, int) error
}
type Remote interface {
	Inspect(context.Context, string) (*procurementcontract.RedemptionState, error)
	Replace(context.Context, string, string, int) (*procurementcontract.RedemptionState, error)
}
type Connections interface{ OpenAfterSale(uint) (Remote, error) }
type Service struct {
	Credit      func(uint) error
	Notify      func(uint)
	Store       Repository
	Connections Connections
}

func (s *Service) Inspect(ctx context.Context, id uint) (*procurementcontract.RedemptionState, error) {
	return s.inspect(ctx, id, false)
}

// InspectForAudit lets administrators verify upstream state for held orders
// without releasing the code or mutating the upstream order.
func (s *Service) InspectForAudit(ctx context.Context, id uint) (*procurementcontract.RedemptionState, error) {
	return s.inspect(ctx, id, true)
}

func (s *Service) inspect(ctx context.Context, id uint, audit bool) (*procurementcontract.RedemptionState, error) {
	o, err := s.Store.GetByID(id)
	if err != nil {
		return nil, err
	}
	if o == nil {
		return nil, ErrConflict
	}
	checkAccess := func() error {
		err := s.Store.CheckRedemptionAccess(ctx, o.LocalOrderID)
		if audit && errors.Is(err, procurementcontract.ErrOrderRiskHeld) {
			return nil
		}
		return err
	}
	if err := checkAccess(); err != nil {
		return nil, err
	}
	remote, err := s.Connections.OpenAfterSale(o.ConnectionID)
	if err != nil {
		return nil, err
	}
	result, err := remote.Inspect(ctx, o.UpstreamOrderNo)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, ErrConflict
	}
	if err := checkAccess(); err != nil {
		return nil, err
	}
	if err := s.Store.ObserveRedemptionProgress(ctx, o.LocalOrderID, result.Code, result.Progress); err != nil {
		return nil, err
	}
	if audit {
		state := *result
		state.Code = ""
		return &state, nil
	}
	return result, nil
}

func (s *Service) Replace(ctx context.Context, id uint, requestID string, revision int) (*procurementcontract.RedemptionState, error) {
	if requestID == "" || len(requestID) > 64 || revision < 1 {
		return nil, ErrConflict
	}
	o, err := s.Store.GetByID(id)
	if err != nil {
		return nil, err
	}
	if o == nil {
		return nil, ErrConflict
	}
	if err := s.Store.CheckRedemptionAccess(ctx, o.LocalOrderID); err != nil {
		return nil, err
	}
	remote, err := s.Connections.OpenAfterSale(o.ConnectionID)
	if err != nil {
		return nil, err
	}
	current, err := remote.Inspect(ctx, o.UpstreamOrderNo)
	if err != nil {
		return nil, err
	}
	// A completed replay returns the current code without replacing it again.
	if o.AfterSaleKind == "replace" && o.AfterSaleRequestID == requestID && o.AfterSaleState == "completed" {
		if o.AfterSaleExpectedRevision != revision {
			return nil, ErrConflict
		}
		if err := s.Store.CheckRedemptionAccess(ctx, o.LocalOrderID); err != nil {
			return nil, err
		}
		return current, nil
	}
	if o.AfterSaleState != "processing" {
		if current.Financial != "paid" || current.Progress != "unused" || !current.CanReplace || current.Revision != revision {
			return nil, ErrConflict
		}
		ok, err := s.Store.BeginAfterSale(id, "replace", requestID, "", revision)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrConflict
		}
	} else if o.AfterSaleKind != "replace" || o.AfterSaleRequestID != requestID || o.AfterSaleExpectedRevision != revision {
		return nil, ErrConflict
	}
	// Fixed remote request ID survives timeouts and process restarts. An unknown
	// outcome may only replay this operation, never start a second replacement.
	if err := s.Store.CheckRedemptionAccess(ctx, o.LocalOrderID); err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", id, requestID)))
	result, err := remote.Replace(ctx, o.UpstreamOrderNo, fmt.Sprintf("xgai-replace-%x", sum[:20]), revision)
	if err != nil {
		return nil, err
	}
	if result.Code == "" || result.Revision <= revision || result.Financial != "paid" {
		return nil, ErrConflict
	}
	if err = s.Store.CompleteReplacement(id, requestID, result.Code, result.Revision); err != nil {
		return nil, err
	}
	if err := s.Store.CheckRedemptionAccess(ctx, o.LocalOrderID); err != nil {
		return nil, err
	}
	if s.Notify != nil {
		s.Notify(o.LocalOrderID)
	}
	return result, nil
}

func (s *Service) RequestRefund(ctx context.Context, id uint, requestID, reason string) error {
	if requestID == "" || len(requestID) > 64 || strings.TrimSpace(reason) == "" || len(reason) > 1000 {
		return ErrConflict
	}
	o, err := s.Store.GetByID(id)
	if err != nil {
		return err
	}
	if o == nil {
		return ErrConflict
	}
	if o.AfterSaleKind == "refund" && o.AfterSaleRequestID == requestID {
		if o.AfterSaleReason != reason {
			return ErrConflict
		}
		return nil
	}
	remote, err := s.Connections.OpenAfterSale(o.ConnectionID)
	if err != nil {
		return err
	}
	current, err := remote.Inspect(ctx, o.UpstreamOrderNo)
	if err != nil {
		return err
	}
	if current.Financial != "paid" || current.Progress != "unused" || !current.CanRefund {
		return ErrConflict
	}
	ok, err := s.Store.BeginAfterSale(id, "refund", requestID, reason, current.Revision)
	if err != nil {
		return err
	}
	if !ok {
		return ErrConflict
	}
	if s.Notify != nil {
		s.Notify(o.LocalOrderID)
	}
	return nil
}

// ReviewRefund is invoked only by an administrator. Remote cancellation is
// confirmed before local credit; retries reuse the operation's stable identity.
func (s *Service) ReviewRefund(ctx context.Context, id uint, approve bool, adminID uint, remark string) error {
	o, err := s.Store.GetByID(id)
	if err != nil {
		return err
	}
	if o == nil || o.AfterSaleKind != "refund" {
		return ErrConflict
	}
	store, ok := s.Store.(interface {
		ReviewAfterSale(uint, string, string, uint, string) (bool, error)
	})
	if !ok {
		return ErrUnsupported
	}
	if o.AfterSaleState == "refunded" {
		return nil
	}
	if !approve {
		ok, err := store.ReviewAfterSale(id, "pending_review", "rejected", adminID, remark)
		if err != nil {
			return err
		}
		if !ok {
			return ErrConflict
		}
		return nil
	}
	if o.AfterSaleState == "pending_review" {
		ok, err := store.ReviewAfterSale(id, "pending_review", "refund_processing", adminID, remark)
		if err != nil {
			return err
		}
		if !ok {
			return ErrConflict
		}
	} else if o.AfterSaleState != "refund_processing" {
		return ErrConflict
	}
	remote, err := s.Connections.OpenAfterSale(o.ConnectionID)
	if err != nil {
		return err
	}
	refunder, ok := remote.(interface {
		Refund(context.Context, string, string, int) (*procurementcontract.RedemptionState, error)
	})
	if !ok {
		return ErrUnsupported
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", id, o.AfterSaleRequestID)))
	result, err := refunder.Refund(ctx, o.UpstreamOrderNo, fmt.Sprintf("xgai-refund-%x", sum[:20]), o.AfterSaleExpectedRevision)
	if err != nil {
		return err
	}
	if result.Financial != "refunded" || result.Progress != "destroyed" {
		return ErrConflict
	}
	if s.Credit == nil {
		return ErrUnsupported
	}
	if err := s.Credit(o.LocalOrderID); err != nil {
		return err
	}
	ok, err = store.ReviewAfterSale(id, "refund_processing", "refunded", adminID, remark)
	if err != nil {
		return err
	}
	if !ok {
		return ErrConflict
	}
	if s.Notify != nil {
		s.Notify(o.LocalOrderID)
	}
	return nil
}
