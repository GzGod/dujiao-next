<template>
  <div class="min-h-screen bg-background text-foreground">
    <div class="container mx-auto max-w-5xl px-4 pt-24 pb-16">
      <header class="mb-10 border-b pb-8">
        <p class="text-xs font-semibold uppercase tracking-[0.2em] text-primary">{{ t('apiDocs.eyebrow') }}</p>
        <h1 class="mt-3 text-3xl font-black tracking-tight md:text-4xl">{{ t('apiDocs.title') }}</h1>
        <p class="mt-4 max-w-3xl leading-relaxed text-muted-foreground">{{ t('apiDocs.subtitle') }}</p>
        <div class="mt-6 flex flex-wrap items-center gap-3">
          <code class="rounded-lg border bg-secondary px-3 py-2 font-mono text-sm break-all">{{ baseUrl }}</code>
          <Button variant="outline" size="sm" @click="copy(baseUrl, 'base')">
            <Check v-if="copiedKey === 'base'" class="h-4 w-4" aria-hidden="true" />
            <Copy v-else class="h-4 w-4" aria-hidden="true" />
            {{ t('apiDocs.copy') }}
          </Button>
          <router-link to="/me/api" class="text-sm font-medium text-primary underline-offset-4 hover:underline">
            {{ t('apiDocs.getCredential') }}
          </router-link>
        </div>
      </header>

      <nav class="mb-12 flex flex-wrap gap-2 text-sm">
        <a v-for="s in sections" :key="s.id" :href="`#${s.id}`"
          class="rounded-full border px-3 py-1 text-muted-foreground transition-colors hover:border-primary hover:text-foreground">
          {{ s.label }}
        </a>
      </nav>

      <!-- 1. 接入准备 -->
      <section id="prepare" class="doc-section">
        <h2>{{ t('apiDocs.prepare.title') }}</h2>
        <ol class="doc-list list-decimal">
          <li v-for="(_, i) in 5" :key="i">{{ t(`apiDocs.prepare.steps.${i}`) }}</li>
        </ol>
        <div class="doc-callout">{{ t('apiDocs.prepare.dujiaoTip') }}</div>
      </section>

      <!-- 2. 签名 -->
      <section id="auth" class="doc-section">
        <h2>{{ t('apiDocs.auth.title') }}</h2>
        <p>{{ t('apiDocs.auth.intro') }}</p>
        <div class="doc-table">
          <div v-for="h in authHeaders" :key="h.name" class="doc-row">
            <code>{{ h.name }}</code>
            <span>{{ t(h.desc) }}</span>
          </div>
        </div>
        <p class="mt-4">{{ t('apiDocs.auth.stringToSign') }}</p>
        <pre class="doc-code">METHOD + "\n" + PATH + "\n" + TIMESTAMP + "\n" + MD5(BODY)
signature = hex( HMAC_SHA256( secret, string_to_sign ) )</pre>
        <ul class="doc-list list-disc">
          <li v-for="(_, i) in 4" :key="i">{{ t(`apiDocs.auth.rules.${i}`) }}</li>
        </ul>
      </section>

      <!-- 3. 接口 -->
      <section id="endpoints" class="doc-section">
        <h2>{{ t('apiDocs.endpoints.title') }}</h2>
        <p>{{ t('apiDocs.endpoints.intro') }}</p>

        <div v-for="ep in endpoints" :key="ep.id" :id="`ep-${ep.id}`" class="mt-8 rounded-2xl border bg-card p-5 md:p-6">
          <div class="flex flex-wrap items-center gap-3">
            <span class="rounded-md px-2 py-0.5 font-mono text-xs font-bold"
              :class="ep.method === 'GET' ? 'bg-success/15 text-success' : 'bg-primary/15 text-primary'">{{ ep.method }}</span>
            <code class="font-mono text-sm break-all">{{ ep.path }}</code>
          </div>
          <p class="mt-3 text-sm text-muted-foreground">{{ t(`apiDocs.endpoints.${ep.id}.desc`) }}</p>

          <div v-if="ep.params" class="mt-4">
            <div class="doc-subtitle">{{ t('apiDocs.endpoints.params') }}</div>
            <div class="doc-table">
              <div v-for="p in ep.params" :key="p" class="doc-row">
                <code>{{ p }}</code>
                <span>{{ t(`apiDocs.endpoints.${ep.id}.fields.${p}`) }}</span>
              </div>
            </div>
          </div>

          <div v-if="ep.request" class="mt-4">
            <div class="doc-subtitle">{{ t('apiDocs.endpoints.request') }}</div>
            <pre class="doc-code">{{ ep.request }}</pre>
          </div>
          <div class="mt-4">
            <div class="doc-subtitle">{{ t('apiDocs.endpoints.response') }}</div>
            <pre class="doc-code">{{ ep.response }}</pre>
          </div>
          <ul v-if="ep.notes" class="doc-list mt-4 list-disc text-sm">
            <li v-for="n in ep.notes" :key="n">{{ t(`apiDocs.endpoints.${ep.id}.notes.${n}`) }}</li>
          </ul>
        </div>
      </section>

      <!-- 4. 订单状态 -->
      <section id="status" class="doc-section">
        <h2>{{ t('apiDocs.status.title') }}</h2>
        <div class="doc-table">
          <div v-for="s in orderStatuses" :key="s" class="doc-row">
            <code>{{ s }}</code>
            <span>{{ t(`apiDocs.status.items.${s}`) }}</span>
          </div>
        </div>
      </section>

      <!-- 5. 回调 -->
      <section id="callback" class="doc-section">
        <h2>{{ t('apiDocs.callback.title') }}</h2>
        <p>{{ t('apiDocs.callback.intro') }}</p>
        <pre class="doc-code">{{ callbackExample }}</pre>
        <ul class="doc-list list-disc">
          <li v-for="(_, i) in 6" :key="i">{{ t(`apiDocs.callback.rules.${i}`) }}</li>
        </ul>
      </section>

      <!-- 6. 错误码 -->
      <section id="errors" class="doc-section">
        <h2>{{ t('apiDocs.errors.title') }}</h2>
        <p>{{ t('apiDocs.errors.intro') }}</p>
        <div class="overflow-x-auto rounded-xl border">
          <table class="w-full min-w-[560px] text-sm">
            <thead class="bg-muted/40 text-left text-xs text-muted-foreground">
              <tr>
                <th class="px-4 py-2">HTTP</th>
                <th class="px-4 py-2">error_code</th>
                <th class="px-4 py-2">{{ t('apiDocs.errors.handling') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y">
              <tr v-for="e in errorCodes" :key="e.code">
                <td class="px-4 py-2 font-mono">{{ e.http }}</td>
                <td class="px-4 py-2 font-mono">{{ e.code }}</td>
                <td class="px-4 py-2 text-muted-foreground">{{ t(`apiDocs.errors.items.${e.code}`) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <!-- 7. 示例代码 -->
      <section id="sample" class="doc-section">
        <h2>{{ t('apiDocs.sample.title') }}</h2>
        <p>{{ t('apiDocs.sample.intro') }}</p>
        <div class="relative">
          <Button variant="outline" size="sm" class="absolute right-3 top-3" @click="copy(nodeSample, 'node')">
            <Check v-if="copiedKey === 'node'" class="h-4 w-4" aria-hidden="true" />
            <Copy v-else class="h-4 w-4" aria-hidden="true" />
            {{ t('apiDocs.copy') }}
          </Button>
          <pre class="doc-code pt-12">{{ nodeSample }}</pre>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, Copy } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { copyText } from '../utils/clipboard'
import { usePageSeo } from '../composables/usePageSeo'

const { t } = useI18n()

usePageSeo({
  title: () => t('apiDocs.title'),
  canonicalPath: () => '/api-docs',
})

const origin = typeof window !== 'undefined' ? window.location.origin : ''
const baseUrl = `${origin}/api/v1/upstream`

const sections = computed(() => [
  { id: 'prepare', label: t('apiDocs.prepare.title') },
  { id: 'auth', label: t('apiDocs.auth.title') },
  { id: 'endpoints', label: t('apiDocs.endpoints.title') },
  { id: 'status', label: t('apiDocs.status.title') },
  { id: 'callback', label: t('apiDocs.callback.title') },
  { id: 'errors', label: t('apiDocs.errors.title') },
  { id: 'sample', label: t('apiDocs.sample.title') },
])

const authHeaders = [
  { name: 'Dujiao-Next-Api-Key', desc: 'apiDocs.auth.headers.key' },
  { name: 'Dujiao-Next-Timestamp', desc: 'apiDocs.auth.headers.timestamp' },
  { name: 'Dujiao-Next-Signature', desc: 'apiDocs.auth.headers.signature' },
]

const endpoints = [
  {
    id: 'ping', method: 'POST', path: '/api/v1/upstream/ping',
    response: `{
  "ok": true,
  "site_name": "",
  "protocol_version": "1.0",
  "user_id": 1,
  "balance": "1000.00",
  "currency": "CNY",
  "member_level": { "id": 2, "slug": "dealer-pro", "name": { "zh-CN": "中级代理" } }
}`,
  },
  {
    id: 'categories', method: 'GET', path: '/api/v1/upstream/categories',
    response: `{
  "ok": true,
  "categories": [
    { "id": 1, "parent_id": 0, "slug": "openai", "name": { "zh-CN": "OpenAI" }, "icon": "", "sort_order": 0 }
  ]
}`,
  },
  {
    id: 'products', method: 'GET', path: '/api/v1/upstream/products?page=1&page_size=50',
    params: ['page', 'page_size', 'updated_after', 'include_inactive'],
    response: `{
  "ok": true,
  "total": 9,
  "page": 1,
  "page_size": 50,
  "includes_inactive": false,
  "items": [
    {
      "id": 27,
      "title": { "zh-CN": "GPT Go（月付）" },
      "price_amount": "28.50",
      "original_price": "30.00",
      "member_price": "28.50",
      "fulfillment_type": "auto",
      "is_active": true,
      "category_id": 1,
      "skus": [
        {
          "id": 9,
          "sku_code": "DEFAULT",
          "price_amount": "28.50",
          "original_price": "30.00",
          "member_price": "28.50",
          "stock_status": "unlimited",
          "stock_quantity": -1,
          "is_active": true
        }
      ],
      "updated_at": "2026-11-06T07:00:00Z"
    }
  ]
}`,
    notes: ['price', 'stock'],
  },
  {
    id: 'product', method: 'GET', path: '/api/v1/upstream/products/:id',
    response: `{
  "ok": true,
  "product": {
    "id": 27,
    "title": { "zh-CN": "GPT Go（月付）" },
    "price_amount": "28.50",
    "original_price": "30.00",
    "is_active": true,
    "skus": [ { "id": 9, "price_amount": "28.50", "stock_status": "unlimited", "stock_quantity": -1, "is_active": true } ]
  }
}`,
    notes: ['inactive'],
  },
  {
    id: 'createOrder', method: 'POST', path: '/api/v1/upstream/orders',
    params: ['sku_id', 'quantity', 'downstream_order_no', 'callback_url', 'trace_id', 'manual_form_data'],
    request: `{
  "sku_id": 9,
  "quantity": 1,
  "downstream_order_no": "your-order-001",
  "callback_url": "https://your-site.example/api/v1/upstream/callback",
  "trace_id": "your-trace-001"
}`,
    response: `{
  "ok": true,
  "order_id": 128,
  "order_no": "DJ20261108070000123456",
  "status": "paid",
  "amount": "28.50",
  "currency": "CNY"
}`,
    notes: ['balance', 'idempotent', 'paymentFailed', 'async'],
  },
  {
    id: 'getOrder', method: 'GET', path: '/api/v1/upstream/orders/:order_id',
    response: `{
  "ok": true,
  "order_id": 128,
  "order_no": "DJ20261108070000123456",
  "status": "delivered",
  "amount": "28.50",
  "refunded_amount": "0.00",
  "currency": "CNY",
  "refund_records": [],
  "fulfillment": {
    "type": "upstream",
    "status": "delivered",
    "payload": "XXXX-XXXX-XXXX",
    "delivery_data": {},
    "delivered_at": "2026-11-08T07:00:35Z"
  },
  "items": [
    { "product_id": 27, "sku_id": 9, "quantity": 1, "unit_price": "28.50", "total_price": "28.50" }
  ]
}`,
    notes: ['fulfillment', 'poll'],
  },
  {
    id: 'cancelOrder', method: 'POST', path: '/api/v1/upstream/orders/:order_id/cancel',
    response: `{ "ok": true, "order_id": 128, "order_no": "DJ20261108070000123456", "status": "canceled" }`,
    notes: ['onlyUnpaid'],
  },
]

const orderStatuses = ['pending_payment', 'paid', 'fulfilling', 'delivered', 'completed', 'canceled', 'partially_refunded', 'refunded']

const callbackExample = `POST {callback_url}
Content-Type: application/json
Dujiao-Next-Api-Key: <your api key>
Dujiao-Next-Timestamp: 1794000000
Dujiao-Next-Signature: <hmac>

{
  "event": "order.fulfilled",
  "order_id": 128,
  "order_no": "DJ20261108070000123456",
  "downstream_order_no": "your-order-001",
  "status": "delivered",
  "fulfillment": {
    "type": "upstream",
    "status": "delivered",
    "payload": "XXXX-XXXX-XXXX",
    "delivery_data": {},
    "delivered_at": "2026-11-08T07:00:35Z"
  },
  "timestamp": 1794000000
}

HTTP/1.1 200 OK
{"ok": true}`

const errorCodes = [
  { http: 401, code: 'missing_auth_headers' },
  { http: 401, code: 'invalid_timestamp' },
  { http: 401, code: 'timestamp_expired' },
  { http: 401, code: 'invalid_signature' },
  { http: 403, code: 'invalid_api_key' },
  { http: 403, code: 'user_disabled' },
  { http: 400, code: 'bad_request' },
  { http: 400, code: 'invalid_callback_url' },
  { http: 400, code: 'sku_unavailable' },
  { http: 400, code: 'product_unavailable' },
  { http: 402, code: 'insufficient_balance' },
  { http: 200, code: 'payment_failed' },
  { http: 409, code: 'insufficient_stock' },
  { http: 404, code: 'order_not_found' },
  { http: 404, code: 'product_not_found' },
  { http: 409, code: 'cancel_not_allowed' },
  { http: 429, code: 'rate_limited' },
  { http: 500, code: 'internal_error' },
]

const nodeSample = `// Node.js 18+，只能放在你的服务器端，不要放到浏览器或前端代码里
import { createHash, createHmac } from 'node:crypto'

const BASE = '${origin}'
const API_KEY = process.env.SHOP_API_KEY
const API_SECRET = process.env.SHOP_API_SECRET

export async function shopRequest(method, path, payload) {
  const body = payload === undefined ? '' : JSON.stringify(payload)
  const timestamp = Math.floor(Date.now() / 1000).toString()
  const signPath = path.split('?')[0]           // 签名不含 query
  const bodyMD5 = createHash('md5').update(body).digest('hex')
  const signature = createHmac('sha256', API_SECRET)
    .update([method, signPath, timestamp, bodyMD5].join('\\n'))
    .digest('hex')

  const res = await fetch(BASE + path, {
    method,
    headers: {
      'Content-Type': 'application/json',
      'Dujiao-Next-Api-Key': API_KEY,
      'Dujiao-Next-Timestamp': timestamp,
      'Dujiao-Next-Signature': signature,
    },
    ...(method === 'GET' ? {} : { body }),   // 发送的字节必须和签名时一致
  })
  const data = await res.json()
  if (!res.ok || data.ok !== true) {
    throw new Error(data.error_code || ('HTTP_' + res.status))
  }
  return data
}

// const me = await shopRequest('POST', '/api/v1/upstream/ping')
// const list = await shopRequest('GET', '/api/v1/upstream/products?page=1&page_size=50')
// const order = await shopRequest('POST', '/api/v1/upstream/orders', {
//   sku_id: 9, quantity: 1, downstream_order_no: 'your-order-001',
// })
// const detail = await shopRequest('GET', \`/api/v1/upstream/orders/\${order.order_id}\`)`

const copiedKey = ref('')
const copy = async (text: string, key: string) => {
  try {
    await copyText(text)
    copiedKey.value = key
    window.setTimeout(() => {
      if (copiedKey.value === key) copiedKey.value = ''
    }, 1500)
  } catch {
    // 复制失败时保持原状
  }
}
</script>

<style scoped>
.doc-section {
  scroll-margin-top: 6rem;
  margin-bottom: 3.5rem;
}
.doc-section h2 {
  margin-bottom: 1rem;
  font-size: 1.5rem;
  font-weight: 800;
  letter-spacing: -0.01em;
}
.doc-section > p {
  margin-bottom: 1rem;
  line-height: 1.75;
  color: var(--color-muted-foreground);
}
.doc-list {
  margin-top: 0.75rem;
  padding-left: 1.25rem;
  line-height: 1.75;
  color: var(--color-muted-foreground);
}
.doc-list > li + li {
  margin-top: 0.4rem;
}
.doc-callout {
  margin-top: 1.25rem;
  border-radius: 0.75rem;
  border: 1px solid var(--color-border);
  background: var(--color-secondary);
  padding: 1rem 1.25rem;
  font-size: 0.875rem;
  line-height: 1.7;
}
.doc-subtitle {
  margin-bottom: 0.5rem;
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--color-muted-foreground);
}
.doc-table {
  overflow: hidden;
  border-radius: 0.75rem;
  border: 1px solid var(--color-border);
}
.doc-row {
  display: grid;
  grid-template-columns: minmax(10rem, 14rem) 1fr;
  gap: 1rem;
  padding: 0.65rem 1rem;
  font-size: 0.875rem;
  line-height: 1.6;
}
.doc-row + .doc-row {
  border-top: 1px solid var(--color-border);
}
.doc-row code {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  word-break: break-all;
}
.doc-row span {
  color: var(--color-muted-foreground);
}
.doc-code {
  overflow-x: auto;
  border-radius: 0.75rem;
  border: 1px solid var(--color-border);
  background: var(--color-secondary);
  padding: 1rem 1.25rem;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 0.8125rem;
  line-height: 1.65;
  white-space: pre;
}
@media (max-width: 640px) {
  .doc-row {
    grid-template-columns: 1fr;
    gap: 0.25rem;
  }
}
</style>
