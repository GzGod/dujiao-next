import test from 'node:test'
import assert from 'node:assert/strict'
import { compareWithOfficial, officialUsdFor } from '../src/utils/officialPrice.ts'

test('matches official tiers by product title', () => {
  assert.equal(officialUsdFor('GPT Plus（直充）'), 20)
  assert.equal(officialUsdFor('GPT Pro 5×（直充）'), 100)
  assert.equal(officialUsdFor('GPT Pro 10×（直充）'), 200)
  assert.equal(officialUsdFor('GPT Pro 25×（订阅升级）'), 500)
  assert.equal(officialUsdFor('GPT Go（月付）'), 8)
  assert.equal(officialUsdFor('Grok Premium · 月卡'), 30)
  assert.equal(officialUsdFor('Spotify 会员'), null)
})

test('only reports a saving when we are cheaper than official', () => {
  assert.deepEqual(compareWithOfficial('GPT Plus（直充）', 123), { usd: 20, officialCny: 134, savingCny: 11 })
  assert.deepEqual(compareWithOfficial('GPT Pro 10×（直充）', 1103), { usd: 200, officialCny: 1338, savingCny: 235 })
  // 只要便宜就展示，差几块钱也照实写出来
  assert.deepEqual(compareWithOfficial('GPT Go（月付）', 51), { usd: 8, officialCny: 54, savingCny: 3 })
  assert.deepEqual(compareWithOfficial('GPT Pro 5×（直充）', 665), { usd: 100, officialCny: 669, savingCny: 4 })
  // 与官方同价或更贵都不展示
  assert.equal(compareWithOfficial('GPT Plus', 134), null)
  assert.equal(compareWithOfficial('Grok Plus · 月卡', 64), null)
  assert.equal(compareWithOfficial('Spotify', 1), null)
})
