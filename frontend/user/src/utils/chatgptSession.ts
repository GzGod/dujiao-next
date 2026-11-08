/**
 * 解析用户从 chatgpt.com/api/auth/session 复制出来的 JSON。
 *
 * 全部在浏览器本地完成：不发请求、不存储。accessToken 是 JWT，只解码 payload 读字段，不校验签名
 * （我们没有、也不需要 OpenAI 的密钥，结果仅供参考）。
 */

export type PlanKind = 'free' | 'go' | 'plus' | 'pro' | 'team' | 'enterprise' | 'edu' | 'unknown'

export interface SessionCheckResult {
  plan: PlanKind
  /** 原始 plan 字段，unknown 时展示给用户 */
  planRaw: string
  email: string
  name: string
  accountId: string
  /** personal / workspace，来自 account.structure */
  structure: string
  /** 订阅是否欠费（account.isDelinquent） */
  delinquent: boolean
  /** 订阅到期时间（ISO），令牌里没有时为空 */
  subscriptionUntil: string
  /** 当前登录会话过期时间（ISO） */
  sessionExpires: string
  /** accessToken 过期时间（ISO） */
  tokenExpires: string
}

export type SessionCheckError = 'empty' | 'not_json' | 'not_session' | 'logged_out'

export type SessionCheckOutcome =
  | { ok: true; result: SessionCheckResult }
  | { ok: false; error: SessionCheckError }

const AUTH_CLAIM = 'https://api.openai.com/auth'
const PROFILE_CLAIM = 'https://api.openai.com/profile'

const asRecord = (value: unknown): Record<string, unknown> =>
  value && typeof value === 'object' && !Array.isArray(value) ? (value as Record<string, unknown>) : {}

const asString = (value: unknown): string => (typeof value === 'string' ? value.trim() : '')

const base64UrlDecode = (segment: string): string => {
  const normalized = segment.replace(/-/g, '+').replace(/_/g, '/')
  const padded = normalized + '='.repeat((4 - (normalized.length % 4)) % 4)
  const binary = atob(padded)
  const bytes = Uint8Array.from(binary, (ch) => ch.charCodeAt(0))
  return new TextDecoder().decode(bytes)
}

/** 解码 JWT payload；格式不对返回空对象 */
export const decodeJwtPayload = (token: string): Record<string, unknown> => {
  const parts = token.split('.')
  if (parts.length < 2 || !parts[1]) return {}
  try {
    return asRecord(JSON.parse(base64UrlDecode(parts[1])))
  } catch {
    return {}
  }
}

export const normalizePlan = (raw: string): PlanKind => {
  const value = raw.toLowerCase().replace(/[\s_-]/g, '')
  if (!value) return 'unknown'
  if (value === 'free') return 'free'
  if (value === 'go' || value === 'chatgptgo') return 'go'
  if (value === 'plus' || value === 'chatgptplus') return 'plus'
  if (value.startsWith('pro')) return 'pro'
  if (value === 'team' || value === 'business' || value === 'chatgptteam') return 'team'
  if (value.startsWith('enterprise')) return 'enterprise'
  if (value.startsWith('edu')) return 'edu'
  return 'unknown'
}

/** 秒级时间戳 / ISO 字符串 → ISO；无效返回空 */
const toIso = (value: unknown): string => {
  if (typeof value === 'number' && Number.isFinite(value) && value > 0) {
    return new Date(value < 1e12 ? value * 1000 : value).toISOString()
  }
  const text = asString(value)
  if (!text) return ''
  if (/^\d+$/.test(text)) return toIso(Number(text))
  const date = new Date(text)
  return Number.isNaN(date.getTime()) ? '' : date.toISOString()
}

/** 用户可能多复制了前后的文字，截取第一个 { 到最后一个 } */
const extractJson = (input: string): string => {
  const start = input.indexOf('{')
  const end = input.lastIndexOf('}')
  return start >= 0 && end > start ? input.slice(start, end + 1) : ''
}

export const parseChatgptSession = (input: string): SessionCheckOutcome => {
  const text = String(input || '').trim()
  if (!text) return { ok: false, error: 'empty' }

  let data: Record<string, unknown>
  try {
    data = asRecord(JSON.parse(extractJson(text) || text))
  } catch {
    return { ok: false, error: 'not_json' }
  }

  // 未登录时接口返回 {}
  if (Object.keys(data).length === 0) return { ok: false, error: 'logged_out' }

  const user = asRecord(data.user)
  const account = asRecord(data.account)
  const accessToken = asString(data.accessToken)
  if (!accessToken && Object.keys(user).length === 0 && Object.keys(account).length === 0) {
    return { ok: false, error: 'not_session' }
  }

  const claims = accessToken ? decodeJwtPayload(accessToken) : {}
  const auth = asRecord(claims[AUTH_CLAIM])
  const profile = asRecord(claims[PROFILE_CLAIM])

  // session 里的 account.planType 最贴近网页显示；没有时退回令牌里的 chatgpt_plan_type
  const planRaw = asString(account.planType) || asString(auth.chatgpt_plan_type)

  return {
    ok: true,
    result: {
      plan: normalizePlan(planRaw),
      planRaw,
      email: asString(user.email) || asString(profile.email),
      name: asString(user.name),
      accountId: asString(account.id) || asString(auth.chatgpt_account_id),
      structure: asString(account.structure),
      delinquent: account.isDelinquent === true,
      subscriptionUntil: toIso(auth.chatgpt_subscription_active_until),
      sessionExpires: toIso(data.expires),
      tokenExpires: toIso(claims.exp),
    },
  }
}

/** 邮箱打码：ab***@gmail.com */
export const maskEmail = (email: string): string => {
  const [local = '', domain] = email.split('@')
  if (!domain) return email
  const head = local.slice(0, Math.min(2, local.length))
  return `${head}***@${domain}`
}
