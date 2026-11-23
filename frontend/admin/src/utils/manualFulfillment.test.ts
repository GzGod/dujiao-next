import { describe, expect, it } from 'vitest'
import { manualFulfillmentTargets } from './manualFulfillment'
import type { AdminOrder } from '../api/types'
const order = (id: number, overrides = {}) => ({ id, status: 'fulfilling', items: [{ fulfillment_type: 'manual' }], ...overrides }) as AdminOrder

describe('manual delivery targets', () => {
  it('selects eligible children rather than the parent order', () => {
    const parent = order(1, { children: [order(2), order(3, { status: 'delivered', fulfillment: {} })] })
    expect(manualFulfillmentTargets(parent).map((o) => o.id)).toEqual([2])
  })
  it('excludes unpaid, refunded, automatic and already fulfilled orders', () => {
    for (const overrides of [{ status: 'pending_payment' }, { status: 'refunded' }, { fulfillment: {} }, { items: [{ fulfillment_type: 'auto' }] }, { items: [] }]) {
      expect(manualFulfillmentTargets(order(1, overrides))).toEqual([])
    }
  })
})
