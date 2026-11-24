// Only non-secret order references are persisted. Guest credentials stay session-only.
export interface PendingOrder {
  orderNo: string
  guest: boolean
  owner: string
  expiresAt: string
  createdAt: number
}
const KEY = 'xuegaoai_pending_orders_v1'
export const PENDING_ORDERS_EVENT = 'xuegaoai:pending-orders'
export function normalizePendingOrders(value: unknown, now = Date.now()): PendingOrder[] {
  if (!Array.isArray(value)) return []
  const seen = new Set<string>()
  return value.filter((row): row is PendingOrder => {
    if (!row || typeof row.orderNo !== 'string' || !/^[A-Za-z0-9_-]{1,100}$/.test(row.orderNo) || typeof row.guest !== 'boolean' || typeof row.owner !== 'string' || typeof row.expiresAt !== 'string' || !Number.isFinite(row.createdAt)) return false
    if (now - row.createdAt > 7 * 86400000 || row.createdAt > now + 60000 || seen.has(row.orderNo)) return false
    const expires = Date.parse(row.expiresAt)
    if (Number.isFinite(expires) && expires <= now) return false
    seen.add(row.orderNo)
    return true
  }).map(({orderNo, guest, owner, expiresAt, createdAt}) => ({orderNo, guest, owner, expiresAt, createdAt})).slice(0, 20)
}
export function readPendingOrders(): PendingOrder[] {
  try { return normalizePendingOrders(JSON.parse(localStorage.getItem(KEY) || '[]')) } catch { return [] }
}
function write(rows: PendingOrder[]) {
  try { localStorage.setItem(KEY, JSON.stringify(normalizePendingOrders(rows))) } catch { /* Storage may be disabled. */ }
  window.dispatchEvent(new Event(PENDING_ORDERS_EVENT))
}
export function rememberPendingOrder(row: PendingOrder) { write([row, ...readPendingOrders().filter(item => item.orderNo !== row.orderNo)]) }
export function forgetPendingOrder(orderNo: string) { write(readPendingOrders().filter(row => row.orderNo !== orderNo)) }
export function reconcilePendingOrder(orderNo: string, order: any) {
  if (!order || typeof order.status !== 'string') return
  if (order.status !== 'pending_payment' || (Number.isFinite(Date.parse(order.expires_at)) && Date.parse(order.expires_at) <= Date.now())) forgetPendingOrder(orderNo)
}
