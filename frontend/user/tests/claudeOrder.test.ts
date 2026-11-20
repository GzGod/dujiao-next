import { test } from 'node:test'
import assert from 'node:assert/strict'
import { hasClaudeItems } from '../src/utils/claudeOrder.ts'
test('Claude delivery identifies all plans and localized child orders', () => {
  for (const title of ['Claude Pro 不带 IP', 'Claude Max 带 IP', {'zh-CN':'Claude Pro 美区成品免K号'}, {'en-US':'Claude Max'}]) assert.equal(hasClaudeItems({items:[{title}]}),true)
  assert.equal(hasClaudeItems({items:[{title:'GPT Plus'}],children:[{items:[{title:'Claude Pro'}]}]}),true)
})
test('Claude guidance leaves GPT and empty orders unchanged', () => {
  for (const order of [null, {}, {items:[{title:'GPT Plus'}]}, {items:[{}]}]) assert.equal(hasClaudeItems(order),false)
})
