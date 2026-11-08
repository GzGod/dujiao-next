import test from 'node:test'
import assert from 'node:assert/strict'
import { maskEmail, normalizePlan, parseChatgptSession } from '../src/utils/chatgptSession.ts'

const b64url = (value: unknown) =>
  Buffer.from(JSON.stringify(value)).toString('base64').replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')

const makeToken = (payload: unknown) => `${b64url({ alg: 'RS256' })}.${b64url(payload)}.sig`

test('reads plan, email and expiry from a plus session', () => {
  const token = makeToken({
    exp: 1794000000,
    'https://api.openai.com/auth': {
      chatgpt_plan_type: 'plus',
      chatgpt_account_id: 'acc-1',
      chatgpt_subscription_active_until: '2026-12-01T00:00:00Z',
    },
    'https://api.openai.com/profile': { email: 'token@example.com' },
  })
  const session = {
    user: { email: 'user@example.com', name: '张三' },
    expires: '2027-02-01T00:00:00.000Z',
    account: { id: 'acc-1', planType: 'plus', structure: 'personal', isDelinquent: false },
    accessToken: token,
  }
  const outcome = parseChatgptSession(`复制内容：${JSON.stringify(session)} 结束`)
  assert.equal(outcome.ok, true)
  if (!outcome.ok) return
  assert.equal(outcome.result.plan, 'plus')
  assert.equal(outcome.result.email, 'user@example.com')
  assert.equal(outcome.result.name, '张三')
  assert.equal(outcome.result.subscriptionUntil, '2026-12-01T00:00:00.000Z')
  assert.equal(outcome.result.tokenExpires, new Date(1794000000 * 1000).toISOString())
  assert.equal(outcome.result.delinquent, false)
})

test('falls back to token claims when account block is missing', () => {
  const token = makeToken({
    'https://api.openai.com/auth': { chatgpt_plan_type: 'free' },
    'https://api.openai.com/profile': { email: 'free@example.com' },
  })
  const outcome = parseChatgptSession(JSON.stringify({ accessToken: token }))
  assert.equal(outcome.ok, true)
  if (!outcome.ok) return
  assert.equal(outcome.result.plan, 'free')
  assert.equal(outcome.result.email, 'free@example.com')
  assert.equal(outcome.result.subscriptionUntil, '')
})

test('reports clear errors for bad input', () => {
  assert.deepEqual(parseChatgptSession('   '), { ok: false, error: 'empty' })
  assert.deepEqual(parseChatgptSession('{not json'), { ok: false, error: 'not_json' })
  assert.deepEqual(parseChatgptSession('{}'), { ok: false, error: 'logged_out' })
  assert.deepEqual(parseChatgptSession('{"foo":1}'), { ok: false, error: 'not_session' })
})

test('normalizes plan names and masks email', () => {
  assert.equal(normalizePlan('pro'), 'pro')
  assert.equal(normalizePlan('prolite'), 'pro')
  assert.equal(normalizePlan('chatgptplus'), 'plus')
  assert.equal(normalizePlan('team'), 'team')
  assert.equal(normalizePlan('weird'), 'unknown')
  assert.equal(maskEmail('abcdef@gmail.com'), 'ab***@gmail.com')
})
