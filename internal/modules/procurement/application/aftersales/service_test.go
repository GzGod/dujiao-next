package aftersales

import (
	"context"
	"errors"
	procurementcontract "github.com/dujiao-next/internal/modules/procurement/contract"
	procurementdomain "github.com/dujiao-next/internal/modules/procurement/domain"
	"sync"
	"testing"
)

type testStore struct {
	mu         sync.Mutex
	order      procurementdomain.Order
	delivery   string
	failCommit bool
	accessErr  error
}

func (s *testStore) GetByID(uint) (*procurementdomain.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o := s.order
	return &o, nil
}
func (s *testStore) CheckRedemptionAccess(context.Context, uint) error {
	return s.accessErr
}

func (s *testStore) ObserveRedemptionProgress(context.Context, uint, string, string) error {
	return nil
}

func (s *testStore) BeginAfterSale(id uint, kind, requestID, reason string, revision int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.order.AfterSaleState != "" && s.order.AfterSaleState != "completed" && s.order.AfterSaleState != "rejected" {
		return false, nil
	}
	if s.order.CodeRevision != 0 && s.order.CodeRevision != revision {
		return false, nil
	}
	s.order.AfterSaleKind = kind
	s.order.AfterSaleRequestID = requestID
	s.order.AfterSaleExpectedRevision = revision
	s.order.AfterSaleReason = reason
	s.order.AfterSaleState = "processing"
	if kind == "refund" {
		s.order.AfterSaleState = "pending_review"
	}
	return true, nil
}
func (s *testStore) CompleteReplacement(id uint, requestID, code string, rev int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failCommit {
		s.failCommit = false
		return errors.New("database unavailable")
	}
	s.delivery = code
	s.order.CodeRevision = rev
	s.order.AfterSaleState = "completed"
	return nil
}
func (s *testStore) ReviewAfterSale(id uint, from, to string, admin uint, remark string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.order.AfterSaleState != from {
		return false, nil
	}
	s.order.AfterSaleState = to
	return true, nil
}

type testRemote struct {
	mu           sync.Mutex
	state        procurementcontract.RedemptionState
	requestID    string
	replaceCalls int
	refundCalls  int
	timeout      bool
	refundIDs    map[string]bool
}

func (r *testRemote) Inspect(context.Context, string) (*procurementcontract.RedemptionState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := r.state
	return &copy, nil
}
func (r *testRemote) Replace(_ context.Context, no, id string, rev int) (*procurementcontract.RedemptionState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.requestID != "" && r.requestID != id {
		return nil, errors.New("id changed")
	}
	if r.requestID == "" {
		r.requestID = id
		r.replaceCalls++
		r.state.Revision++
		r.state.Code = "replacement"
	}
	if r.timeout {
		r.timeout = false
		return nil, errors.New("timeout after remote commit")
	}
	copy := r.state
	return &copy, nil
}
func (r *testRemote) Refund(_ context.Context, no, id string, rev int) (*procurementcontract.RedemptionState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.refundIDs[id] {
		r.refundCalls++
		r.refundIDs[id] = true
	}
	r.state.Financial = "refunded"
	r.state.Progress = "destroyed"
	copy := r.state
	return &copy, nil
}

type testConnections struct{ remote *testRemote }

func (c testConnections) OpenAfterSale(uint) (Remote, error) { return c.remote, nil }
func fixture() (*Service, *testStore, *testRemote) {
	store := &testStore{order: procurementdomain.Order{ID: 1, Status: "fulfilled", LocalOrderID: 9, UpstreamOrderNo: "test"}}
	remote := &testRemote{state: procurementcontract.RedemptionState{Code: "original", Revision: 1, Financial: "paid", Progress: "unused", CanReplace: true, CanRefund: true}, refundIDs: map[string]bool{}}
	return &Service{Store: store, Connections: testConnections{remote}}, store, remote
}
func TestReplacementRecoveryNeverChangesIdentity(t *testing.T) {
	for _, failure := range []string{"remote_timeout", "local_commit"} {
		t.Run(failure, func(t *testing.T) {
			s, store, remote := fixture()
			if failure == "remote_timeout" {
				remote.timeout = true
			} else {
				store.failCommit = true
			}
			if _, err := s.Replace(context.Background(), 1, "fixed", 1); err == nil {
				t.Fatal("expected uncertain first result")
			}
			if _, err := s.Replace(context.Background(), 1, "new-id", 1); err == nil {
				t.Fatal("new identity accepted during uncertain operation")
			}
			got, err := s.Replace(context.Background(), 1, "fixed", 1)
			if err != nil {
				t.Fatal(err)
			}
			if got.Revision != 2 || store.delivery != "replacement" || remote.replaceCalls != 1 {
				t.Fatal("replacement duplicated or not persisted")
			}
			if _, err = s.Replace(context.Background(), 1, "fixed", 1); err != nil {
				t.Fatal(err)
			}
			if remote.replaceCalls != 1 {
				t.Fatal("replay replaced again")
			}
			if _, err = s.Replace(context.Background(), 1, "fixed", 2); err == nil {
				t.Fatal("same identity accepted changed revision")
			}
		})
	}
}
func TestRefundRequiresReviewAndRecoversLocalCreditFailure(t *testing.T) {
	s, store, remote := fixture()
	ctx := context.Background()
	credits := 0
	fail := true
	s.Credit = func(uint) error {
		if fail {
			fail = false
			return errors.New("wallet unavailable")
		}
		credits++
		return nil
	}
	if err := s.RequestRefund(ctx, 1, "refund-id", "customer request"); err != nil {
		t.Fatal(err)
	}
	if remote.refundCalls != 0 || credits != 0 || store.order.AfterSaleState != "pending_review" {
		t.Fatal("request moved funds before review")
	}
	if _, err := s.Replace(ctx, 1, "replace-id", 1); err == nil {
		t.Fatal("replacement accepted while refund pending")
	}
	if err := s.ReviewRefund(ctx, 1, true, 2, "approved"); err == nil {
		t.Fatal("expected credit failure")
	}
	if err := s.ReviewRefund(ctx, 1, false, 2, "reject"); err == nil {
		t.Fatal("uncertain refund must not be rejected")
	}
	if err := s.ReviewRefund(ctx, 1, true, 2, "approved"); err != nil {
		t.Fatal(err)
	}
	if err := s.ReviewRefund(ctx, 1, true, 2, "approved"); err != nil {
		t.Fatal(err)
	}
	if remote.refundCalls != 1 || credits != 1 || store.order.AfterSaleState != "refunded" {
		t.Fatal("refund duplicated")
	}
}
func TestUsedCodeAndRejectedRefund(t *testing.T) {
	s, store, remote := fixture()
	remote.state.Progress = "processing"
	remote.state.CanReplace = false
	if _, err := s.Replace(context.Background(), 1, "r", 1); err == nil {
		t.Fatal("used code replaced")
	}
	remote.state.Progress = "unused"
	if err := s.RequestRefund(context.Background(), 1, "f", "reason"); err != nil {
		t.Fatal(err)
	}
	if err := s.ReviewRefund(context.Background(), 1, false, 2, "not approved"); err != nil {
		t.Fatal(err)
	}
	if store.order.AfterSaleState != "rejected" || remote.refundCalls != 0 {
		t.Fatal("rejection affected funds")
	}
}

func TestHeldOrderCannotExposeOrReplaceRedemptionCode(t *testing.T) {
	for _, replay := range []bool{false, true} {
		service, store, remote := fixture()
		if replay {
			store.order.AfterSaleKind = "replace"
			store.order.AfterSaleRequestID = "fixed"
			store.order.AfterSaleExpectedRevision = 1
			store.order.AfterSaleState = "completed"
		}
		store.accessErr = procurementcontract.ErrOrderRiskHeld
		if result, err := service.Inspect(t.Context(), 1); result != nil || !errors.Is(err, store.accessErr) {
			t.Fatalf("held inspection returned %v, %v", result, err)
		}
		if result, err := service.Replace(t.Context(), 1, "fixed", 1); result != nil || !errors.Is(err, store.accessErr) {
			t.Fatalf("held replacement returned %v, %v", result, err)
		}
		if remote.replaceCalls != 0 {
			t.Fatal("held replacement reached upstream")
		}
	}
}

func TestHeldOrderAllowsRedactedAuditInspectionOnly(t *testing.T) {
	service, store, remote := fixture()
	store.accessErr = procurementcontract.ErrOrderRiskHeld
	state, err := service.InspectForAudit(t.Context(), 1)
	if err != nil || state == nil || state.Progress != "unused" || state.Code != "" {
		t.Fatalf("expected code-free audit status, got error %v", err)
	}
	if remote.state.Code != "original" || remote.replaceCalls != 0 || remote.refundCalls != 0 {
		t.Fatal("audit inspection changed upstream state")
	}
	if state, err := service.Inspect(t.Context(), 1); state != nil || !errors.Is(err, store.accessErr) {
		t.Fatal("customer inspection bypassed the hold")
	}
}
