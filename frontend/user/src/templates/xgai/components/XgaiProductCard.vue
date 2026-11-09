<template>
  <article class="xpc" :class="{ 'xpc--featured': featured, 'xpc--disabled': soldOut }" @click="emit('open', product.slug)">
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
    <p class="xpc__note">{{ note }}</p>

    <div class="xpc__price">
      <strong><i>{{ symbol }}</i>{{ priceText }}</strong>
      <span v-if="originalText" class="xpc__was">{{ symbol }}{{ originalText }}</span>
      <del v-if="official">{{ t('xgai.card.official', { cny: official.officialCny }) }}</del>
    </div>
    <p v-if="official" class="xpc__save">{{ t('xgai.card.save', { amount: official.savingCny }) }}</p>
    <p v-else class="xpc__save xpc__save--muted">{{ stockText }}</p>

    <button type="button" class="xpc__cta" :disabled="soldOut" @click.stop="emit('buy', product)">
      {{ soldOut ? t('xg.stock.out') : t('xgai.card.choose') }} <ArrowRight />
    </button>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowRight } from 'lucide-vue-next'
import { useLocalized } from '../../../composables/useProduct'
import { getFirstImageUrl } from '../../../utils/image'
import { amountToCents, centsToAmount } from '../../../utils/money'
import { brandLogoUrl, detectAiBrand } from '../../../utils/aiBrand'
import { compareWithOfficial } from '../../../utils/officialPrice'

const props = defineProps<{ product: any; featured?: boolean }>()
const emit = defineEmits<{ open: [slug: string]; buy: [product: any] }>()

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
  min-height: 300px;
  padding: 24px;
  border: 1px solid var(--x-line);
  border-radius: var(--x-r-lg);
  background: rgba(255, 255, 255, 0.88);
  box-shadow: var(--x-shadow-sm);
  cursor: pointer;
  transition: transform 220ms var(--x-ease), box-shadow 220ms var(--x-ease), border-color 220ms var(--x-ease);
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
  overflow: hidden;
  color: var(--x-ink);
  font-size: 17px;
  font-weight: 650;
  letter-spacing: -0.02em;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.xpc__note {
  margin: 16px 0 0;
  min-height: 44px;
  color: var(--x-ink-3);
  font-size: 14px;
  line-height: 1.6;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

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
