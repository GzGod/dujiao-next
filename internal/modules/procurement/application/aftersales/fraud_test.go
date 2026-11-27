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

type unconfirmedPauseRemote struct{ fraudTestRemote }

func (r *unconfirmedPauseRemote) PauseRedemption(_ context.Context, _, requestID string) error {
	r.pauses++
	return nil
}

func TestFraudPauseAcknowledgementDoesNotConfirmStop(t *testing.T) {
	remote := &unconfirmedPauseRemote{fraudTestRemote: fraudTestRemote{
		allowed: true, state: contract.RedemptionState{ReleaseStatus: "occupied", Financial: "paid"},
	}}
	state, err := FraudOperation(context.Background(), remote, "purchase", "upstream_pause")
	if err == nil || state == nil || state.ReleaseStatus != "occupied" || remote.pauses != 1 || remote.refunds != 0 {
		t.Fatalf("pause state=%+v err=%v calls=%d refunds=%d", state, err, remote.pauses, remote.refunds)
	}
}

type timeoutPauseRemote struct {
	fraudTestRemote
	requestIDs []string
}

func (r *timeoutPauseRemote) PauseRedemption(ctx context.Context, orderNo, requestID string) error {
	r.requestIDs = append(r.requestIDs, requestID)
	if len(r.requestIDs) == 1 {
		return context.DeadlineExceeded
	}
	return r.fraudTestRemote.PauseRedemption(ctx, orderNo, requestID)
}

func TestFraudPauseRetryReusesRequestID(t *testing.T) {
	remote := &timeoutPauseRemote{fraudTestRemote: fraudTestRemote{
		allowed: true, state: contract.RedemptionState{ReleaseStatus: "occupied", Financial: "paid"},
	}}
	ctx := context.Background()
	if _, err := FraudOperation(ctx, remote, "purchase", "upstream_pause"); err != context.DeadlineExceeded {
		t.Fatalf("expected timeout, got %v", err)
	}
	state, err := FraudOperation(ctx, remote, "purchase", "upstream_pause")
	if err != nil || state == nil || state.Pause == nil || state.Pause.Status != "payment_review" {
		t.Fatalf("retry state=%+v err=%v", state, err)
	}
	if len(remote.requestIDs) != 2 || remote.requestIDs[0] == "" || remote.requestIDs[0] != remote.requestIDs[1] || remote.refunds != 0 {
		t.Fatal("retry must reuse the original pause request ID and must not refund")
	}
}

type lostRefundResponseRemote struct{ fraudTestRemote }

func (r *lostRefundResponseRemote) RefundRedemption(ctx context.Context, orderNo, requestID string, revision int) (*contract.RedemptionState, error) {
	_, _ = r.fraudTestRemote.RefundRedemption(ctx, orderNo, requestID, revision)
	return nil, context.DeadlineExceeded
}

func TestFraudRefundLostResponseUsesReadBack(t *testing.T) {
	remote := &lostRefundResponseRemote{fraudTestRemote: fraudTestRemote{
		allowed: true, state: contract.RedemptionState{
			ReleaseStatus: "released", Financial: "paid", CanRefund: true, Revision: 3,
		},
	}}
	state, err := FraudOperation(context.Background(), remote, "purchase", "upstream_refund")
	if err != nil || !supplierRefundConfirmed(state) || remote.refunds != 1 {
		t.Fatalf("refund state=%+v err=%v calls=%d", state, err, remote.refunds)
	}
}

func TestFraudStatusRequiresRefundCapability(t *testing.T) {
	remote := &fraudTestRemote{state: contract.RedemptionState{
		ReleaseStatus: "released", Financial: "paid", CanRefund: true, Revision: 3,
	}}
	for _, enabled := range []bool{false, true} {
		remote.allowed = enabled
		state, err := FraudOperation(context.Background(), remote, "purchase", "upstream_status")
		if err != nil || state == nil || state.CanRefund != enabled || remote.pauses != 0 || remote.refunds != 0 {
			t.Fatalf("capability enabled=%v, state=%+v err=%v", enabled, state, err)
		}
	}
}
