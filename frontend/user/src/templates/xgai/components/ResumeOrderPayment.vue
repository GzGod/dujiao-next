<template>
  <div class="resume-payment">
    <form v-if="needsAuth" class="resume-payment__form" @submit.prevent="load">
      <h3>{{ t('pendingOrders.restoreTitle') }}</h3><p>{{ t('pendingOrders.authHint') }}</p>
      <input v-model="credentials.email" type="email" required autocomplete="email" :placeholder="t('checkout.guestEmailPlaceholder')" />
      <input v-model="credentials.order_password" type="password" required autocomplete="off" :placeholder="t('checkout.guestPasswordPlaceholder')" />
      <button class="x-btn x-btn--primary" :disabled="busy">{{ t('pendingOrders.check') }}</button>
    </form>
    <p v-else-if="busy">{{ t('common.loading') }}</p>
    <InlinePayment v-else-if="result && !chooseChannel" :result="result" :guest="reference.guest" :email="credentials.email" :password="credentials.order_password" :channels="channels" :initial-channel-id="selected" @back="chooseChannel = true" />
    <div v-else-if="order" class="resume-payment__form">
      <h3>{{ t('checkout.paymentMethod') }}</h3>
      <p>{{ t('pendingOrders.sameOrder') }}</p>
      <button v-for="channel in channels" :key="channel.id" type="button" class="x-btn x-btn--ghost" :aria-pressed="selected === channel.id" @click="selected = channel.id">{{ channel.name }}</button>
      <button type="button" class="x-btn x-btn--primary" :disabled="!selected || busy || order.status !== 'pending_payment'" @click="pay">{{ t('pendingOrders.resume') }}</button>
      <button v-if="result" type="button" class="x-link" @click="chooseChannel = false">{{ t('payment.inlineBack') }}</button>
    </div>
    <p v-if="error" role="alert" class="resume-payment__error">{{ error }}</p>
  </div>
</template>
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { guestOrderAPI, userOrderAPI, paymentAPI } from '../../../api/order'
import { useAppStore } from '../../../stores/app'
import { loadGuestOrderAuth, saveGuestOrderAuth } from '../../../utils/guestOrderAuth'
import { reconcilePendingOrder, type PendingOrder } from '../../../utils/pendingOrders'
import InlinePayment from './InlinePayment.vue'
const props = defineProps<{ reference: PendingOrder }>()
const emit = defineEmits<{ loaded: [order: any]; busy: [value: boolean] }>()
const { t } = useI18n()
const app = useAppStore()
const credentials = ref(loadGuestOrderAuth())
const needsAuth = ref(props.reference.guest && (!credentials.value.email || !credentials.value.order_password))
const busy = ref(false), error = ref(''), order = ref<any>(null), result = ref<any>(null)
const selected = ref<number | null>(null), chooseChannel = ref(false)
const eligibleChannels = ref<any[] | null>(null)
const channels = computed(() => eligibleChannels.value || app.config?.payment_channels || [])
const load = async () => {
  if (busy.value) return
  busy.value = true; emit('busy', true); error.value = ''
  try {
    const response = props.reference.guest
      ? await guestOrderAPI.detail(props.reference.orderNo, credentials.value, { silentBusinessError: true })
      : await userOrderAPI.detail(props.reference.orderNo, { silentBusinessError: true })
    order.value = response.data.data
    if (!order.value?.order_no) throw new Error(t('pendingOrders.restoreError'))
    if (props.reference.guest) saveGuestOrderAuth(credentials.value)
    needsAuth.value = false
    emit('loaded', order.value)
    reconcilePendingOrder(props.reference.orderNo, order.value)
    if (!props.reference.guest && order.value.status === 'pending_payment') {
      const response = await userOrderAPI.getPaymentChannels({ order_no: props.reference.orderNo, amount: order.value.total_amount })
      eligibleChannels.value = Array.isArray(response.data.data) ? response.data.data : []
    }
    let payment: any = null
    if (order.value.status === 'pending_payment') {
      try {
        const response = props.reference.guest
          ? await guestOrderAPI.latestPayment({ ...credentials.value, order_no: props.reference.orderNo })
          : await paymentAPI.latest({ order_no: props.reference.orderNo })
        payment = response.data.data
      } catch { /* Show a same-order channel choice when no usable payment is available. */ }
    }
    if (payment?.pay_url || payment?.qr_code || order.value.status !== 'pending_payment') {
      result.value = { order_no: props.reference.orderNo, order: order.value, payment: payment || {} }
      selected.value = payment?.channel_id || null
      chooseChannel.value = false
    } else chooseChannel.value = true
  } catch { error.value = t('pendingOrders.restoreError'); if (props.reference.guest) needsAuth.value = true }
  finally { busy.value = false; emit('busy', false) }
}
const pay = async () => {
  if (!selected.value || busy.value || order.value?.status !== 'pending_payment') return
  busy.value = true; emit('busy', true); error.value = ''
  try {
    const payload = { order_no: props.reference.orderNo, channel_id: selected.value }
    const response = props.reference.guest ? await guestOrderAPI.createPayment({ ...payload, ...credentials.value }) : await paymentAPI.create(payload)
    result.value = { ...response.data.data, order_no: props.reference.orderNo, order: order.value }
    chooseChannel.value = false
  } catch { error.value = t('pendingOrders.restoreError') }
  finally { busy.value = false; emit('busy', false) }
}
onMounted(() => { if (!needsAuth.value) void load() })
</script>
<style scoped>
.resume-payment { position: relative; min-height: 480px; height: 100%; padding: 12px; }
.resume-payment__form { display: grid; align-content: center; gap: 14px; height: 100%; }
.resume-payment__form input { width: 100%; border: 1px solid var(--ui-border); background: var(--ui-bg-elevated); padding: 10px 12px; border-radius: 12px; }
.resume-payment__form p { font-size: 13px; color: var(--ui-text-muted); }
.resume-payment__error { color: #dc4747; font-size: 13px; }
@media(max-width:640px) { .resume-payment { min-height: 440px; } }
</style>
