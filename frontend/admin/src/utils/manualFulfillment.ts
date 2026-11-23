import type { AdminOrder } from '../api/types'

export function manualFulfillmentTargets(order: AdminOrder | null): AdminOrder[] {
  if (!order) return []
  if (order.children?.length) return order.children.flatMap(manualFulfillmentTargets)
  if (order.fulfillment || !['paid', 'fulfilling'].includes(order.status)) return []
  if (!order.items?.length || !order.items.every((item) => item.fulfillment_type === 'manual')) return []
  return [order]
}
