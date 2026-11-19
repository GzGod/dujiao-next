import type { AdminFulfillment, AdminOrder } from '@/api/types'

export interface IssuedCardRecord {
  key: string
  order: AdminOrder
  buyer: AdminOrder
  fulfillment: AdminFulfillment
  content: string
}

export function fulfillmentContent(fulfillment: AdminFulfillment): string {
  if (fulfillment.payload?.trim()) return fulfillment.payload
  const data = fulfillment.delivery_data
  if (!data || typeof data !== 'object') return ''
  const lines: string[] = []
  if (typeof data.note === 'string' && data.note.trim()) lines.push(data.note)
  if (Array.isArray(data.entries)) {
    for (const entry of data.entries) {
      if (!entry || typeof entry !== 'object') continue
      const key = String(entry.key ?? '').trim()
      const value = String(entry.value ?? '').trim()
      if (key || value) lines.push([key, value].filter(Boolean).join(': '))
    }
  }
  for (const [key, value] of Object.entries(data)) {
    if (key === 'note' || key === 'entries' || value == null || typeof value === 'object') continue
    if (String(value).trim()) lines.push(`${key}: ${value}`)
  }
  return lines.join('\n')
}

export function issuedCardRecords(orders: AdminOrder[]): IssuedCardRecord[] {
  const rows: IssuedCardRecord[] = []
  const seen = new Set<number>()
  for (const buyer of orders) {
    for (const order of [buyer, ...(buyer.children || [])]) {
      const fulfillment = order.fulfillment
      if (!fulfillment || fulfillment.status !== 'delivered' || seen.has(fulfillment.id)) continue
      seen.add(fulfillment.id)
      rows.push({ key: `${order.id}:${fulfillment.id}`, buyer, order, fulfillment, content: fulfillmentContent(fulfillment) })
    }
  }
  return rows
}
