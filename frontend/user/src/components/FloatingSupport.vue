<template>
  <aside ref="floater" class="floating-support" :class="{ 'floating-support--dragging': dragging, 'floating-support--left': position && position.x < 320, 'floating-support--top': position && position.y < 300 }" :style="position ? { left: position.x + 'px', top: position.y + 'px', right: 'auto', bottom: 'auto' } : undefined" :aria-label="t('floatingSupport.title')">
    <a v-if="supportUrl && pet?.mode !== 'hidden'" :href="supportUrl" target="_blank" rel="noopener noreferrer" class="support-pet" @pointerdown="startDrag" @pointermove="moveDrag" @pointerup="endDrag" @pointercancel="cancelDrag" @click="petClick" @dragstart.prevent @keydown="moveWithKeyboard" :title="t('floatingSupport.dragHint')" :aria-label="t('floatingSupport.label')">
      <SupportPetVisual :pet="pet" />
      <span class="support-pet__label">{{ t('floatingSupport.title') }} ↗</span>
    </a>
    <a v-if="supportUrl && pet?.mode !== 'hidden'" class="support-credit" :href="petCreditUrl" target="_blank" rel="noopener noreferrer">{{petName}} · Petdex</a>
  </aside>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import SupportPetVisual from './SupportPetVisual.vue'
import { useAppStore } from '../stores/app'

const { t } = useI18n()
const appStore = useAppStore()
const pet = computed(() => appStore.isResellerTenant ? appStore.config?.reseller_theme?.support_pet : undefined)
const petName = computed(() => !pet.value || pet.value.mode === 'default' ? 'Jiyi' : pet.value.name || 'Petdex')
const petCreditUrl = computed(() => !pet.value || pet.value.mode === 'default' ? 'https://petdex.dev/pets/jiyi' : /^https:\/\/petdex\.dev\/pets\/[a-z0-9-]+\/?$/i.test(pet.value.source_url || '') ? pet.value.source_url : 'https://petdex.dev/')
const supportUrl = computed(() => {
  const configured = String(appStore.config?.contact?.telegram || '').trim()
  if (/^https:\/\/(?:t\.me|telegram\.me)\//i.test(configured)) return configured
  return appStore.isResellerTenant ? '' : 'https://t.me/xuegaozhanshen'
})
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
onMounted(() => {
  try { localStorage.removeItem('xuegaoai_pending_orders_v1') } catch { /* Storage unavailable. */ }
  restorePosition()
  window.addEventListener('resize', restorePosition)
})
onBeforeUnmount(() => window.removeEventListener('resize', restorePosition))
</script>

<style scoped>
.floating-support { position: fixed; right: max(18px, env(safe-area-inset-right)); bottom: max(30px, env(safe-area-inset-bottom)); z-index: 45; display: flex; flex-direction: column; align-items: center; gap: 5px; }
.support-pet { touch-action: none; user-select: none; cursor: grab; display: flex; align-items: center; flex-direction: column; color: var(--ui-text-primary, #20232b); text-decoration: none; transition: transform .25s; }
.floating-support--dragging .support-pet { cursor: grabbing; transform: none; }
.floating-support--left .support-note { right: auto; left: 0; }
.floating-support--top .support-note { bottom: auto; top: calc(100% + 12px); }
.support-pet:hover { transform: translateY(-4px); }
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
@media(max-width:640px) { .floating-support { right: 12px; bottom: calc(85px + env(safe-area-inset-bottom)); } }
@media(prefers-reduced-motion:reduce) { .support-pet, .support-note-enter-active, .support-note-leave-active { transition: none; } }
</style>
