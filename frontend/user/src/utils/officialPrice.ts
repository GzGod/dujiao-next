/**
 * 官方订阅价（美区月付，美元），用于商品卡片上的「比官方省多少」。
 *
 * 价格来源：OpenAI 定价页（Go $8 / Plus $20 / Pro $100·$200·$500）与 xAI X Subscription
 * （Plus $8 / Premium $30 / Super $100 / Ultra $200），2026-11 核对。官方调价时改这里。
 * 只在我们确实比官方便宜时展示，避免对用户做误导性对比。
 */

export const USD_CNY_RATE = 6.69

interface OfficialPriceRule {
  pattern: RegExp
  usd: number
}

// 顺序重要：更具体的规则放前面（Pro 25× 要在 Pro 前面匹配）
const RULES: OfficialPriceRule[] = [
  { pattern: /gpt\s*pro\s*25\s*[x×]/i, usd: 500 },
  { pattern: /gpt\s*pro\s*10\s*[x×]/i, usd: 200 },
  { pattern: /gpt\s*pro\s*5\s*[x×]/i, usd: 100 },
  { pattern: /gpt\s*plus/i, usd: 20 },
  { pattern: /gpt\s*go/i, usd: 8 },
  { pattern: /grok\s*ultra/i, usd: 200 },
  { pattern: /grok\s*super/i, usd: 100 },
  { pattern: /grok\s*premium/i, usd: 30 },
  { pattern: /grok\s*plus/i, usd: 8 },
]

/** 至少省这么多（元）才展示，几块钱的差价不值得占位置 */
const MIN_SAVING_CNY = 5

export interface OfficialComparison {
  usd: number
  officialCny: number
  savingCny: number
}

export const officialUsdFor = (title: string): number | null => {
  const rule = RULES.find((r) => r.pattern.test(title))
  return rule ? rule.usd : null
}

/** 返回与官方价的对比；识别不出、或不比官方便宜时返回 null */
export const compareWithOfficial = (title: string, ourPriceCny: number): OfficialComparison | null => {
  const usd = officialUsdFor(title)
  if (usd === null || !Number.isFinite(ourPriceCny) || ourPriceCny <= 0) return null
  const officialCny = Math.round(usd * USD_CNY_RATE)
  const savingCny = officialCny - Math.round(ourPriceCny)
  if (savingCny < MIN_SAVING_CNY) return null
  return { usd, officialCny, savingCny }
}
