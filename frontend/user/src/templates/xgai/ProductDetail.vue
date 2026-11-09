<template>
  <div class="xpd">
    <!-- 加载 -->
    <div v-if="loading" class="x-container xpd__hero">
      <div class="xpd__visual xpd__skeleton" />
      <div class="xpd__info">
        <div class="xpd__sk-line" style="width: 40%" />
        <div class="xpd__sk-line xpd__sk-line--lg" style="width: 70%" />
        <div class="xpd__sk-line" style="width: 90%" />
        <div class="xpd__sk-line xpd__sk-line--lg" style="width: 45%" />
      </div>
    </div>

    <template v-else-if="product">
      <nav class="x-container xpd__crumb" :aria-label="t('xgai.detail.breadcrumb')">
        <RouterLink to="/">{{ t('nav.home') }}</RouterLink>
        <span>/</span>
        <RouterLink :to="backToListTo">{{ t('xgai.nav.products') }}</RouterLink>
        <template v-if="categoryName">
          <span>/</span>
          <span>{{ categoryName }}</span>
        </template>
        <span>/</span>
        <b>{{ title }}</b>
      </nav>

      <section class="x-container xpd__hero">
        <!-- 视觉区：有商品图就展示图，否则展示品牌 logo -->
        <div class="xpd__visual" :class="{ 'xpd__visual--photo': showPhoto }">
          <template v-if="showPhoto">
            <img :src="currentImage" :alt="title" class="xpd__photo" />
            <div v-if="images.length > 1" class="xpd__thumbs">
              <button
                v-for="(img, idx) in images"
                :key="idx"
                type="button"
                :class="{ 'is-active': img === currentImage }"
                :aria-label="`${idx + 1}`"
                @click="currentImage = img"
              >
                <img :src="img" alt="" loading="lazy" />
              </button>
            </div>
          </template>
          <template v-else>
            <span class="xpd__orb" aria-hidden="true" />
            <div class="xpd__glyph" :class="`xpd__glyph--${brand}`">
              <img v-if="brandLogo" :src="brandLogo" alt="" />
              <span v-else>{{ title.slice(0, 1) }}</span>
            </div>
            <div class="xpd__float xpd__float--one">
              <Check />
              <span>
                <b>{{ t('xgai.detail.floatAutoTitle') }}</b>
                <small>{{ t('xgai.detail.floatAutoText') }}</small>
              </span>
            </div>
            <div class="xpd__float xpd__float--two"><i /> {{ t('xgai.detail.floatPay') }}</div>
            <p class="xpd__visual-note">
              {{ t('xgai.detail.visualLead') }}<br />
              <b>{{ t('xgai.detail.visualTail') }}</b>
            </p>
          </template>
        </div>

        <!-- 信息与购买 -->
        <div class="xpd__info">
          <span class="xpd__brand">
            {{ brandLine }}
            <i :class="`is-${stockTone}`">{{ getStockStatusLabel(product) }}</i>
          </span>
          <h1>{{ title }}</h1>
          <p class="xpd__lede">{{ lede }}</p>

          <!-- 价格：沿用 classic 的优先级（批发 > 促销 > 会员 > 原价） -->
          <div class="xpd__price">
            <strong><i>{{ currencySymbol }}</i>{{ priceNumber(displayPrice) }}</strong>
            <span v-if="strikePrice !== null" class="xpd__was">{{ formatPrice(strikePrice, siteCurrency) }}</span>
            <del v-if="official">{{ t('xgai.card.official', { cny: official.officialCny }) }}</del>
            <em v-if="official">{{ t('xgai.detail.save', { amount: official.savingCny }) }}</em>
          </div>
          <p v-if="priceTag" class="xpd__price-tag">{{ priceTag }}</p>

          <!-- 批发规则 / 活动规则 -->
          <div v-if="selectedSkuWholesaleRules.length" class="xpd__rules">
            <b>{{ t('products.wholesaleRulesTitle') }}</b>
            <span v-for="tier in selectedSkuWholesaleRules" :key="`${tier.sku_id || tier.sku_code || 'all'}-${tier.min_quantity}`">{{ formatWholesaleTier(tier) }}</span>
          </div>
          <div v-if="hasPromotionRules(product)" class="xpd__rules">
            <b>{{ t('products.promotionRulesTitle') }}</b>
            <span v-for="rule in getPromotionRules(product)" :key="rule.id">{{ formatPromotionRule(rule) }}</span>
          </div>

          <!-- 规格：只有多个规格时才显示 -->
          <div v-if="activeSkus.length > 1" class="xpd__choice">
            <label>{{ t('productDetail.skuTitle') }}</label>
            <div>
              <button
                v-for="sku in activeSkus"
                :key="sku.id"
                type="button"
                :class="{ 'is-active': normalizeSkuId(sku.id) === selectedSkuId }"
                :disabled="!isSkuPurchasable(sku)"
                @click="selectedSkuId = normalizeSkuId(sku.id)"
              >
                <span>
                  <b>{{ skuDisplayText(sku) }}</b>
                  <small>{{ skuStockText(sku) }}</small>
                </span>
              </button>
            </div>
            <p v-if="requiresSKUSelection" class="xpd__hint">{{ t('productDetail.skuRequired') }}</p>
          </div>

          <!-- 数量：只有允许买多个时才显示 -->
          <div v-if="showQuantity" class="xpd__qty">
            <label>{{ t('productDetail.quantity') }}</label>
            <div>
              <button type="button" :aria-label="t('xgai.detail.decrease')" :disabled="quantity <= quantityEffectiveMin" @click="quantity = Math.max(quantityEffectiveMin, quantity - 1)"><Minus /></button>
              <input inputmode="numeric" :value="quantity" :aria-label="t('productDetail.quantity')" @change="handleQuantityInput($event)" @keydown.enter.prevent="($event.target as HTMLInputElement)?.blur()" />
              <button type="button" :aria-label="t('xgai.detail.increase')" :disabled="quantityEffectiveLimit !== null && quantity >= quantityEffectiveLimit" @click="quantity = quantity + 1"><Plus /></button>
            </div>
          </div>

          <ul class="xpd__features">
            <li v-for="item in features" :key="item"><Check /> {{ item }}</li>
          </ul>

          <p v-if="cannotPurchaseReason" class="xpd__alert">{{ cannotPurchaseReason }}</p>
          <p v-if="purchaseWarning" class="xpd__alert xpd__alert--warn">{{ purchaseWarning }}</p>

          <div ref="purchaseActionsRef" class="xpd__actions">
            <button v-if="requiresLogin" type="button" class="x-btn x-btn--primary x-btn--lg x-btn--block" @click="goLogin">
              {{ t('productDetail.loginToBuy') }} <ArrowRight />
            </button>
            <template v-else>
              <button type="button" class="x-btn x-btn--primary x-btn--lg xpd__buy" :disabled="!canPurchase" @click="buyNow">
                {{ t('xgai.detail.buyNow') }} · {{ formatPrice(lineTotal, siteCurrency) }} <ArrowRight />
              </button>
              <button type="button" class="x-btn x-btn--ghost x-btn--lg" :disabled="!canPurchase" :aria-label="t('productDetail.addToCart')" @click="addToCart">
                <ShoppingBag /> <span class="xpd__cart-text">{{ t('productDetail.addToCart') }}</span>
              </button>
            </template>
          </div>

          <div class="xpd__safe">
            <ShieldCheck />
            <span>
              <b>{{ t('xgai.detail.safeTitle') }}</b>
              <small>{{ t('xgai.detail.safeText') }}</small>
            </span>
          </div>
        </div>
      </section>

      <!-- 说明区 -->
      <section class="x-container xpd__content">
        <div class="xpd__tabs" role="tablist">
          <button
            v-for="tab in tabs"
            :key="tab.key"
            type="button"
            role="tab"
            :aria-selected="activeTab === tab.key"
            :class="{ 'is-active': activeTab === tab.key }"
            @click="activeTab = tab.key"
          >{{ tab.label }}</button>
        </div>

        <!-- 商品说明：后台填写的详情；没有就给出通用说明 -->
        <div v-if="activeTab === 'details'" class="xpd__grid">
          <article>
            <small>{{ t('xgai.detail.beforeBuy') }}</small>
            <h2>{{ t('xgai.detail.notShared') }}</h2>
            <p>{{ t('xgai.detail.notSharedText') }}</p>
            <div class="xpd__note">
              <ShieldCheck />
              <span>
                <b>{{ t('xgai.detail.noPasswordTitle') }}</b>
                <small>{{ t('xgai.detail.noPasswordText') }}</small>
              </span>
            </div>
          </article>
          <div v-if="hasRichContent" class="xpd__prose prose max-w-none prose-a:text-primary prose-img:rounded-md">
            <div v-html="processHtmlForDisplay(getLocalizedText(product.content))" />
          </div>
          <div v-else class="xpd__steps">
            <span v-for="(step, index) in steps" :key="step.title">
              <i><Check v-if="index === steps.length - 1" /><template v-else>{{ index + 1 }}</template></i>
              <b>{{ step.title }}</b>
              <small>{{ step.text }}</small>
            </span>
          </div>
        </div>

        <div v-else-if="activeTab === 'process'" class="xpd__steps xpd__steps--wide">
          <span v-for="(step, index) in steps" :key="step.title">
            <i><Check v-if="index === steps.length - 1" /><template v-else>{{ index + 1 }}</template></i>
            <b>{{ step.title }}</b>
            <small>{{ step.text }}</small>
          </span>
        </div>

        <div v-else class="xpd__faq">
          <details v-for="item in faqs" :key="item.q">
            <summary>{{ item.q }} <ChevronDown /></summary>
            <p>{{ item.a }}</p>
          </details>
        </div>
      </section>

      <!-- 相关文章 -->
      <section v-if="relatedPosts.length" class="x-container xpd__related">
        <h2 class="x-h2">{{ t('productDetail.relatedPosts') }}</h2>
        <div>
          <RouterLink v-for="rp in relatedPosts" :key="rp.id" :to="`/blog/${rp.slug}`">
            <small>{{ formatRelatedPostDate(rp.published_at) }}</small>
            <b>{{ getLocalizedText(rp.title) }}</b>
            <p v-if="rp.summary">{{ getLocalizedText(rp.summary) }}</p>
          </RouterLink>
        </div>
      </section>

      <!-- 移动端固定购买条：桌面购买按钮滚出屏幕后出现 -->
      <Transition name="xpd-bar">
        <div v-if="showMobileBar" class="xpd__bar">
          <span>
            <small>{{ title }}</small>
            <b>{{ formatPrice(lineTotal, siteCurrency) }}</b>
          </span>
          <button v-if="requiresLogin" type="button" class="x-btn x-btn--primary" @click="goLogin">{{ t('productDetail.loginToBuy') }}</button>
          <button v-else type="button" class="x-btn x-btn--primary" :disabled="!canPurchase" @click="buyNow">{{ t('xgai.detail.buyNow') }}</button>
        </div>
      </Transition>
    </template>

    <!-- 找不到 -->
    <div v-else class="x-container xpd__missing">
      <AlertCircle />
      <p>{{ t('productDetail.notFound') }}</p>
      <div>
        <button type="button" class="x-btn x-btn--primary" @click="loadProduct"><RotateCw /> {{ t('errorBoundary.retry') }}</button>
        <RouterLink :to="backToListTo" class="x-btn x-btn--ghost">{{ t('productDetail.backToProducts') }}</RouterLink>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  AlertCircle, ArrowRight, Check, ChevronDown, Minus, Plus, RotateCw, ShieldCheck, ShoppingBag,
} from 'lucide-vue-next'
import { processHtmlForDisplay } from '../../utils/content'
import { useProductDetail } from '../../composables/useProductDetail'
import { brandLogoUrl, detectAiBrand } from '../../utils/aiBrand'
import { compareWithOfficial } from '../../utils/officialPrice'
import './styles/xgai-detail.css'

const { t } = useI18n()

// 移动端固定购买条：桌面购买区离开视口时显示
const purchaseActionsRef = ref<HTMLElement | null>(null)
const showMobileBar = ref(false)
let observer: IntersectionObserver | null = null
const setupMobileBarObserver = () => {
  observer?.disconnect()
  if (!purchaseActionsRef.value) return
  observer = new IntersectionObserver(
    (entries) => {
      const entry = entries[0]
      if (entry) showMobileBar.value = !entry.isIntersecting && entry.boundingClientRect.top < 0
    },
    { threshold: 0 },
  )
  observer.observe(purchaseActionsRef.value)
}

// 监听 ref 绑定，确保商品加载后（DOM 渲染完成）再挂 observer
watch(purchaseActionsRef, (el) => {
  if (el) setupMobileBarObserver()
})

const {
  getLocalizedText, siteCurrency, formatPrice,
  getStockStatusLabel, getStockBadgeVariant,
  hasPromotionPrice, getPromotionPriceAmount,
  hasSkuPromotionPrice,
  hasPromotionRules, getPromotionRules,
  formatPromotionRule, formatWholesaleTier, formatRelatedPostDate, normalizeSkuId,
  loading, product, relatedPosts, currentImage, selectedSkuId, quantity, purchaseWarning,
  activeSkus, selectedSku,
  selectedSkuMemberPrice, hasMemberPrice,
  hasSelectedSkuWholesalePrice, selectedSkuWholesaleFinalIsMember, selectedSkuWholesaleFinalPrice,
  selectedSkuWholesaleRules,
  selectedSkuPromotionPrice, selectedSkuPromotionFinalIsMember, selectedSkuPromotionFinalPrice,
  isSkuPurchasable, skuDisplayText, skuStockText,
  quantityEffectiveLimit, quantityEffectiveMin, handleQuantityInput,
  requiresLogin, requiresSKUSelection, canPurchase, cannotPurchaseReason,
  categoryName, images,
  backToListTo,
  addToCart, buyNow, goLogin, loadProduct,
} = useProductDetail({ onLoaded: () => setupMobileBarObserver() })

const title = computed(() => getLocalizedText(product.value?.title) || '')
const brand = computed(() => detectAiBrand(product.value))
const brandLogo = computed(() => brandLogoUrl(brand.value))
const brandLine = computed(() => {
  if (brand.value === 'openai') return 'OPENAI · CHATGPT'
  if (brand.value === 'grok') return 'xAI · GROK'
  return categoryName.value
})
// 品牌商品默认用 logo 视觉（上游图片常常无法访问）；非品牌商品才用商品图
const showPhoto = computed(() => brand.value === 'other' && Boolean(currentImage.value))
const lede = computed(() => getLocalizedText(product.value?.description) || t('xgai.detail.defaultLede'))
const hasRichContent = computed(() => Boolean(getLocalizedText(product.value?.content)?.trim()))

const stockTone = computed(() => {
  const variant = getStockBadgeVariant(product.value?.stock_status)
  if (variant === 'destructive') return 'out'
  if (variant === 'warning') return 'low'
  return 'in'
})

const currencySymbol = computed(() => (String(siteCurrency.value || 'CNY').toUpperCase() === 'CNY' ? '¥' : `${siteCurrency.value} `))
const priceNumber = (value: unknown) => {
  const n = Number(value)
  if (!Number.isFinite(n)) return '—'
  return Number.isInteger(n) ? String(n) : n.toFixed(2)
}

// 当前单价与划线价：与 classic 页面同一优先级
const priceState = computed(() => {
  const p = product.value
  const sku = selectedSku.value
  if (!p) return { price: 0, strike: null as number | null, tag: '' }
  if (sku && hasSelectedSkuWholesalePrice.value) {
    return {
      price: Number(selectedSkuWholesaleFinalPrice.value),
      strike: Number(sku.price_amount),
      tag: selectedSkuWholesaleFinalIsMember.value ? t('products.memberPriceTag') : t('products.wholesaleTag'),
    }
  }
  if (sku && hasSkuPromotionPrice(sku)) {
    const member = selectedSkuPromotionFinalIsMember.value
    return {
      price: Number(member ? selectedSkuPromotionFinalPrice.value : selectedSkuPromotionPrice.value),
      strike: Number(sku.price_amount),
      tag: member ? t('products.memberPriceTag') : t('products.promotionTag'),
    }
  }
  if (sku && hasMemberPrice.value) {
    return { price: Number(selectedSkuMemberPrice.value), strike: Number(sku.price_amount), tag: t('products.memberPriceTag') }
  }
  if (sku) return { price: Number(sku.price_amount), strike: null, tag: '' }
  if (hasPromotionPrice(p)) {
    return { price: Number(getPromotionPriceAmount(p)), strike: Number(p.price_amount), tag: t('products.promotionTag') }
  }
  return { price: Number(p.price_amount), strike: null, tag: '' }
})
const displayPrice = computed(() => priceState.value.price)
const strikePrice = computed(() => priceState.value.strike)
const priceTag = computed(() => priceState.value.tag)
const lineTotal = computed(() => (displayPrice.value || 0) * (quantity.value || 1))
// 官方价对比只看零售口径（代理价已经另有优惠标签）
const official = computed(() => (currencySymbol.value === '¥' && !priceTag.value ? compareWithOfficial(title.value, displayPrice.value) : null))

const showQuantity = computed(() => quantityEffectiveLimit.value === null || quantityEffectiveLimit.value > 1)

const features = computed(() => {
  const list = [t('xgai.detail.featOwn'), t('xgai.detail.featTrack')]
  if (product.value?.purchase_type === 'guest') list.push(t('xgai.detail.featGuest'))
  else list.push(t('xgai.detail.featMember'))
  return list
})

const steps = computed(() => [
  { title: t('xgai.detail.step1t'), text: t('xgai.detail.step1') },
  { title: t('xgai.detail.step2t'), text: t('xgai.detail.step2') },
  { title: t('xgai.detail.step3t'), text: t('xgai.detail.step3') },
  { title: t('xgai.detail.step4t'), text: t('xgai.detail.step4') },
])

type Tab = 'details' | 'process' | 'faq'
const activeTab = ref<Tab>('details')
const tabs = computed(() => [
  { key: 'details' as const, label: t('xgai.detail.tabDetails') },
  { key: 'process' as const, label: t('xgai.detail.tabProcess') },
  { key: 'faq' as const, label: t('xgai.detail.tabFaq') },
])
const faqs = computed(() => [1, 2, 3, 4].map((n) => ({ q: t(`xgai.detail.faq${n}q`), a: t(`xgai.detail.faq${n}a`) })))

onUnmounted(() => {
  observer?.disconnect()
  observer = null
})
</script>
