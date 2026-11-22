import data from '../../../../internal/shared/productguide/content.json'

export interface ProductGuide {
  kind: string
  summary: string
  suitable: string
  before: string[]
  steps: { title: string; text: string }[]
  faqs: { q: string; a: string }[]
}

export function getProductGuide(slug: string | undefined, locale: string): ProductGuide | null {
  const products = data.products as Record<string, any>
  const product = products[slug || ''] || Object.values(products).find((p) => p.canonicalSlug === slug)
  if (!product) return null
  const lang = Object.prototype.hasOwnProperty.call(data.labels, locale) ? locale : 'zh-CN'
  const specific = product.locales[lang]
  const common = (data.common as Record<string, any>)[product.kind][lang]
  return { ...specific, kind: product.kind, before: [...specific.checks, ...common.before], steps: common.steps, faqs: [...specific.faqs, ...common.faqs] }
}

export function productGuideLabels(locale: string): Record<string, string> {
  return (data.labels as Record<string, Record<string, string>>)[locale] || data.labels['zh-CN']
}

export function siteBuyingContent(locale: string) {
  return (data.site as Record<string, typeof data.site["zh-CN"]>)[locale] || data.site["zh-CN"]
}
