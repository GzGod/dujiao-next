<template>
  <article class="xg-card" :class="{ 'xg-card--disabled': soldOut }" @click="open">
    <div class="xg-card__head">
      <span class="xg-card__logo">
        <img v-if="logo && !logoFailed" :src="logo" :alt="title" loading="lazy" @error="logoFailed = true" />
        <span v-else aria-hidden="true">{{ title.slice(0, 1) }}</span>
      </span>
      <XgStockTag :status="product.stock_status" />
    </div>

    <h3 class="xg-card__title">{{ title }}</h3>
    <p class="xg-card__desc">{{ desc }}</p>

    <div class="xg-card__foot">
      <div class="xg-price">
        <span class="xg-price__symbol">{{ symbol }}</span>
        <span class="xg-price__amount">{{ priceText }}</span>
        <span v-if="originalText" class="xg-price__original">{{ symbol }}{{ originalText }}</span>
      </div>
      <button type="button" class="xg-btn xg-btn--primary xg-btn--sm" :disabled="soldOut" @click.stop="buy">
        {{ soldOut ? t('xg.stock.out') : t('xg.card.buy') }}
      </button>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { getFirstImageUrl } from '../../utils/image'
import { brandLogoUrl, detectAiBrand } from '../../utils/aiBrand'
import { amountToCents, centsToAmount } from '../../utils/money'
import { useLocalized } from '../../composables/useProduct'
import XgStockTag from './XgStockTag.vue'

const props = defineProps<{ product: any }>()
const emit = defineEmits<{ open: [slug: string]; buy: [product: any] }>()

const { t } = useI18n()
const { getLocalizedText, siteCurrency } = useLocalized()
const logoFailed = ref(false)

const title = computed(() => getLocalizedText(props.product?.title) || '')
// 图标统一用 AI 品牌 logo（GPT → OpenAI，Grok → Grok），识别不出品牌时才用商品自带图片
const logo = computed(() => brandLogoUrl(detectAiBrand(props.product)) || getFirstImageUrl(props.product?.images))
const soldOut = computed(() => Boolean(props.product?.is_sold_out || props.product?.stock_status === 'out_of_stock'))
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
const priceText = computed(() => fmt(hasPromotion.value ? props.product?.promotion_price_amount : props.product?.price_amount))
const originalText = computed(() => (hasPromotion.value ? fmt(props.product?.price_amount) : ''))

// 交付说明：商品自带简介优先；没有简介时用统一的交付说明。
// 上游对接商品在接口里统一标成 manual，但实际是付款后由系统向上游采购自动发码，所以不按该字段区分。
const desc = computed(() => getLocalizedText(props.product?.description) || t('xg.card.auto'))

const open = () => emit('open', props.product?.slug)
const buy = () => {
  if (!soldOut.value) emit('buy', props.product)
}
</script>
