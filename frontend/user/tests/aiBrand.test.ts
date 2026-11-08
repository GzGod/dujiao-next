import test from 'node:test'
import assert from 'node:assert/strict'
import { brandLogoUrl, detectAiBrand, sortByBrandThenPrice } from '../src/utils/aiBrand.ts'

const p = (title: string, price: string, category = '') => ({
  title: { 'zh-CN': title },
  price_amount: price,
  category: category ? { name: { 'zh-CN': category } } : undefined,
})

test('detects brand from title or category', () => {
  assert.equal(detectAiBrand(p('GPT Plus（菲律宾直充）', '123')), 'openai')
  assert.equal(detectAiBrand(p('Grok Plus · 月卡', '64')), 'grok')
  assert.equal(detectAiBrand(p('会员月卡', '10', 'OpenAI')), 'openai')
  assert.equal(detectAiBrand(p('会员月卡', '10', 'XAI')), 'grok')
  assert.equal(detectAiBrand(p('Spotify', '10')), 'other')
  assert.equal(brandLogoUrl('openai'), '/images/brands/openai.svg')
  assert.equal(brandLogoUrl('other'), '')
})

test('puts all GPT products first, then sorts by price within a brand', () => {
  const sorted = sortByBrandThenPrice([
    p('Grok Ultra · 月卡', '1575'),
    p('GPT Pro 5×', '684'),
    p('Grok Plus · 月卡', '64'),
    p('GPT Go（月付）', '30'),
    p('Spotify', '5'),
    p('GPT Plus', '123'),
  ])
  assert.deepEqual(
    sorted.map((x) => x.title['zh-CN']),
    ['GPT Go（月付）', 'GPT Plus', 'GPT Pro 5×', 'Grok Plus · 月卡', 'Grok Ultra · 月卡', 'Spotify'],
  )
})
