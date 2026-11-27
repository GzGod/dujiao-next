<template>
  <section class="inline-pay" aria-live="polite">
    <div class="inline-pay__top"><button v-if="!paid && !expired" type="button" class="inline-pay__back" @click="emit('back')">← {{ t('payment.inlineBack') }}</button><small v-else>{{ t('payment.title') }}</small><span v-if="remaining && !paid && !expired" class="inline-pay__timer">{{ remaining }}</span></div>
    <div v-if="embeddedCashier" class="inline-pay__co-brand"><img class="inline-pay__shop-logo" :src="appStore.config?.brand?.site_logo || (theme === 'dark' ? '/brand/xuegaoai-horizontal-dark.svg' : '/brand/xuegaoai-horizontal.svg')" :class="{ 'inline-pay__shop-logo--default': !appStore.config?.brand?.site_logo }" alt="雪糕AI" /><span aria-hidden="true">×</span><span class="inline-pay__gm-brand"><img src="/images/payments/gmwallet.png" alt="" /><b>GMwallet</b></span></div>
    <div v-else class="inline-pay__method"><img v-if="isAlipay" src="/images/payments/alipay.ico" alt="" /><span v-else class="inline-pay__coins"><img src="/images/payments/usdt.svg" alt="" /><img src="/images/payments/usdc.svg" alt="" /></span><b>{{ currentChannel?.name || payment.channel_name || '—' }}</b></div>
    <h2>{{ paid ? t('payment.inlineSuccess') : expired ? t('payment.orderExpired') : t(isRedirect ? 'payment.redirectTitle' : 'payment.inlineScan') }}</h2>
    <strong>{{ formatPrice(payable.amount, payable.currency) }}</strong>
    <GmInlineCashier @gesture="recordGesture" v-if="embeddedCashier && !paid && !expired" :order-no="result.order_no" :guest="guest" :email="email" :password="password" @ready="emit('ready')" />
    <Transition v-if="!embeddedCashier || paid || expired" name="qr-reveal" appear mode="out-in">
      <div v-if="paid && deliveryUnderReview" class="inline-pay__result" key="review"><Clock3 /><DeliveryReviewNotice :order-no="result.order_no" /></div>
      <div v-else-if="paid" class="inline-pay__result" key="paid"><CheckCircle2 /><p>{{ t(isIp ? 'payment.ipDeliveryHint' : isClaude ? 'payment.claudeBotHint' : isGpt ? (deliveryCode ? 'payment.gptRedeemHint' : 'payment.gptDeliveryPending') : 'payment.inlineSuccessHint') }}</p><template v-if="isGpt && deliveryCode"><code class="inline-pay__code">{{ deliveryCode }}</code><button type="button" class="x-btn x-btn--ghost" @click="copyCode">{{ t(codeCopied ? 'payment.gptCodeCopied' : 'payment.gptCopyCode') }}</button></template><a v-if="isGpt" :href="appStore.redemptionUrl()" target="_blank" rel="noopener noreferrer" class="x-btn x-btn--primary">{{ t('payment.gptRedeemOpen') }} →</a><a v-if="isIp" href="https://t.me/xuegaozhanshen" target="_blank" rel="noopener noreferrer" class="x-btn x-btn--primary">{{ t('floatingSupport.title') }} →</a><a v-if="isClaude && botUrl" :href="botUrl" target="_blank" rel="noopener noreferrer" class="x-btn x-btn--primary">{{ t('payment.claudeBotOpen') }} →</a><button v-else-if="isClaude" type="button" class="x-btn x-btn--primary" :disabled="botLoading" @click="loadBot">{{ t(botLoading ? 'common.loading' : 'payment.claudeBotRetry') }}</button></div>
      <div v-else-if="waitingForExpiry" class="inline-pay__result" key="qr-review"><Clock3 /><p>{{ t(gestureCount >= 3 ? 'payment.qrIssuePaused' : 'payment.qrIssueWaiting') }}</p></div>
      <div v-else-if="expired" class="inline-pay__result" key="expired"><Clock3 /><p>{{ t('payment.orderExpired') }}</p></div>
      <div v-else-if="isRedirect && payUrl" class="inline-pay__result" key="redirect"><ExternalLink /><p>{{ t('payment.redirectTip') }}</p><a :href="payUrl" target="_blank" rel="noopener noreferrer" class="x-btn x-btn--primary">{{ t('payment.inlineOpenCashier') }} →</a></div>
      <div v-else-if="qr" class="inline-pay__qr" key="qr"><img @contextmenu.prevent="recordGesture" @dragstart.prevent="recordGesture" :src="qr" :alt="t('payment.inlineScan')" @load="emit('ready')" /><span>{{ t('payment.inlineWaiting') }}</span></div>
      <div v-else class="inline-pay__result" key="error"><AlertCircle /><p>{{ result.payment_error || t('payment.inlineUnavailable') }}</p></div>
    </Transition>
    <p v-if="isMock" class="inline-pay__mock">{{ t('payment.inlineMock') }}</p>
    <p class="inline-pay__order">{{ result.order_no }}</p>

    <p v-if="refreshError" class="inline-pay__error">{{ t('payment.inlineRefreshError') }}</p>
    <div class="inline-pay__actions">
    <a v-if="payUrl && !isRedirect && !qr && !paid && !expired" :href="payUrl" target="_blank" rel="noopener noreferrer" class="x-btn x-btn--primary">{{ t('payment.inlineOpenCashier') }} <ExternalLink :size="15" /></a>
    <button type="button" class="x-btn x-btn--ghost" :disabled="refreshing" @click="refresh">{{ t('payment.refreshStatus') }}</button>
    </div>
    <RouterLink :to="orderLink" class="x-link">{{ t('payment.inlineViewOrder') }} →</RouterLink>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import QRCode from 'qrcode'
import DeliveryReviewNotice from './DeliveryReviewNotice.vue'
import GmInlineCashier from './GmInlineCashier.vue'
import { embeddedGmCashierUrl } from '../../../utils/inlinePayment'
import { resolvePaymentPresentationMode } from '../../../utils/paymentResumePolicy'
import { inlinePayableAmount, shouldRefreshInlineOrder, inlineOrderItems } from '../../../utils/inlinePayment'
import { CheckCircle2, Clock3, AlertCircle, ExternalLink } from 'lucide-vue-next'
import { guestOrderAPI, userOrderAPI, paymentAPI } from '../../../api'
import { useTheme } from '../../../utils/theme'
import { useAppStore } from '../../../stores/app'
import { useLocalized } from '../../../composables/useProduct'

const props = defineProps<{ result: any; guest: boolean; email: string; password: string; channels: any[]; initialChannelId: number | null }>()
const emit = defineEmits<{ back: []; ready: [] }>()
const { t } = useI18n()
const { formatPrice } = useLocalized()
const appStore = useAppStore()
const { theme } = useTheme()
const payment = ref<any>(props.result.payment || props.result)
const gestureCount = ref(0)
const waitingForExpiry = ref(false)
let rotating = false
let nextCheck = 0
const rotateWhenExpired = async () => {
  if (!waitingForExpiry.value || rotating || disposed || paid.value || expired.value || gestureCount.value >= 3 || Date.now() < nextCheck) return
  nextCheck = Date.now() + 15000
  rotating = true
  try {
    const payload = { order_no: props.result.order_no, channel_id: currentChannelId.value, use_balance: Number(order.value?.wallet_paid_amount || 0) > 0 }
    const response = props.guest ? await guestOrderAPI.createPayment({ ...payload, email: props.email, order_password: props.password }) : await paymentAPI.create(payload)
    if (!disposed && response.data.data?.payment_id) {
      payment.value = response.data.data
      waitingForExpiry.value = false
    }
  } catch { /* The gateway may still accept the previous QR. */ }
  finally { rotating = false }
}
let lastGestureAt = -Infinity
const recordGesture = async (event: Event) => {
  if (!event.isTrusted || performance.now() - lastGestureAt < 800) return
  lastGestureAt = performance.now()
  const data = { order_no: props.result.order_no, payment_id: payment.value.payment_id || payment.value.id || props.result.payment_id, event_id: crypto.randomUUID(), kind: event.type }
  try {
    const response = props.guest ? await guestOrderAPI.qrEvent(data, { email: props.email, order_password: props.password }) : await userOrderAPI.qrEvent(data)
    if (isAlipay.value) {
      gestureCount.value = Math.max(gestureCount.value, Number(response.data.data?.count || 0))
      waitingForExpiry.value = gestureCount.value > 0
    }
  } catch { /* A failed report does not confirm an order-state change. */ }
}
const currentChannelId = ref(Number(payment.value.channel_id || props.initialChannelId || 0))
const currentChannel = computed(() => props.channels.find(channel => Number(channel.id) === currentChannelId.value))
const isAlipay = computed(() => String(currentChannel.value?.channel_type || payment.value.channel_type).includes('alipay'))
const isRedirect = computed(() => resolvePaymentPresentationMode(payment.value.interaction_mode || currentChannel.value?.interaction_mode) === 'redirect')
const order = ref(props.result.order)
const deliveryUnderReview = computed(() => Boolean(order.value?.delivery_under_review ?? (!order.value?.status && props.result.delivery_under_review)))
const payable = computed(() => paid.value ? { amount: String(order.value?.total_amount || props.result.order?.total_amount || '0'), currency: order.value?.currency || 'CNY' } : inlinePayableAmount(payment.value, order.value))
const isIp = computed(() => inlineOrderItems(order.value).some((item: any) => item.slug === 'us-residential-ip-monthly' || /家宽|家寬|residential IP/i.test(JSON.stringify(item.title || item.product_title || ''))))
const isClaude = computed(() => inlineOrderItems(order.value || props.result.order).some((item: any) => { const title = JSON.stringify(item.title || item.product_title || ''); return /claude/i.test(title) && /pro|max|成品|account/i.test(title) && !/门户|門戶|portal/i.test(title) }))
const isGpt = computed(() => inlineOrderItems(order.value).some((item: any) => /gpt|openai/i.test(JSON.stringify(item.title || item.product_title || ''))))
const deliveryCode = computed(() => {
  const deliveries = [order.value?.fulfillment, ...(order.value?.children || []).map((child: any) => child.fulfillment)]
  return deliveries.map(item => typeof item?.payload === 'string' ? item.payload : typeof item?.payload?.content === 'string' ? item.payload.content : '').filter(Boolean).join('\n')
})
const codeCopied = ref(false)
const copyCode = async () => { try { await navigator.clipboard.writeText(deliveryCode.value); codeCopied.value = true } catch { codeCopied.value = false } }
const botUrl = ref('')
const botLoading = ref(false)
const loadBot = async () => {
  if (botLoading.value || !paid.value || !isClaude.value || deliveryUnderReview.value) return
  botLoading.value = true
  try {
    const response = props.guest
      ? await guestOrderAPI.telegramDelivery(props.result.order_no, { email: props.email, order_password: props.password })
      : await userOrderAPI.telegramDelivery(props.result.order_no)
    const value = String(response.data.data?.url || '')
    if (/^https:\/\/t\.me\/[A-Za-z0-9_]+(?:\?start=[A-Za-z0-9_-]+)?$/.test(value) || (import.meta.env.DEV && String(props.result.order_no).startsWith('MOCK') && value.startsWith('http://127.0.0.1:8080/mock/bot'))) botUrl.value = value
  } catch { /* Keep the retry and order entry visible. */ }
  finally { botLoading.value = false }
}
const now = ref(Date.now())
const qr = ref('')
const refreshing = ref(false)
const refreshError = ref(false)
let disposed = false
let timer: ReturnType<typeof setInterval> | undefined
const paid = computed(() => props.result.order_paid || ['paid', 'processing', 'fulfilling', 'partially_delivered', 'completed', 'delivered'].includes(order.value?.status))
const expires = computed(() => new Date(order.value?.expires_at || props.result.expires_at || '').getTime())
const expired = computed(() => order.value?.status === 'canceled' || (Number.isFinite(expires.value) && expires.value <= now.value))
const remaining = computed(() => {
  if (!Number.isFinite(expires.value)) return ''
  const seconds = Math.max(0, Math.floor((expires.value - now.value) / 1000))
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`
})
const payUrl = computed(() => {
  const raw = String(payment.value.pay_url || '')
  try { return ['http:', 'https:'].includes(new URL(raw).protocol) ? raw : '' } catch { return '' }
})
const embeddedCashier = computed(() => embeddedGmCashierUrl(payUrl.value, import.meta.env.DEV))
const isMock = computed(() => import.meta.env.DEV && String(props.result.order_no).startsWith('MOCK'))
const orderLink = computed(() => `${props.guest ? '/guest/orders/' : '/orders/'}${encodeURIComponent(props.result.order_no)}`)
const refresh = async () => {
  if (refreshing.value || disposed) return
  refreshing.value = true
  try {
    const response = props.guest
      ? await guestOrderAPI.detail(props.result.order_no, { email: props.email, order_password: props.password }, { silentBusinessError: true })
      : await userOrderAPI.detail(props.result.order_no, { silentBusinessError: true })
    if (!disposed) { order.value = response.data.data; refreshError.value = false }
  } catch { if (!disposed) refreshError.value = true }
  finally { refreshing.value = false }
}
let qrRequest = 0
watch(payment, async () => {
  const request = ++qrRequest
  const content = isRedirect.value ? '' : String(payment.value.qr_code || payUrl.value)
  if (!content) { qr.value = ''; await nextTick(); emit('ready'); return }
  const data = /^data:image\/(png|jpeg|webp);base64,/.test(content) ? content : await QRCode.toDataURL(content, { width: 280, margin: 2, errorCorrectionLevel: 'M' }).catch(() => '')
  if (!disposed && request === qrRequest) { qr.value = data; await nextTick(); emit('ready') }
}, { immediate: true })
watch(() => [paid.value, isClaude.value, deliveryUnderReview.value], async () => { if (paid.value) { await nextTick(); emit('ready'); void loadBot() } }, { immediate: true })
onMounted(async () => {
  if (paid.value || expired.value || !payUrl.value && !payment.value.qr_code) emit('ready')
  await refresh()
  if (disposed) return
  timer = setInterval(() => { now.value = Date.now(); void rotateWhenExpired(); if (shouldRefreshInlineOrder(order.value, isGpt.value, Boolean(deliveryCode.value), expired.value)) void refresh() }, 2000)
})
onBeforeUnmount(() => { disposed = true; clearInterval(timer) })
</script>

<style scoped>
.inline-pay { display: flex; flex-direction: column; align-items: center; gap: 10px; padding: 14px 8px; text-align: center; }
.inline-pay small, .inline-pay__order { color: var(--x-ink-4); font-size: 11px; overflow-wrap: anywhere; }
.inline-pay h2 { margin: 0; color: var(--x-ink); font-size: 20px; font-weight: 650; }
.inline-pay > strong { color: var(--x-blue); font-size: 32px; }
.inline-pay__qr { display: grid; gap: 12px; justify-items: center; padding: 16px; background: white; border: 1px solid var(--x-line); border-radius: 20px; color: #555; font-size: 12px; box-shadow: var(--x-shadow-sm); }
.inline-pay__qr img { width: min(200px, 48vw); height: auto; }
.inline-pay__result { display: grid; place-items: center; min-height: 200px; gap: 16px; color: var(--x-blue); }
.inline-pay__result svg { width: 64px; height: 64px; }
.inline-pay__mock, .inline-pay__error { font-size: 12px; color: #a66a00; }
.qr-reveal-enter-active, .qr-reveal-leave-active { transition: opacity .3s; }
.qr-reveal-enter-from { opacity: 0; }
.qr-reveal-leave-to { opacity: 0; }
@keyframes payment-reveal { from { opacity: 0; transform: translateY(16px); } to { opacity: 1; transform: none; } }
@media(prefers-reduced-motion: reduce) { .inline-pay { animation: none; } .qr-reveal-enter-active, .qr-reveal-leave-active { transition: none; } }
.inline-pay { gap: 8px; padding: 4px 0; height: 100%; max-height: 100%; min-height: 0; }
.inline-pay__top { display: flex; align-items: center; justify-content: space-between; width: 100%; margin-bottom: 2px; }
.inline-pay__timer { font-size: 12px; color: var(--x-ink-3); font-variant-numeric: tabular-nums; border: 1px solid var(--x-line); padding: 3px 9px; border-radius: 20px; }
.inline-pay h2 { font-size: 18px; }
.inline-pay > strong { font-size: 28px; line-height: 1.3; }
.inline-pay__qr { gap: 8px; padding: 10px; border-radius: 16px; }
.inline-pay__qr img { width: min(180px, 44vw, 29dvh); }
.inline-pay__order { font-size: 10px; margin: 0; }
.inline-pay__mock { font-size: 11px; margin: 0; }
.inline-pay__actions { display: flex; justify-content: center; align-items: center; gap: 8px; width: 100%; margin-top: auto; }
.inline-pay__actions .x-btn { padding: 8px 12px; min-height: 36px; font-size: 12px; }
.inline-pay > .x-link { font-size: 12px; padding: 4px; }
.inline-pay__method, .inline-pay__coins { display: inline-flex; align-items: center; gap: 6px; }
.inline-pay__method { font-size: 12px; color: var(--x-ink-2); }
.inline-pay__method img { width: 20px; height: 20px; object-fit: contain; }
.inline-pay__choices { display: flex; flex-wrap: wrap; gap: 6px; justify-content: center; }
.inline-pay__choices .x-btn { padding: 6px 10px; font-size: 11px; }
.inline-pay__choices [aria-pressed=true] { color: var(--x-blue); border-color: var(--x-blue); }
.inline-pay__qr img { width: min(170px, 42vw, 26dvh); }
.inline-pay__back { display: inline-flex; align-items: center; padding: 4px 0; border: 0; background: transparent; color: var(--x-ink-2); cursor: pointer; font-size: 12px; }
.inline-pay__back:hover { color: var(--x-blue); }
.inline-pay__actions .x-btn { background: transparent !important; color: var(--x-ink-2) !important; border: 1px solid var(--x-line-strong) !important; box-shadow: none; }
.inline-pay__actions .x-btn:hover { background: var(--x-surface) !important; color: var(--x-ink) !important; }
.inline-pay { position: relative; padding-bottom: 82px; box-sizing: border-box; }
.inline-pay__actions { position: absolute; bottom: 36px; left: 0; margin: 0; height: 36px; }
.inline-pay > .x-link { position: absolute; bottom: 0; left: 0; right: 0; height: 30px; text-align: center; }
.inline-pay__qr { min-height: min(218px, 34dvh); }
.inline-pay__code { max-width: 100%; overflow-wrap: anywhere; padding: 10px 14px; border: 1px solid var(--x-line-strong); border-radius: 10px; font-size: 13px; color: var(--x-ink); user-select: all; }
.inline-pay__result:has(.inline-pay__code) { min-height: 0; gap: 10px; }
.inline-pay__result:has(.inline-pay__code) > svg { width: 38px; height: 38px; }
</style>

<style scoped>
.inline-pay:has(.gm-inline) { gap: 5px; }
.inline-pay:has(.gm-inline) > h2, .inline-pay:has(.gm-inline) > strong { display: none; }

</style>

<style scoped>
/* GM owns the available middle space; footer rows participate in layout. */
.inline-pay:has(.gm-inline) { padding-bottom: 0; gap: 7px; overflow: hidden; }
.inline-pay:has(.gm-inline) > .inline-pay__top,
.inline-pay:has(.gm-inline) > .inline-pay__method,
.inline-pay:has(.gm-inline) > .inline-pay__mock,
.inline-pay:has(.gm-inline) > .inline-pay__order { flex: none; }
.inline-pay:has(.gm-inline) > .inline-pay__actions { position: static; height: 32px; flex: none; margin-top: 2px; }
.inline-pay:has(.gm-inline) > .x-link { position: static; height: 24px; flex: none; padding: 2px; }
.inline-pay:has(.gm-inline) > .inline-pay__mock { margin-top: 3px; line-height: 1.5; }
.inline-pay:has(.gm-inline) > .inline-pay__order { line-height: 1.5; }
</style>

<style scoped>
.inline-pay__co-brand { width:100%;display:flex;align-items:center;justify-content:flex-start;gap:10px;margin:4px 0 8px;flex:none; }
.inline-pay__shop-logo { display:block;width:auto;height:22px;max-width:140px;object-fit:contain; }
.inline-pay__co-brand > span { color:var(--x-ink-4);font-size:17px;font-weight:400; }
.inline-pay__gm-brand { display:inline-flex;align-items:center;gap:6px; }
.inline-pay__gm-brand img { width:23px;height:23px;object-fit:contain; }
.inline-pay__gm-brand b { font-size:13px;color:var(--x-ink);font-weight:650;letter-spacing:-.3px; }

</style>

<style scoped>
.inline-pay:has(.gm-inline) { display:grid; grid-template-columns:1fr 1fr; grid-template-rows:auto auto minmax(0,1fr) auto auto; column-gap:12px; row-gap:7px; align-items:stretch; }
.inline-pay:has(.gm-inline) > .inline-pay__top { grid-column:1/-1; grid-row:1; }
.inline-pay:has(.gm-inline) > .inline-pay__co-brand { grid-column:1/-1; grid-row:2; margin:0 0 4px; }
.inline-pay:has(.gm-inline) > .gm-inline { grid-column:1/-1; grid-row:3; }
.inline-pay:has(.gm-inline) > .inline-pay__mock { grid-column:1/-1; grid-row:4; text-align:center; }
.inline-pay:has(.gm-inline) > .inline-pay__order { display:none; }
.inline-pay:has(.gm-inline) > .inline-pay__actions { grid-column:1; grid-row:5; justify-content:flex-start; margin:0; height:28px; }
.inline-pay:has(.gm-inline) > .inline-pay__actions .x-btn { border:0!important; box-shadow:none; padding:2px 0; min-height:28px; font-size:11px; }
.inline-pay:has(.gm-inline) > .x-link { grid-column:2; grid-row:5; text-align:right; padding:4px 0; height:28px; font-size:11px; }
.inline-pay:has(.gm-inline) > .inline-pay__error { grid-column:1/-1; }
</style>

<style scoped>
.inline-pay:has(.gm-inline) > .inline-pay__actions .x-btn,
.inline-pay:has(.gm-inline) > .inline-pay__actions .x-btn:hover { background:none!important; border:0!important; border-radius:0!important; box-shadow:none!important; color:var(--x-ink-3)!important; }
.inline-pay:has(.gm-inline) > .inline-pay__actions { align-self:end; justify-self:start; width:auto; }
.inline-pay:has(.gm-inline) > .x-link { align-self:end; justify-self:end; width:auto; white-space:nowrap; }
</style>
