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
		return remote.InspectRedemption(ctx, orderNo)
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
		if err != nil {
			return current, err
		}
		if result == nil || result.Financial != "refunded" || result.Progress != "destroyed" {
			return current, errors.New("upstream destruction and refund unconfirmed")
		}
		return result, nil
	default:
		return current, nil
	}
}
