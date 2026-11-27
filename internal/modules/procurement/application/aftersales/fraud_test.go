package aftersales

import (
	"context"
	"testing"

	"github.com/dujiao-next/internal/modules/procurement/contract"
)

type fraudTestRemote struct {
	state           contract.RedemptionState
	allowed         bool
	pauses, refunds int
}

func (r *fraudTestRemote) InspectRedemption(context.Context, string) (*contract.RedemptionState, error) {
	copy := r.state
	return &copy, nil
}
func (r *fraudTestRemote) HasRedemptionOperation(context.Context, string) (bool, error) {
	return r.allowed, nil
}
func (r *fraudTestRemote) PauseRedemption(_ context.Context, _, requestID string) error {
	if len(requestID) == 0 {
		panic("missing idempotency key")
	}
	r.pauses++
	r.state.Pause = &contract.RedemptionPause{Status: "payment_review", RecoveryStatus: "not_scheduled"}
	r.state.ReleaseStatus = "needs_review"
	return nil
}
func (r *fraudTestRemote) RefundRedemption(_ context.Context, _, requestID string, revision int) (*contract.RedemptionState, error) {
	if len(requestID) == 0 || revision != r.state.Revision {
		panic("invalid refund request")
	}
	r.refunds++
	r.state.Financial = "refunded"
	r.state.Progress = "destroyed"
	return r.InspectRedemption(context.Background(), "")
}

func TestFraudStopAndExplicitSupplierRefund(t *testing.T) {
	ctx := context.Background()
	r := &fraudTestRemote{allowed: true, state: contract.RedemptionState{ReleaseStatus: "occupied", Financial: "paid", Revision: 3}}
	state, err := FraudOperation(ctx, r, "purchase", "upstream_pause")
	if err != nil || state.Pause.Status != "payment_review" || r.pauses != 1 || r.refunds != 0 {
		t.Fatalf("pause state=%+v err=%v", state, err)
	}
	r.state.CanRefund = true
	if _, err := FraudOperation(ctx, r, "purchase", "upstream_refund"); err == nil || r.refunds != 0 {
		t.Fatal("unreleased award should never be refunded")
	}
	r.state.ReleaseStatus = "released"
	r.allowed = false
	if _, err := FraudOperation(ctx, r, "purchase", "upstream_refund"); err == nil || r.refunds != 0 {
		t.Fatal("disabled refund must be rejected")
	}
	r.allowed = true
	for range 2 {
		state, err = FraudOperation(ctx, r, "purchase", "upstream_refund")
		if err != nil || state.Financial != "refunded" || state.Progress != "destroyed" || r.refunds != 1 {
			t.Fatalf("refund state=%+v err=%v calls=%d", state, err, r.refunds)
		}
	}
}
