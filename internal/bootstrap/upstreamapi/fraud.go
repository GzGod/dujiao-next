package upstreamwiring

import (
	"context"

	"errors"

	"time"

	"github.com/dujiao-next/internal/app/container"
	"github.com/dujiao-next/internal/constants"
	aftersales "github.com/dujiao-next/internal/modules/procurement/application/aftersales"
	"github.com/dujiao-next/internal/modules/procurement/contract"
)

type fraudUpstreamRow struct {
	ID      uint                      `json:"id"`
	OrderNo string                    `json:"order_no"`
	State   *contract.RedemptionState `json:"state,omitempty"`
	Error   bool                      `json:"error,omitempty"`
}

func NewFraudUpstream(c *container.Container) func(context.Context, uint, string, uint) (any, error) {
	return func(ctx context.Context, orderID uint, action string, procurementID uint) (any, error) {
		if action == "upstream_refund" && procurementID == 0 {
			return nil, errors.New("procurement id is required")
		}
		ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
		defer cancel()
		order, err := c.OrderService.GetOrderForAdmin(orderID)
		if err != nil {
			return nil, err
		}
		if order == nil || order.ParentID != nil || order.RiskHoldStatus != "rejected" {
			return nil, errors.New("fraud action requires rejected parent order")
		}
		ids := []uint{order.ID}
		for _, child := range order.Children {
			ids = append(ids, child.ID)
		}
		rows := make([]fraudUpstreamRow, 0)
		for _, id := range ids {
			p, err := c.ProcurementOrderRepo.GetByLocalOrderID(id)
			if err != nil {
				return nil, err
			}
			if p == nil || p.UpstreamOrderNo == "" || procurementID != 0 && p.ID != procurementID {
				continue
			}
			conn, err := c.SiteConnectionService.GetByID(p.ConnectionID)
			if err != nil {
				return nil, err
			}
			if conn == nil || conn.Protocol != constants.ConnectionProtocolGptget {
				continue
			}
			row := fraudUpstreamRow{ID: p.ID, OrderNo: p.UpstreamOrderNo}
			adapter, err := c.SiteConnectionService.GetAdapter(conn)
			if err == nil {
				if remote, ok := adapter.(aftersales.FraudRemote); ok {
					row.State, err = aftersales.FraudOperation(ctx, remote, p.UpstreamOrderNo, action)
					if action == "upstream_pause" && row.State != nil {
						// Refund eligibility is verified separately on refresh.
						publicState := *row.State
						publicState.CanRefund = false
						row.State = &publicState
					}
				} else {
					err = errors.New("unsupported fraud operation")
				}
			}
			row.Error = err != nil
			rows = append(rows, row)
		}
		if action == "upstream_refund" && (procurementID == 0 || len(rows) != 1 || rows[0].Error) {
			return nil, errors.New("upstream refund could not be confirmed")
		}
		return rows, nil
	}
}
