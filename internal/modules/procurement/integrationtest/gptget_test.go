package procurement_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dujiao-next/internal/constants"
	procurementcontract "github.com/dujiao-next/internal/modules/procurement/contract"
	siteconnectionapp "github.com/dujiao-next/internal/modules/siteconnection/application"
)

func TestGptgetCancelManualUsesStringOrderAndRevision(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "refunded", true: "conflict"}[conflict], func(t *testing.T) {
			db := setupProcurementTestDB(t)
			order := createProcTestOrder(t, db, "GPTGET-REFUND-1", constants.OrderStatusFulfilling, constants.FulfillmentTypeUpstream)
			posts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					if r.URL.Path != "/api/v1/open/redemptions/orders/AP-string" {
						t.Errorf("wrong order lookup %s", r.URL.Path)
					}
					json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": map[string]any{"financial_status": "paid", "code_revision": 7, "can_refund": true}})
					return
				}
				posts++
				if r.URL.Path != "/api/v1/open/redemptions/orders/AP-string/refund" {
					t.Errorf("wrong refund path %s", r.URL.Path)
				}
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				if body["expected_code_revision"] != float64(7) || body["request_id"] != "original-trace-refund" {
					t.Errorf("wrong refund parameters %+v", body)
				}
				if conflict {
					w.WriteHeader(409)
					json.NewEncoder(w).Encode(map[string]any{"ok": false, "error_code": "revision_conflict"})
					return
				}
				json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": map[string]any{"financial_status": "refunded"}})
			}))
			defer server.Close()
			connSvc := newTestSiteConnectionService(db, "test-key", t.TempDir())
			conn, err := connSvc.Create(siteconnectionapp.CreateInput{Name: "gptget", BaseURL: server.URL, ApiKey: "key", ApiSecret: "secret", Protocol: constants.ConnectionProtocolGptget})
			if err != nil {
				t.Fatal(err)
			}
			proc := createTestProcurementOrder(t, db, conn.ID, order.ID, order.OrderNo, constants.ProcurementStatusAccepted)
			db.Model(proc).Updates(map[string]any{"upstream_order_no": "AP-string", "trace_id": "original-trace"})
			err = newTestProcurementService(db, connSvc).CancelManual(proc.ID)
			var result ProcurementOrder
			db.First(&result, proc.ID)
			if conflict {
				if !errors.Is(err, procurementcontract.ErrUpstreamCancelRefused) || result.Status != constants.ProcurementStatusAccepted {
					t.Fatalf("conflict must preserve accepted state: err=%v status=%s", err, result.Status)
				}
			} else if err != nil || result.Status != constants.ProcurementStatusCanceled {
				t.Fatalf("successful refund: err=%v status=%s", err, result.Status)
			}
			if posts != 1 {
				t.Fatalf("expected one refund request, got %d", posts)
			}
		})
	}
}
