import test from 'node:test'
import assert from 'node:assert/strict'
import { inlinePayableAmount, shouldRefreshInlineOrder, embeddedGmCashierUrl } from '../src/utils/inlinePayment.ts'
import { isKycServiceProduct } from '../src/utils/aiBrand.ts'

test('shows the actual online amount after wallet deductions and fees, in settlement currency', () => {
  const order = { total_amount: '100', currency: 'CNY' }
  assert.deepEqual(inlinePayableAmount({ online_pay_amount: '60' }, order), { amount: '60', currency: 'CNY' })
  assert.deepEqual(inlinePayableAmount({ payable_amount: '62', online_pay_amount: '60' }, order), { amount: '62', currency: 'CNY' })
  assert.deepEqual(inlinePayableAmount({ payable_amount: '7.50', currency: 'GBP' }, order), { amount: '7.50', currency: 'GBP' })
  assert.deepEqual(inlinePayableAmount({ payable_amount: '0' }, order), { amount: '0', currency: 'CNY' })
  assert.deepEqual(inlinePayableAmount({}, order), { amount: '100', currency: 'CNY' })
})
test('keeps watching delayed GPT delivery after payment, but stops at code delivery or a terminal status', () => {
  for (const status of ['paid', 'processing', 'fulfilling', 'partially_delivered']) {
    assert.equal(shouldRefreshInlineOrder({ status }, true, false, true), true)
    assert.equal(shouldRefreshInlineOrder({ status }, true, true, false), false)
  }
  for (const status of ['completed', 'delivered', 'refunded', 'canceled', 'expired']) assert.equal(shouldRefreshInlineOrder({ status }, true, false, false), false)
  assert.equal(shouldRefreshInlineOrder({ status: 'pending_payment' }, true, false, false), true)
  assert.equal(shouldRefreshInlineOrder({ status: 'pending_payment' }, true, false, true), false)
  assert.equal(shouldRefreshInlineOrder({ status: 'paid' }, false, false, false), false)
})
test('KYC verified accounts remain Claude products in every language while verification services stay in KYC', () => {
  assert.equal(isKycServiceProduct({ slug: 'claude-pro-africa-kyc-account-no-ip', title: { 'en-US': 'Claude Pro Africa KYC-Verified Account', 'zh-CN': 'Claude Pro 非洲过K号' } }), false)
  for (const title of ['Claude Pro Africa KYC-Verified Account', 'Claude Pro 非洲过K号', 'Claude Pro 非洲已過 K 帳號']) assert.equal(isKycServiceProduct({ slug: 'claude-pro-africa-kyc-account-no-ip', title }), false)
  assert.equal(isKycServiceProduct({ slug: 'korea-claude-kyc', title: '韩国 Claude KYC' }), true)
  assert.equal(isKycServiceProduct({ slug: 'portal-verification', title: '韓國 Claude 門戶驗證' }), true)
  assert.equal(isKycServiceProduct({ title: 'GPT Plus' }), false)
})

test('embeds only the owned GM cashier and never arbitrary URLs or production fixtures', () => {
  assert.equal(embeddedGmCashierUrl('https://pay.xuegaogx.cc/pay/checkout-counter/trade123'), 'https://pay.xuegaogx.cc/pay/checkout-counter/trade123')
  assert.equal(embeddedGmCashierUrl('https://evil.test/cashier/trade123'), '')
  assert.equal(embeddedGmCashierUrl('https://pay.xuegaogx.cc.evil.test/cashier/id'), '')
  assert.equal(embeddedGmCashierUrl('http://127.0.0.1:4189/cashier/candidate-select'), '')
  assert.equal(embeddedGmCashierUrl('http://127.0.0.1:4189/cashier/candidate-select', true), 'http://127.0.0.1:4189/cashier/candidate-select')
})
