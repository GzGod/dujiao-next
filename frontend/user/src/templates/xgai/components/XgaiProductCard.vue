<template>
  <article ref="card" class="xpc" :class="{ 'xpc--featured': featured, 'xpc--disabled': soldOut, 'xpc--expanded': expanded }" tabindex="0" :aria-label="title" @mouseenter="schedulePreview" @mouseleave="leaveCard" @focusout="cancelPreview" @keydown.enter.self="openPreview" @click="onCardClick">
    <span v-if="featured" class="xpc__badge">{{ t('xgai.card.popular') }}</span>
    <div class="xpc__brand">
      <span class="xpc__icon" :class="`xpc__icon--${brand}`">
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

  <dialog ref="preview" class="xpc-preview" :aria-label="title" @cancel.prevent="closePreview" @click="onBackdrop">
    <section class="xpc-preview__layout">
      <button class="xpc-paper__close" type="button" :aria-label="t('announcement.close')" @click="closePreview"><X /></button>
      <div class="xpc__brand" data-morph="brand">
        <span class="xpc__icon" :class="`xpc__icon--${brand}`"><img v-if="logo" :src="logo" alt="" /><span v-else>{{ title.slice(0, 1) }}</span></span>
        <span class="xpc__name"><small>{{ brandLabel }}</small><b>{{ title }}</b></span>
      </div>
      <div class="xpc-preview__description">
        <p class="xpc-paper__note">{{ note }}</p>
        <div v-if="richContent" class="xpc-paper__content" v-html="richContent" />
      </div>
      <div class="xpc__price" data-morph="price"><strong><i>{{ symbol }}</i>{{ priceText }}</strong><span v-if="originalText" class="xpc__was">{{ symbol }}{{ originalText }}</span><del v-if="official">{{ t('xgai.card.official', { cny: official.officialCny }) }}</del></div>
      <p class="xpc__save" :class="{ 'xpc__save--muted': !official }" data-morph="save">{{ official ? t('xgai.card.save', { amount: official.savingCny }) : stockText }}</p>
      <button class="xpc__cta" data-morph="cta" :disabled="soldOut" @click="buy">{{ soldOut ? t('xg.stock.out') : t('xgai.card.choose') }} <ArrowRight /></button>
    </section>
  </dialog>

</template>

<script setup lang="ts">
import { computed, ref, nextTick, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowRight, X } from 'lucide-vue-next'
import { useLocalized } from '../../../composables/useProduct'
import { getFirstImageUrl } from '../../../utils/image'
import { amountToCents, centsToAmount } from '../../../utils/money'
import { brandLogoUrl, detectAiBrand } from '../../../utils/aiBrand'
import { compareWithOfficial } from '../../../utils/officialPrice'
import { productAPI } from '../../../api/product'
import { processHtmlForDisplay } from '../../../utils/content'

const props = defineProps<{ product: any; featured?: boolean }>()
const emit = defineEmits<{ open: [slug: string]; buy: [product: any] }>()

const expanded = ref(false)
const detail = ref<any>(null)
const richContent = computed(() => processHtmlForDisplay(getLocalizedText(detail.value?.content || props.product?.content) || ''))
let detailRequested = false
const preview = ref<HTMLDialogElement | null>(null)
const card = ref<HTMLElement | null>(null)
let previewTimer: ReturnType<typeof setTimeout> | undefined
const cancelPreview = () => { clearTimeout(previewTimer); previewTimer = undefined }
let morph: Animation | undefined
let closing = false
let hoverReady = true
const leaveCard = () => { cancelPreview(); hoverReady = true }
const reduceMotion = () => window.matchMedia('(prefers-reduced-motion: reduce)').matches
const morphParts = ['brand', 'price', 'save', 'cta'] as const
const sourceSelector = { brand: '.xpc__brand', price: '.xpc__price', save: '.xpc__save', cta: '.xpc__cta' }
const openPreview = () => {
  cancelPreview()
  const panel = preview.value
  const source = card.value
  if (!panel || !source || document.querySelector('dialog[open], [aria-modal="true"]')) return
  const start = source.getBoundingClientRect()
  const style = getComputedStyle(source)
  panel.style.setProperty('--card-padding', style.paddingTop)
  panel.style.setProperty('--card-radius', style.borderRadius)
  panel.style.setProperty('--brand-width', `${start.width - parseFloat(style.paddingLeft) * 2 - 2}px`)
  panel.showModal()
  const end = panel.getBoundingClientRect()
  panel.style.setProperty('--layout-width', `${end.width - 2}px`)
  panel.style.setProperty('--layout-height', `${end.height - 2}px`)
  const targets = new Map(morphParts.map(part => [part, panel.querySelector(`[data-morph="${part}"]`)!.getBoundingClientRect()]))
  const duration = reduceMotion() ? 0 : 680
  const timing = { duration, easing: 'cubic-bezier(.22, 1, .36, 1)', fill: 'both' as FillMode }
  // Animate the actual card bounds: no rotation, image scaling or shape substitution.
  morph = panel.animate([
    { left: `${start.left}px`, top: `${start.top}px`, width: `${start.width}px`, height: `${start.height}px`, borderRadius: style.borderRadius },
    { left: `${end.left}px`, top: `${end.top}px`, width: `${end.width}px`, height: `${end.height}px`, borderRadius: style.borderRadius },
  ], timing)
  morphParts.forEach(part => {
    const from = source.querySelector(sourceSelector[part])?.getBoundingClientRect()
    const node = panel.querySelector<HTMLElement>(`[data-morph="${part}"]`)
    if (!from || !node) return
    const to = targets.get(part)!
    node.animate([
      { width: `${from.width}px`, transform: `translate(${from.left - start.left - (to.left - end.left)}px, ${from.top - start.top - (to.top - end.top)}px)` },
      { width: `${to.width}px`, transform: 'translate(0, 0)' },
    ], timing)
  })
  expanded.value = true
  const opening = morph
  opening.onfinish = () => { opening.cancel(); if (morph === opening) morph = undefined }
  if (!detailRequested) {
    detailRequested = true
    productAPI.detail(props.product.slug).then(response => { detail.value = response.data.data }).catch(() => { detailRequested = false })
  }
}
const schedulePreview = () => {
  cancelPreview()
  if (hoverReady && !closing && !expanded.value && window.matchMedia('(hover: hover)').matches) previewTimer = setTimeout(openPreview, 1000)
}
const closePreview = (after?: unknown) => {
  cancelPreview()
  const panel = preview.value
  const source = card.value
  if (!panel?.open || !source || closing) { if (!closing && typeof after === 'function') after(); return }
  closing = true
  hoverReady = false
  const start = panel.getBoundingClientRect()
  const end = source.getBoundingClientRect()
  const partStarts = new Map(morphParts.map(part => [part, panel.querySelector(`[data-morph="${part}"]`)!.getBoundingClientRect()]))
  const panelShadow = getComputedStyle(panel).boxShadow
  const cardShadow = getComputedStyle(source).boxShadow
  if (morph) { morph.onfinish = null; morph.cancel() }
  const duration = reduceMotion() ? 0 : 480
  panel.classList.add('xpc-preview--closing')
  morph = panel.animate([
    { left: `${start.left}px`, top: `${start.top}px`, width: `${start.width}px`, height: `${start.height}px`, boxShadow: panelShadow },
    { left: `${end.left}px`, top: `${end.top}px`, width: `${end.width}px`, height: `${end.height}px`, boxShadow: cardShadow },
  ], { duration, easing: 'cubic-bezier(.22, 1, .36, 1)', fill: 'forwards' })
  morphParts.forEach(part => {
    const node = panel.querySelector<HTMLElement>(`[data-morph="${part}"]`)
    const to = source.querySelector(sourceSelector[part])?.getBoundingClientRect()
    if (!node || !to) return
    const from = partStarts.get(part)!
    node.animate([{ width: `${from.width}px`, transform: 'translate(0,0)' }, { width: `${to.width}px`, transform: `translate(${to.left - end.left - (from.left - start.left)}px, ${to.top - end.top - (from.top - start.top)}px)` }], { duration, easing: 'cubic-bezier(.22, 1, .36, 1)', fill: 'forwards' })
  })
  morph.onfinish = async () => {
    // Restore the real card before removing its animated copy to avoid an empty frame.
    expanded.value = false
    await nextTick()
    panel.close()
    panel.getAnimations({ subtree: true }).forEach(animation => animation.cancel())
    panel.classList.remove('xpc-preview--closing')
    expanded.value = false
    closing = false
    morph = undefined
    if (typeof after === 'function') after()
  }
}
const onBackdrop = (event: MouseEvent) => { if (event.target === preview.value) closePreview() }
const onCardClick = () => openPreview()
const buy = () => closePreview(() => emit('buy', props.product))
onBeforeUnmount(() => { cancelPreview(); morph?.cancel(); preview.value?.close() })
const { t } = useI18n()
const { getLocalizedText, siteCurrency } = useLocalized()

const title = computed(() => getLocalizedText(props.product?.title) || '')
const brand = computed(() => detectAiBrand(props.product))
const brandLabel = computed(() => (brand.value === 'openai' ? 'OPENAI' : brand.value === 'grok' ? 'xAI' : getLocalizedText(props.product?.category?.name) || ''))
const logo = computed(() => brandLogoUrl(brand.value) || getFirstImageUrl(props.product?.images))
// 只用商品自带的简介；没有就用通用的交付说明，不编卖点
const note = computed(() => getLocalizedText(props.product?.description) || t('xg.card.auto'))
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
const paidAmount = computed(() => (hasPromotion.value ? props.product?.promotion_price_amount : props.product?.price_amount))
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

.xpc--expanded { visibility: hidden; }
.xpc-preview { position: fixed; inset: auto; left: max(16px, calc(50vw - 310px)); top: max(24px, calc(50dvh - 240px)); width: min(620px, calc(100vw - 32px)); height: min(480px, calc(100dvh - 48px)); max-width: none; max-height: none; box-sizing: border-box; margin: 0; padding: 0; overflow: hidden; border: 1px solid var(--x-line); border-radius: var(--card-radius); color: var(--x-ink); background: var(--x-surface); box-shadow: 0 32px 120px #0003; }
.xpc-preview::backdrop { background: #1116; animation: xpc-backdrop .68s ease both; }
.xpc-preview__layout { position: relative; display: flex; flex-direction: column; width: var(--layout-width); height: var(--layout-height); padding: var(--card-padding); box-sizing: border-box; }
.xpc-preview .xpc__brand { width: var(--brand-width); flex: 0 0 auto; min-height: 96px; align-items: flex-start; }
.xpc-preview .xpc__name b { font-size: 16px; }
.xpc-preview .xpc__price { flex: 0 0 auto; }
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
