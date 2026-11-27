/** Payment amounts are paired with their settlement currency, never the order currency. */
export function inlinePayableAmount(payment: any, order: any): { amount: string; currency: string } {
  const currency = String(payment?.currency || order?.currency || 'CNY')
  for (const key of ['payable_amount', 'amount', 'online_pay_amount']) {
    const value = payment?.[key]
    if (value !== undefined && value !== null && value !== '' && Number.isFinite(Number(value)) && Number(value) >= 0) {
      return { amount: String(value), currency }
    }
  }
  return { amount: String(order?.total_amount ?? '0'), currency: String(order?.currency || 'CNY') }
}

export function shouldRefreshInlineOrder(order: any, gpt: boolean, hasCode: boolean, expired: boolean): boolean {
  if (order?.status === 'pending_payment') return !expired
  // Paid GPT orders may still be waiting for asynchronous upstream fulfillment.
  return gpt && !hasCode && ['paid', 'processing', 'fulfilling', 'partially_delivered', 'delivered', 'completed'].includes(order?.status)
}

/** Embed only our cashier origin; a third-party payment URL remains an explicit link. */
export function embeddedGmCashierUrl(raw: string, dev = false): string {
  try {
    const url = new URL(raw)
    const trusted = url.origin === 'https://pay.xuegaogx.cc' || (dev && url.origin === 'http://127.0.0.1:4189')
    return trusted && !url.username && !url.password && /^\/(?:cashier|pay\/checkout-counter)\/[A-Za-z0-9_-]+\/?$/.test(url.pathname) ? url.href : ''
  } catch { return '' }
}

export function inlineOrderItems(order: any): any[] {
  if (!order) return []
  return [...(Array.isArray(order.items) ? order.items : []), ...(order.children || []).flatMap(inlineOrderItems)]
}
