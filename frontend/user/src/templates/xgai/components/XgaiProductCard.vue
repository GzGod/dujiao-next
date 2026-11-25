<template>
  <article ref="card" class="xpc" :class="{ 'xpc--featured': featured, 'xpc--disabled': soldOut }" tabindex="0" :aria-label="title" @pointermove="moveLight" @pointerleave="glassActive=false; resetLight()" @mouseenter="glassActive=true; schedulePreview()" @mouseleave="leaveCard" @focusin="glassActive=true" @focusout="leaveFocus" @keydown.enter.self="openPreview" @click="onCardClick">
    <Transition name="glass-light"><GlassSurface v-if="glassActive" :radius="20" :intensity=".42" /></Transition>
    <span v-if="featured" class="xpc__badge">{{ t('xgai.card.popular') }}</span>
    <div class="xpc__brand">
      <span class="xpc__icon" :class="[`xpc__icon--${brand}`, { 'xpc__icon--artwork': productArtworkUrl(product) }]">
        <img v-if="logo" :src="logo" alt="" />
        <span v-else>{{ title.slice(0, 1) }}</span>
      </span>
      <span class="xpc__name">
        <small>{{ brandLabel }}</small>
        <b>{{ title }}</b>
      </span>
    </div>


    <div class="xpc__price">
      <strong><i>{{ symbol }}</i>{{ priceText }}</strong>
      <span v-if="originalText" class="xpc__was">{{ symbol }}{{ originalText }}</span>
      <del v-if="official">{{ t('xgai.card.official', { cny: official.officialCny }) }}</del>
    </div>
    <p v-if="official" class="xpc__save">{{ t('xgai.card.save', { amount: official.savingCny }) }}</p>
    <p v-else class="xpc__save xpc__save--muted">{{ stockText }}</p>

    <button type="button" class="xpc__cta" :disabled="soldOut" @click.stop="buy">
      {{ soldOut ? t('xg.stock.out') : t('xgai.card.choose') }} <ArrowRight />
    </button>
  </article>

</template>

<script setup lang="ts">
import { computed, ref, onBeforeUnmount } from 'vue'
import '../styles/glass-commerce.css'
import GlassSurface from './GlassSurface.vue'
import { useI18n } from 'vue-i18n'
import { ArrowRight } from 'lucide-vue-next'
import { useLocalized } from '../../../composables/useProduct'
import { getFirstImageUrl } from '../../../utils/image'
import { amountToCents, centsToAmount } from '../../../utils/money'
import { brandLogoUrl, detectAiBrand, productArtworkUrl } from '../../../utils/aiBrand'
import { compareWithOfficial } from '../../../utils/officialPrice'

const props = defineProps<{ product: any; featured?: boolean }>()
const emit = defineEmits<{ open: [slug: string]; buy: [product: any, source?: HTMLElement] }>()

const card = ref<HTMLElement | null>(null)
const glassActive=ref(false)
const leaveFocus=(event:FocusEvent)=>{
 if(card.value?.contains(event.relatedTarget as Node))return
 glassActive.value=false;cancelPreview()
}
let lightFrame=0
const moveLight=(event:PointerEvent)=>{
 if(event.pointerType==='touch'||lightFrame)return
 const x=event.clientX,y=event.clientY
 lightFrame=requestAnimationFrame(()=>{lightFrame=0;const el=card.value;if(!el)return;const r=el.getBoundingClientRect();el.style.setProperty('--glass-x',`${x-r.left}px`);el.style.setProperty('--glass-y',`${y-r.top}px`)})
}
const resetLight=()=>{cancelAnimationFrame(lightFrame);lightFrame=0;card.value?.style.removeProperty('--glass-x');card.value?.style.removeProperty('--glass-y')}
onBeforeUnmount(()=>cancelAnimationFrame(lightFrame))
let previewTimer: ReturnType<typeof setTimeout> | undefined
let hoverReady = true
const cancelPreview = () => { clearTimeout(previewTimer); previewTimer = undefined }
const leaveCard = () => { cancelPreview(); hoverReady = true; if (card.value) delete card.value.dataset.revealCooldown }
const openPreview = () => {
  cancelPreview()
  if (!hoverReady || document.querySelector('dialog[open], [aria-modal="true"]')) return
  hoverReady = false
  emit('buy', props.product, card.value || undefined)
}
const schedulePreview = () => {
  cancelPreview()
  if (hoverReady && !card.value?.dataset.revealCooldown && window.matchMedia('(hover: hover)').matches) previewTimer = setTimeout(openPreview, 1000)
}
const onCardClick = () => { hoverReady = true; openPreview() }
const buy = () => { hoverReady = true; openPreview() }
onBeforeUnmount(cancelPreview)
const { t } = useI18n()
const { getLocalizedText, siteCurrency } = useLocalized()

const title = computed(() => getLocalizedText(props.product?.title) || '')
const brand = computed(() => detectAiBrand(props.product))
const brandLabel = computed(() => (brand.value === 'openai' ? 'OPENAI' : brand.value === 'grok' ? 'xAI' : getLocalizedText(props.product?.category?.name) || ''))
const logo = computed(() => productArtworkUrl(props.product) || brandLogoUrl(brand.value) || getFirstImageUrl(props.product?.images))
// 只用商品自带的简介；没有就用通用的交付说明，不编卖点
const soldOut = computed(() => Boolean(props.product?.is_sold_out || props.product?.stock_status === 'out_of_stock'))
const stockText = computed(() => (soldOut.value ? t('xg.stock.out') : t('xg.stock.in')))
const symbol = computed(() => (String(siteCurrency.value || 'CNY').toUpperCase() === 'CNY' ? '¥' : `${siteCurrency.value} `))

const fmt = (amount: any) => {
  const cents = amountToCents(amount)
  if (cents === null) return ''
  const text = centsToAmount(cents)
  return text.endsWith('.00') ? text.slice(0, -3) : text
}
const hasPromotion = computed(() => {
  const promo = amountToCents(props.product?.promotion_price_amount)
  const base = amountToCents(props.product?.price_amount)
  return promo !== null && base !== null && promo < base
})
const paidAmount = computed(() => {
  const skus = (props.product?.skus || []).filter((sku: any) => sku.is_active)
  if (skus.length) {
    const amounts = skus.map((sku: any) => {
      const candidates = [sku.price_amount, sku.member_price_amount, sku.promotion_price_amount]
        .map(amountToCents).filter((value: number | null): value is number => value !== null && value > 0)
      return candidates.length ? Math.min(...candidates) : null
    }).filter((value: number | null): value is number => value !== null)
    if (amounts.length) return centsToAmount(Math.min(...amounts))
  }
  return hasPromotion.value ? props.product?.promotion_price_amount : props.product?.price_amount
})
const priceText = computed(() => fmt(paidAmount.value))
const originalText = computed(() => (hasPromotion.value ? fmt(props.product?.price_amount) : ''))
// 官方价对比：只在确实更便宜时出现（规则见 utils/officialPrice）
const official = computed(() => (symbol.value === '¥' ? compareWithOfficial(title.value, Number(paidAmount.value)) : null))
</script>

<style scoped>
.xpc {
  position: relative;
  display: flex;
  flex-direction: column;
  height: 340px;
  min-height: 340px;
  padding: 24px;
  border: 1px solid var(--x-line);
  border-radius: var(--x-r-lg);
  background: rgba(255, 255, 255, 0.88);
  box-shadow: var(--x-shadow-sm);
  cursor: pointer;
  transition: transform 320ms var(--x-ease), box-shadow 320ms var(--x-ease), border-color 320ms var(--x-ease);
  align-self: start;
}
.xpc:hover {
  border-color: rgba(0, 113, 227, 0.22);
  box-shadow: var(--x-shadow);
  transform: translateY(-4px);
}
.xpc--featured {
  border-color: rgba(0, 113, 227, 0.24);
  background: linear-gradient(180deg, rgba(246, 251, 255, 0.98), rgba(255, 255, 255, 0.95));
  box-shadow: 0 10px 34px rgba(0, 113, 227, 0.07), inset 0 0 0 1px rgba(0, 113, 227, 0.04);
}
.xpc--disabled { cursor: default; opacity: 0.7; }
.xpc--disabled:hover { transform: none; box-shadow: var(--x-shadow-sm); }

.xpc__badge {
  position: absolute;
  top: 16px;
  right: 16px;
  padding: 3px 10px;
  border-radius: var(--x-r-pill);
  color: #fff;
  background: var(--x-blue);
  font-size: 12px;
  font-weight: 600;
}

.xpc__brand {
  display: flex;
  align-items: center;
  gap: 12px;
}
.xpc__icon {
  width: 44px;
  height: 44px;
  display: grid;
  flex-shrink: 0;
  place-items: center;
  overflow: hidden;
  border: 1px solid var(--x-line);
  border-radius: 12px;
  background: #fff;
  color: var(--x-ink);
  font-weight: 700;
}
.xpc__icon img { width: 28px; height: 28px; object-fit: contain; }
.xpc__icon--artwork img { width: 100%; height: 100%; object-fit: cover; }
.xpc__icon--grok { background: #111; }
.xpc__icon--grok img { filter: invert(1); }
.xpc__name {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 2px;
}
.xpc__name small {
  color: var(--x-ink-4);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.08em;
}
.xpc__name b {
  color: var(--x-ink);
  font-size: 17px;
  font-weight: 650;
  letter-spacing: -0.02em;
  white-space: normal;
  overflow-wrap: anywhere;
  line-height: 1.5;
}

.xpc:focus-visible { border-color: var(--x-blue); outline: none; }

.xpc__price { flex: 0 0 auto; }
.xpc-preview .xpc__cta { flex: 0 0 auto; margin-top: 20px; }
.xpc-preview__description { flex: 1; min-height: 0; overflow-y: auto; margin-top: 10px; animation: xpc-content .45s .2s ease both; }
.xpc-paper__note { white-space: pre-wrap; overflow-wrap: anywhere; font-size: 15px; line-height: 1.85; color: var(--x-ink-2); }
.xpc-paper__content { color: var(--x-ink-2); font-size: 14px; line-height: 1.8; overflow-wrap: anywhere; }
.xpc-paper__content :deep(img) { max-width: 100%; height: auto; }
.xpc-paper__close { position: absolute; z-index: 2; top: 14px; right: 14px; display: grid; place-items: center; width: 32px; height: 32px; border: 1px solid var(--x-line); border-radius: 50%; background: var(--x-surface); color: inherit; cursor: pointer; animation: xpc-content .35s .3s ease both; }
.xpc-paper__close svg { width: 17px; height: 17px; }
.xpc-preview--closing .xpc-preview__description, .xpc-preview--closing .xpc-paper__close { animation: none; opacity: 0; transition: opacity .12s; }
@keyframes xpc-content { from { opacity: 0; } to { opacity: 1; } }
.xpc-preview--closing::backdrop { animation: xpc-backdrop-close .48s cubic-bezier(.22, 1, .36, 1) both; }
@keyframes xpc-backdrop { from { opacity: 0; } to { opacity: 1; } }
@keyframes xpc-backdrop-close { from { opacity: 1; } to { opacity: 0; } }
.xpc:focus-visible { border-color: var(--x-blue); outline: none; }
@media (prefers-reduced-motion: reduce) { .xpc, .xpc-preview::backdrop, .xpc-preview__description, .xpc-paper__close { animation: none; transition: none; } }

.xpc__price {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 4px 10px;
  margin-top: 18px;
  padding-top: 18px;
  border-top: 1px solid var(--x-line);
}
.xpc__price strong {
  color: var(--x-blue);
  font-size: 32px;
  font-weight: 700;
  line-height: 1;
  letter-spacing: -0.04em;
}
.xpc__price strong i {
  margin-right: 2px;
  font-size: 17px;
  font-style: normal;
  font-weight: 600;
}
.xpc__was {
  color: var(--x-ink-4);
  font-size: 14px;
  text-decoration: line-through;
}
.xpc__price del {
  color: var(--x-ink-4);
  font-size: 13px;
}
.xpc__save {
  align-self: flex-start;
  margin: 10px 0 0;
  padding: 2px 10px;
  border-radius: var(--x-r-pill);
  color: var(--x-green);
  background: var(--x-green-soft);
  font-size: 12px;
  font-weight: 600;
}
.xpc__save--muted {
  color: var(--x-ink-3);
  background: var(--x-bg);
}

.xpc__cta {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-height: 42px;
  margin-top: auto;
  padding: 0 18px;
  border: 1px solid var(--x-line-strong);
  border-radius: var(--x-r-pill);
  color: var(--x-ink-2);
  background: transparent;
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
  transition: background-color 180ms var(--x-ease), border-color 180ms var(--x-ease), color 180ms var(--x-ease);
}
.xpc__price + .xpc__save + .xpc__cta,
.xpc__save + .xpc__cta { margin-top: 20px; }
.xpc__cta svg { width: 15px; height: 15px; }
.xpc:hover .xpc__cta:not(:disabled),
.xpc--featured .xpc__cta:not(:disabled) {
  color: #fff;
  border-color: var(--x-blue);
  background: var(--x-blue);
}
.xpc__cta:disabled { cursor: not-allowed; }
</style>
