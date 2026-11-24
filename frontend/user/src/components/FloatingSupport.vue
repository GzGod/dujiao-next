<template>
  <aside ref="floater" class="floating-support" :class="{ 'floating-support--dragging': dragging, 'floating-support--left': position && position.x < 320, 'floating-support--top': position && position.y < 300 }" :style="position ? { left: position.x + 'px', top: position.y + 'px', right: 'auto', bottom: 'auto' } : undefined" :aria-label="t('floatingSupport.title')">
    <Transition name="support-note">
      <section v-if="open && pending.length" class="support-note" aria-live="polite">
        <div class="support-note__head"><strong>{{ t('pendingOrders.title') }}</strong><button type="button" @click="open = false" :aria-label="t('pendingOrders.dismiss')"><X :size="16" /></button></div>
        <p>{{ t('pendingOrders.hint', { count: pending.length }) }}</p>
        <div v-for="item in pending.slice(0, 3)" :key="item.orderNo" class="support-note__order">
          <small>{{ item.orderNo }}</small>
          <span>{{ t(verified.has(item.orderNo) ? 'pendingOrders.unpaid' : 'pendingOrders.unconfirmed') }}</span>
          <button type="button" class="support-note__resume" @click="resume(item)">{{ t(verified.has(item.orderNo) ? 'pendingOrders.resume' : 'pendingOrders.check') }} →</button>
        </div>
        <small class="support-note__privacy">{{ t('pendingOrders.authHint') }}</small>
      </section>
    </Transition>
    <button v-if="pending.length" class="support-pending" type="button" @click="open = !open" :aria-expanded="open">{{ t('pendingOrders.badge', { count: pending.length }) }}</button>
    <a v-if="supportUrl" :href="supportUrl" target="_blank" rel="noopener noreferrer" class="support-pet" @pointerdown="startDrag" @pointermove="moveDrag" @pointerup="endDrag" @pointercancel="cancelDrag" @click="petClick" @dragstart.prevent @keydown="moveWithKeyboard" :title="t('pendingOrders.dragHint')" :aria-label="t('floatingSupport.label')">
      <span class="support-pet__frame" aria-hidden="true"><span class="support-pet__sprite" /></span>
      <span class="support-pet__label">{{ t('floatingSupport.title') }} ↗</span>
    </a>
    <a class="support-credit" href="https://petdex.dev/pets/jiyi" target="_blank" rel="noopener noreferrer">Jiyi · Petdex</a>
  </aside>
  <ProductQuickBuy v-if="resuming" :key="resuming.orderNo" :product="resumeProduct" :resume-order="resuming" :visible="true" inline-checkout @update:visible="resuming = null" />
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount, watch, defineAsyncComponent } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
const ProductQuickBuy = defineAsyncComponent(() => import('./ProductQuickBuy.vue'))
import { X } from 'lucide-vue-next'
import { useAppStore } from '../stores/app'
import { useUserAuthStore } from '../stores/userAuth'
import { guestOrderAPI, userOrderAPI } from '../api/order'
import { loadGuestOrderAuth } from '../utils/guestOrderAuth'
import { readPendingOrders, reconcilePendingOrder, PENDING_ORDERS_EVENT, type PendingOrder } from '../utils/pendingOrders'

const { t } = useI18n()
const appStore = useAppStore()
const auth = useUserAuthStore()
const route = useRoute()
const rows = ref(readPendingOrders())
const verified = ref(new Set<string>())
const open = ref(true)
const pending = computed(() => rows.value.filter(row => auth.isAuthenticated ? !row.guest && row.owner === String(auth.user?.id || '') : row.guest))
const supportUrl = computed(() => {
  const configured = String(appStore.config?.contact?.telegram || '').trim()
  if (/^https:\/\/(?:t\.me|telegram\.me)\//i.test(configured)) return configured
  return appStore.isResellerTenant ? '' : 'https://t.me/xuegaozhanshen'
})
const resuming = ref<PendingOrder | null>(null)
const resumeProduct = computed(() => ({ title: { 'zh-CN': t('pendingOrders.restoreTitle'), 'zh-TW': t('pendingOrders.restoreTitle'), 'en-US': t('pendingOrders.restoreTitle') }, price_amount: '0', stock_status: 'in_stock' }))
const resume = (row: PendingOrder) => { resuming.value = row }
const floater = ref<HTMLElement | null>(null)
const position = ref<{ x: number; y: number } | null>(null)
const dragging = ref(false)
let drag: { id: number; x: number; y: number; left: number; top: number } | null = null
let suppressClick = false
const positionKey = 'xuegaoai_support_position_v1'
const clampPosition = (x: number, y: number) => ({
  x: Math.max(8, Math.min(x, window.innerWidth - (floater.value?.offsetWidth || 100) - 8)),
  y: Math.max(8, Math.min(y, window.innerHeight - (floater.value?.offsetHeight || 155) - 8)),
})
const restorePosition = () => {
  try {
    const value = JSON.parse(localStorage.getItem(positionKey) || 'null')
    if (value && Number.isFinite(value.x) && Number.isFinite(value.y)) position.value = clampPosition(value.x * window.innerWidth, value.y * window.innerHeight)
  } catch { /* Keep default placement. */ }
}
const startDrag = (event: PointerEvent) => {
  if (!event.isPrimary || event.button !== 0) return
  const rect = floater.value?.getBoundingClientRect()
  if (!rect) return
  suppressClick = false
  drag = { id: event.pointerId, x: event.clientX, y: event.clientY, left: rect.left, top: rect.top }
  ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
}
const moveDrag = (event: PointerEvent) => {
  if (!drag || event.pointerId !== drag.id) return
  const dx = event.clientX - drag.x, dy = event.clientY - drag.y
  if (!dragging.value && Math.hypot(dx, dy) < 6) return
  dragging.value = true; suppressClick = true
  event.preventDefault()
  position.value = clampPosition(drag.left + dx, drag.top + dy)
}
const savePosition = () => {
  if (!position.value) return
  try { localStorage.setItem(positionKey, JSON.stringify({ x: position.value.x / window.innerWidth, y: position.value.y / window.innerHeight })) } catch { /* Current position still works. */ }
}
const moveWithKeyboard = (event: KeyboardEvent) => {
  const deltas: Record<string, [number, number]> = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] }
  const delta = deltas[event.key], rect = floater.value?.getBoundingClientRect()
  if (!delta || !rect) return
  event.preventDefault()
  const step = event.shiftKey ? 30 : 10
  position.value = clampPosition(rect.left + delta[0] * step, rect.top + delta[1] * step)
  savePosition()
}
const endDrag = (event: PointerEvent) => {
  if (!drag || event.pointerId !== drag.id) return
  if (dragging.value && position.value) {
    savePosition()
  }
  drag = null; dragging.value = false
}
const cancelDrag = () => { drag = null; dragging.value = false }
const petClick = (event: MouseEvent) => { if (suppressClick) { event.preventDefault(); suppressClick = false } }
const sync = () => {
  const next = readPendingOrders()
  if (next.some(row => !rows.value.some(old => old.orderNo === row.orderNo))) open.value = true
  rows.value = next
}
let busy = false
let disposed = false
let timer: ReturnType<typeof setInterval> | undefined
const refresh = async () => {
  sync()
  if (busy || document.visibilityState === 'hidden') return
  busy = true
  try {
    const guestAuth = loadGuestOrderAuth()
    for (const row of pending.value.slice(0, 5)) {
      if (disposed) break
      if (row.guest && (!guestAuth.email || !guestAuth.order_password)) continue
      try {
        const response = row.guest
          ? await guestOrderAPI.detail(row.orderNo, guestAuth, { silentBusinessError: true })
          : await userOrderAPI.detail(row.orderNo, { silentBusinessError: true })
        if (disposed) break
        reconcilePendingOrder(row.orderNo, response.data.data)
        if (response.data.data?.status === 'pending_payment') verified.value = new Set([...verified.value, row.orderNo])
      } catch { /* Unauthenticated/offline references remain explicitly unconfirmed. */ }
    }
  } finally { busy = false; if (!disposed) sync() }
}
const onStorage = () => { sync(); void refresh() }
watch(() => [auth.isAuthenticated, auth.user?.id], () => { verified.value = new Set(); void refresh() })
watch(() => route.fullPath, () => { void refresh() })
onMounted(() => {
  restorePosition()
  window.addEventListener('resize', restorePosition)
  window.addEventListener(PENDING_ORDERS_EVENT, sync)
  window.addEventListener('storage', onStorage)
  window.addEventListener('focus', refresh)
  document.addEventListener('visibilitychange', refresh)
  timer = setInterval(refresh, 30000)
  void refresh()
})
onBeforeUnmount(() => {
  disposed = true
  window.removeEventListener('resize', restorePosition)
  clearInterval(timer)
  window.removeEventListener(PENDING_ORDERS_EVENT, sync)
  window.removeEventListener('storage', onStorage)
  window.removeEventListener('focus', refresh)
  document.removeEventListener('visibilitychange', refresh)
})
</script>

<style scoped>
.floating-support { position: fixed; right: max(18px, env(safe-area-inset-right)); bottom: max(30px, env(safe-area-inset-bottom)); z-index: 45; display: flex; flex-direction: column; align-items: center; gap: 5px; }
.support-pet { touch-action: none; user-select: none; cursor: grab; display: flex; align-items: center; flex-direction: column; color: var(--ui-text-primary, #20232b); text-decoration: none; transition: transform .25s; }
.floating-support--dragging .support-pet { cursor: grabbing; transform: none; }
.floating-support--left .support-note { right: auto; left: 0; }
.floating-support--top .support-note { bottom: auto; top: calc(100% + 12px); }
.support-pet:hover { transform: translateY(-4px); }
.support-pet__frame { display: block; width: 88px; height: 95px; overflow: hidden; filter: drop-shadow(0 5px 7px #0002); }
/* Jiyi spritesheet: https://petdex.dev/pets/jiyi; original 192×208 frames. */
.support-pet__sprite { display: block; width: 192px; height: 208px; transform: scale(.45833); transform-origin: top left; background: url('/images/support/jiyi.webp') 0 0 / 1536px auto no-repeat; image-rendering: pixelated; animation: jiyi-idle 1.1s steps(6) infinite; }
.support-pet:hover .support-pet__sprite { animation: jiyi-wave .7s steps(4) infinite; }
@keyframes jiyi-idle { to { background-position: -1152px 0; } }
@keyframes jiyi-wave { from { background-position: 0 -624px; } to { background-position: -768px -624px; } }
.support-pet__label, .support-pending { border: 1px solid var(--ui-border, #e2e5eb); border-radius: 30px; background: var(--ui-bg-elevated, #fff); color: var(--ui-text-primary, #20232b); box-shadow: 0 4px 20px #0001; font-size: 12px; padding: 6px 12px; font-weight: 600; }
.support-pending { cursor: pointer; color: var(--ui-accent, #1768d8); }
.support-credit { font-size: 9px; color: var(--ui-text-muted, #69707e); text-decoration: none; }
.support-note { position: absolute; right: 0; bottom: calc(100% + 12px); width: min(300px, calc(100vw - 32px)); padding: 17px; border: 1px solid var(--ui-border, #e2e5eb); border-radius: 18px; background: var(--ui-bg-elevated, #fff); color: var(--ui-text-primary, #20232b); box-shadow: 0 12px 40px #0002; max-height: 55dvh; overflow-y: auto; }
.support-note__head { display: flex; align-items: center; justify-content: space-between; gap: 10px; font-size: 15px; }
.support-note__head button { background: none; border: 0; padding: 6px; color: var(--ui-text-muted, #69707e); cursor: pointer; }
.support-note p, .support-note__privacy { font-size: 12px; color: var(--ui-text-muted, #69707e); line-height: 1.6; }
.support-note p { margin: 8px 0 12px; }
.support-note__order { display: grid; gap: 5px; border-top: 1px solid var(--ui-border, #e2e5eb); padding: 10px 0; font-size: 12px; }
.support-note__order small { overflow-wrap: anywhere; color: var(--ui-text-muted, #69707e); }
.support-note__resume { background: none; border: 0; padding: 0; text-align: left; cursor: pointer; color: var(--ui-accent, #1768d8); font-weight: 650; }
.support-note-enter-active, .support-note-leave-active { transition: opacity .22s, transform .22s; }
.support-note-enter-from, .support-note-leave-to { opacity: 0; transform: translateY(8px); }
.support-pet:focus-visible, .support-pending:focus-visible { outline: 2px solid var(--ui-accent, #1768d8); outline-offset: 4px; }
@media(max-width:640px) { .floating-support { right: 12px; bottom: calc(85px + env(safe-area-inset-bottom)); } .support-pet__frame { width: 68px; height: 74px; } .support-pet__sprite { transform: scale(.354); } }
@media(prefers-reduced-motion:reduce) { .support-pet__sprite { animation: none!important; } .support-pet, .support-note-enter-active, .support-note-leave-active { transition: none; } }
</style>
