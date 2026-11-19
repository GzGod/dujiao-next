export type TranslateFn = (...args: any[]) => string

export const orderStatusLabel = (t: TranslateFn, status?: string) => {
  if (!status) return '-'
  const map: Record<string, string> = {
    pending_payment: t('order.status.pending_payment'),
    paid: t('order.status.paid'),
    fulfilling: t('order.status.fulfilling'),
    partially_delivered: t('order.status.partially_delivered'),
    partially_refunded: t('order.status.partially_refunded'),
    delivered: t('order.status.delivered'),
    completed: t('order.status.completed'),
    canceled: t('order.status.canceled'),
    refunded: t('order.status.refunded'),
  }
  return map[status] || status
}

export const orderStatusClass = (status?: string) => {
  switch (status) {
    case 'pending_payment':
      return 'text-amber-700 border-amber-200 bg-amber-50 dark:text-amber-300 dark:border-amber-800 dark:bg-amber-950'
    case 'paid':
      return 'text-emerald-700 border-emerald-200 bg-emerald-50 dark:text-emerald-300 dark:border-emerald-800 dark:bg-emerald-950'
    case 'partially_delivered':
      return 'text-orange-700 border-orange-200 bg-orange-50 dark:text-orange-300 dark:border-orange-800 dark:bg-orange-950'
    case 'partially_refunded':
      return 'text-orange-700 border-orange-200 bg-orange-50 dark:text-orange-300 dark:border-orange-800 dark:bg-orange-950'
    case 'delivered':
    case 'completed':
      return 'text-slate-800 border-slate-200 bg-slate-50 dark:text-slate-300 dark:border-slate-800 dark:bg-slate-950'
    case 'canceled':
      return 'text-slate-500 border-slate-200 bg-slate-50 dark:text-slate-300 dark:border-slate-800 dark:bg-slate-950'
    case 'refunded':
      return 'text-blue-700 border-blue-200 bg-blue-50 dark:text-blue-300 dark:border-blue-800 dark:bg-blue-950'
    default:
      return 'text-slate-600 border-slate-200 bg-slate-50 dark:text-slate-300 dark:border-slate-800 dark:bg-slate-950'
  }
}

export const paymentStatusLabel = (t: TranslateFn, status?: string) => {
  if (!status) return '-'
  const map: Record<string, string> = {
    initiated: t('payment.status.initiated'),
    pending: t('payment.status.pending'),
    success: t('payment.status.success'),
    failed: t('payment.status.failed'),
    expired: t('payment.status.expired'),
  }
  return map[status] || status
}

// 支付异常码未配置翻译时原样显示，方便排查新增的异常码
export const paymentExceptionLabel = (t: TranslateFn, code?: string) => {
  if (!code) return '-'
  const key = `admin.payments.exceptions.${code}`
  const translated = t(key)
  return translated === key ? code : translated
}

export const paymentStatusClass = (status?: string) => {
  switch (status) {
    case 'success':
      return 'text-emerald-700 border-emerald-200 bg-emerald-50 dark:text-emerald-300 dark:border-emerald-800 dark:bg-emerald-950'
    case 'pending':
      return 'text-amber-700 border-amber-200 bg-amber-50 dark:text-amber-300 dark:border-amber-800 dark:bg-amber-950'
    case 'failed':
      return 'text-rose-700 border-rose-200 bg-rose-50 dark:text-rose-300 dark:border-rose-800 dark:bg-rose-950'
    case 'expired':
      return 'text-slate-500 border-slate-200 bg-slate-50 dark:text-slate-300 dark:border-slate-800 dark:bg-slate-950'
    default:
      return 'text-slate-600 border-slate-200 bg-slate-50 dark:text-slate-300 dark:border-slate-800 dark:bg-slate-950'
  }
}

export const userStatusLabel = (t: TranslateFn, status?: string) => {
  if (!status) return '-'
  const map: Record<string, string> = {
    active: t('admin.users.status.active'),
    disabled: t('admin.users.status.disabled'),
  }
  return map[status] || status
}

export const userStatusClass = (status?: string) => {
  if (status === 'active') return 'text-emerald-700 border-emerald-200 bg-emerald-50 dark:text-emerald-300 dark:border-emerald-800 dark:bg-emerald-950'
  if (status === 'disabled') return 'text-rose-700 border-rose-200 bg-rose-50 dark:text-rose-300 dark:border-rose-800 dark:bg-rose-950'
  return 'text-slate-600 border-slate-200 bg-slate-50 dark:text-slate-300 dark:border-slate-800 dark:bg-slate-950'
}
