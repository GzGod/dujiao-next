package aftersales

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	procurementcontract "github.com/dujiao-next/internal/modules/procurement/contract"
)

type FraudRemote interface {
	InspectRedemption(context.Context, string) (*procurementcontract.RedemptionState, error)
	HasRedemptionOperation(context.Context, string) (bool, error)
	PauseRedemption(context.Context, string, string) error
	RefundRedemption(context.Context, string, string, int) (*procurementcontract.RedemptionState, error)
}

func FraudOperation(ctx context.Context, remote FraudRemote, orderNo, action string) (*procurementcontract.RedemptionState, error) {
	current, err := remote.InspectRedemption(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, errors.New("missing upstream order state")
	}
	switch action {
	case "upstream_pause":
		if current.ReleaseStatus != "occupied" && current.ReleaseStatus != "awaiting_original_stop" || current.Pause != nil && current.Pause.Status != "" {
			return current, nil
		}
		allowed, err := remote.HasRedemptionOperation(ctx, "pause_order")
		if err != nil || !allowed {
			return current, errors.New("upstream pause unavailable")
		}
		sum := sha256.Sum256([]byte(orderNo))
		if err := remote.PauseRedemption(ctx, orderNo, fmt.Sprintf("xgai-fraud-pause-%x", sum[:16])); err != nil {
			return current, err
		}
		updated, err := remote.InspectRedemption(ctx, orderNo)
		if err != nil {
			return current, err
		}
		if updated == nil || updated.Pause == nil ||
			(updated.Pause.Status != "paused" && updated.Pause.Status != "payment_review") {
			return current, errors.New("upstream pause outcome unconfirmed")
		}
		return updated, nil
	case "upstream_refund":
		if current.Financial == "refunded" && current.Progress == "destroyed" {
			return current, nil
		}
		allowed, err := remote.HasRedemptionOperation(ctx, "refund_released_order")
		if err != nil || !allowed || current.Financial != "paid" || current.ReleaseStatus != "released" || !current.CanRefund || current.Revision < 1 {
			return current, errors.New("upstream order is not refundable")
		}
		sum := sha256.Sum256([]byte(orderNo))
		result, err := remote.RefundRedemption(ctx, orderNo, fmt.Sprintf("xgai-fraud-refund-%x", sum[:16]), current.Revision)
		if err == nil && supplierRefundConfirmed(result) {
			return result, nil
		}
		// An interrupted response can arrive after GPTGET completed the refund.
		// Read back the original order without resubmitting the mutation.
		observed, inspectErr := remote.InspectRedemption(ctx, orderNo)
		if inspectErr == nil && supplierRefundConfirmed(observed) {
			return observed, nil
		}
		return current, errors.Join(errors.New("upstream destruction and refund unconfirmed"), err, inspectErr)
	case "upstream_status":
		state := *current
		state.CanRefund = false
		if current.Financial == "paid" && current.ReleaseStatus == "released" && current.CanRefund && current.Revision > 0 {
			enabled, err := remote.HasRedemptionOperation(ctx, "refund_released_order")
			if err != nil {
				return &state, err
			}
			state.CanRefund = enabled
		}
		return &state, nil
	default:
		return nil, ErrUnsupported
	}
}

func supplierRefundConfirmed(state *procurementcontract.RedemptionState) bool {
	return state != nil && state.Financial == "refunded" && state.Progress == "destroyed"
}
