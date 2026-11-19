package upstreamwiring

import (
	"context"
	"github.com/dujiao-next/internal/app/container"
	orderrefund "github.com/dujiao-next/internal/modules/order/application/refund"
	aftersales "github.com/dujiao-next/internal/modules/procurement/application/aftersales"
	procurementcontract "github.com/dujiao-next/internal/modules/procurement/contract"
	"github.com/dujiao-next/internal/shared/money"
)

type redemptionAdapter interface {
	InspectRedemption(context.Context, string) (*procurementcontract.RedemptionState, error)
	RefundRedemption(context.Context, string, string, int) (*procurementcontract.RedemptionState, error)
	ReplaceRedemption(context.Context, string, string, int) (*procurementcontract.RedemptionState, error)
}
type afterSaleConnections struct{ container *container.Container }

func (a afterSaleConnections) OpenAfterSale(id uint) (aftersales.Remote, error) {
	conn, err := a.container.SiteConnectionService.GetByID(id)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, aftersales.ErrUnsupported
	}
	adapter, err := a.container.SiteConnectionService.GetAdapter(conn)
	if err != nil {
		return nil, err
	}
	remote, ok := adapter.(redemptionAdapter)
	if !ok {
		return nil, aftersales.ErrUnsupported
	}
	return redemptionRemote{adapter: remote}, nil
}

type redemptionRemote struct{ adapter redemptionAdapter }

func (r redemptionRemote) Inspect(ctx context.Context, no string) (*procurementcontract.RedemptionState, error) {
	return r.adapter.InspectRedemption(ctx, no)
}
func (r redemptionRemote) Replace(ctx context.Context, no, id string, rev int) (*procurementcontract.RedemptionState, error) {
	return r.adapter.ReplaceRedemption(ctx, no, id, rev)
}

func (r redemptionRemote) Refund(ctx context.Context, no, id string, rev int) (*procurementcontract.RedemptionState, error) {
	return r.adapter.RefundRedemption(ctx, no, id, rev)
}

func afterSaleCredit(c *container.Container) func(uint) error {
	return func(id uint) error {
		order, err := c.OrderService.GetOrderForAdmin(id)
		if err != nil {
			return err
		}
		if order.ParentID != nil {
			order, err = c.OrderService.GetOrderForAdmin(*order.ParentID)
			if err != nil {
				return err
			}
		}
		if len(order.Children) > 1 {
			return aftersales.ErrUnsupported
		}
		remaining := order.TotalAmount.Decimal.Sub(order.RefundedAmount.Decimal).Round(2)
		if remaining.IsNegative() {
			return aftersales.ErrConflict
		}
		if remaining.IsZero() {
			return nil
		}
		_, _, _, err = c.OrderRefundService.AdminRefundToWallet(orderrefund.AdminRefundToWalletInput{OrderID: order.ID, Amount: money.FromDecimal(remaining), Remark: "下游退款申请审核通过"})
		return err
	}
}
