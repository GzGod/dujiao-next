import { describe, expect, it } from 'vitest'
import type { AdminFulfillment, AdminOrder } from '@/api/types'
import { fulfillmentContent, issuedCardRecords } from './issuedCards'

const delivery = (id: number, extra: Partial<AdminFulfillment> = {}) => ({ id, status: 'delivered', type: 'upstream', payload: `fixture-code-${id}`, ...extra }) as AdminFulfillment
const order = (id: number, extra: Partial<AdminOrder> = {}) => ({ id, order_no: `TEST-${id}`, ...extra }) as AdminOrder

describe('issuedCardRecords', () => {
  it('associates child deliveries with the parent buyer and ignores pending deliveries', () => {
    const parent = order(1, { user_id: 3, user_email: 'buyer@example.test', children: [
      order(2, { fulfillment: delivery(12) }),
      order(3, { fulfillment: delivery(13, { status: 'pending' }) }),
      order(4),
    ] })
    const guest = order(5, { guest_email: 'guest@example.test', fulfillment: delivery(15, { type: 'auto' }) })
    const rows = issuedCardRecords([parent, guest])
    expect(rows).toHaveLength(2)
    expect(rows[0]!.order.id).toBe(2)
    expect(rows[0]!.buyer.user_email).toBe('buyer@example.test')
    expect(rows[1]!.buyer.guest_email).toBe('guest@example.test')
  })
  it('preserves multiline deliveries and does not duplicate the same fulfillment', () => {
    const fulfillment = delivery(4, { payload: 'fixture-A\nfixture-B' })
    const rows = issuedCardRecords([order(1, { fulfillment, children: [order(2, { fulfillment })] })])
    expect(rows).toHaveLength(1)
    expect(rows[0]!.content).toBe('fixture-A\nfixture-B')
  })
  it('shows the latest replacement content from refreshed order data', () => {
    expect(issuedCardRecords([order(1, { fulfillment: delivery(1, { payload: 'replacement-fixture' }) })])[0]!.content).toBe('replacement-fixture')
  })
})
describe('fulfillmentContent', () => {
  it('preserves manual delivery fields without stringifying nested objects', () => {
    expect(fulfillmentContent(delivery(1, { payload: '', delivery_data: {
      note: 'Manual account', entries: [{key:'Account',value:'fixture@example.test'}, {key:'',value:'fixture-password'}], region:'US', nested:{ private:'ignored' },
    } }))).toBe('Manual account\nAccount: fixture@example.test\nfixture-password\nregion: US')
  })
  it('prefers the actually sent payload and keeps empty deliveries empty', () => {
    expect(fulfillmentContent(delivery(1, {payload:'sent-code', delivery_data:{note:'older note'}}))).toBe('sent-code')
    expect(fulfillmentContent(delivery(1, {payload:''}))).toBe('')
  })
})
