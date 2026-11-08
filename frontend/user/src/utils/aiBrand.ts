/**
 * 按商品标题 / 分类识别 AI 品牌，用于商品图标和首页排序。
 * 只看前端拿到的公开字段，不依赖后端新增字段。
 */

export type AiBrand = 'openai' | 'grok' | 'other'

/** 品牌在商品中心的展示顺序：GPT 系列最前 */
const BRAND_ORDER: AiBrand[] = ['openai', 'grok', 'other']

const textOf = (value: unknown): string => {
  if (typeof value === 'string') return value
  if (value && typeof value === 'object') return Object.values(value as Record<string, unknown>).filter((v) => typeof v === 'string').join(' ')
  return ''
}

export const detectAiBrand = (product: any): AiBrand => {
  const haystack = [
    textOf(product?.title),
    textOf(product?.category?.name),
    textOf(product?.category?.slug),
    textOf(product?.slug),
  ].join(' ')
  if (/chatgpt|\bgpt|openai|codex/i.test(haystack)) return 'openai'
  if (/grok|\bxai\b/i.test(haystack)) return 'grok'
  return 'other'
}

/** 品牌 logo（public/images/brands 下的静态文件），未知品牌返回空串 */
export const brandLogoUrl = (brand: AiBrand): string =>
  brand === 'other' ? '' : `/images/brands/${brand}.svg`

const priceOf = (product: any): number => {
  const value = Number(product?.promotion_price_amount || product?.price_amount || 0)
  return Number.isFinite(value) ? value : 0
}

/** 同品牌放一起，GPT 在前；同品牌内按价格从低到高 */
export const sortByBrandThenPrice = <T>(products: T[]): T[] =>
  products
    .map((product, index) => ({ product, index, brand: BRAND_ORDER.indexOf(detectAiBrand(product)) }))
    .sort((a, b) => a.brand - b.brand || priceOf(a.product) - priceOf(b.product) || a.index - b.index)
    .map((entry) => entry.product)
