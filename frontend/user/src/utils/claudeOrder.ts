export interface SupportOrder {
  order_no?: string
  status?: string
  paid_at?: string | null
  items?: { title?: unknown }[]
  children?: SupportOrder[]
}

export function hasClaudeItems(order: SupportOrder | null | undefined): boolean {
  if (!order) return false
  return Boolean(order.items?.some(item => {
    const titles = typeof item.title === 'string' ? [item.title] : item.title && typeof item.title === 'object' ? Object.values(item.title) : []
    return titles.some(title => typeof title === 'string' && /\bclaude\b/i.test(title))
  }) || order.children?.some(hasClaudeItems))
}
