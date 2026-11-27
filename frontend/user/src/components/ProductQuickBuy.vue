<template>
  <Teleport to="body">
    <!-- Overlay -->
    <Transition
      appear
      enter-active-class="transition duration-300 ease-out"
      enter-from-class="opacity-0"
      enter-to-class="opacity-100"
      leave-active-class="transition duration-200 ease-in"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0"
    >
      <div
        v-if="visible"
        class="fixed inset-0 z-50 bg-black/40 backdrop-blur-[2px]"
        @click="close"
      />
    </Transition>

    <!--
      Responsive panel:
      - Mobile (<md): bottom sheet, slides up
      - Desktop (md+): centered modal, fades + scales in
    -->
    <Transition
      appear
      :enter-active-class="sourceElement ? '' : 'quick-buy-enter-active'"
      :enter-from-class="sourceElement ? '' : 'quick-buy-enter-from'"
      :enter-to-class="sourceElement ? '' : 'quick-buy-enter-to'"
      :leave-active-class="sourceElement ? '' : 'quick-buy-leave-active'"
      :leave-from-class="sourceElement ? '' : 'quick-buy-leave-from'"
      :leave-to-class="sourceElement ? '' : 'quick-buy-leave-to'"
      v-bind="sourceElement ? { onEnter: enterFromCard, onLeave: returnToCard } : {}"
    >
      <div
        v-if="visible"
        class="fixed z-50 inset-x-0 bottom-0 md:inset-0 md:flex md:items-center md:justify-center md:p-6"
        @click.self="close"
      >
        <div
          ref="dialogPanel"
          role="dialog"
          tabindex="-1"
          :aria-label="t('quickBuy.title')"
          aria-modal="true"
          class="
            relative w-full max-h-[85vh] flex flex-col
            rounded-t-2xl md:rounded-2xl
            bg-card/95 backdrop-blur-xl text-card-foreground border-t md:border
            shadow-2xl
            md:max-w-md md:max-h-[75vh]
          "
          :class="{ 'quick-buy-checkout': inlineCheckout, 'quick-buy-checkout--paying': inlineCheckout && paymentCreated }"
          @click.stop
        >
          <!-- Mobile drag handle -->
          <div class="md:hidden flex justify-center pt-2.5 pb-1 shrink-0">
            <div class="w-8 h-1 rounded-full bg-gray-300 dark:bg-gray-600" />
          </div>

          <!-- Desktop header bar -->
          <div class="hidden md:flex items-center justify-between px-5 pt-4 pb-0 shrink-0">
            <h2 class="text-sm font-semibold text-foreground">{{ t('quickBuy.title') }}</h2>
            <Button type="button" variant="ghost" size="icon" class="-mr-1 text-muted-foreground" @click="close">
              <X />
            </Button>
          </div>

          <!-- Scrollable content area -->
          <div class="overflow-y-auto overscroll-contain flex-1 px-4 md:px-5 md:pt-4" :class="{ 'quick-buy-checkout__columns': inlineCheckout }">
            <div :class="{ 'quick-buy-checkout__info': inlineCheckout }" :tabindex="inlineCheckout ? 0 : undefined" :aria-label="inlineCheckout ? productTitle : undefined">
            <!-- Product header -->
            <div class="flex gap-3.5 mb-4" :class="{ 'quick-buy-checkout__product': inlineCheckout }">
              <!-- Image -->
              <div
                class="w-[90px] h-[90px] md:w-[100px] md:h-[100px] rounded-xl md:rounded-2xl overflow-hidden shrink-0 border cursor-pointer"
                @click="goToDetail"
              >
                <img
                  v-if="productImage && !productImageFailed"
                  :src="productImage"
                  :alt="productTitle"
                  @error="productImageFailed = true"
                  class="w-full h-full"
                  :class="aiBrand.productArtworkUrl(product) ? 'object-cover' : 'object-contain p-2'"
                />
                <div v-else class="w-full h-full flex items-center justify-center bg-muted">
                  <ImageIcon class="w-7 h-7 text-muted-foreground" :stroke-width="1.5" />
                </div>
              </div>

              <!-- Info -->
              <div class="flex-1 min-w-0 flex flex-col">
                <h3
                  class="text-sm md:text-[15px] font-semibold text-foreground line-clamp-2 leading-snug cursor-pointer hover:underline decoration-gray-300 dark:decoration-gray-600 underline-offset-2"
                  @click="goToDetail"
                >
                  {{ productTitle }}
                </h3>

                <div class="mt-1.5 flex flex-wrap items-center gap-1">
                  <Badge :variant="product.fulfillment_type === 'auto' ? 'info' : 'neutral'" size="xs">
                    {{ getFulfillmentTypeLabel(product.fulfillment_type) }}
                  </Badge>
                  <Badge :variant="getStockBadgeVariant(product.stock_status)" size="xs">
                    {{ getStockStatusLabel(product) }}
                  </Badge>
                  <Badge v-if="hasSelectedSkuWholesalePrice" variant="success" size="xs">
                    {{ t('products.wholesaleTag') }}
                  </Badge>
                  <Badge v-if="showSelectedSkuMemberBadge" variant="warning" size="xs">
                    {{ t('products.memberPriceTag') }}
                  </Badge>
                </div>

                <!-- Price -->
                <div class="mt-auto pt-1">
                  <template v-if="selectedSku && hasSelectedSkuWholesalePrice">
                    <span
                      class="text-lg md:text-xl font-bold"
                      :class="selectedSkuWholesaleFinalIsMember ? 'text-amber-600 dark:text-amber-300' : 'text-emerald-600 dark:text-emerald-400'"
                    >
                      {{ formatPrice(selectedSkuWholesaleFinalPrice!, siteCurrency) }}
                    </span>
                    <span class="ml-1.5 text-xs text-muted-foreground line-through">
                      {{ formatPrice(selectedSku.price_amount, siteCurrency) }}
                    </span>
                  </template>
                  <template v-else-if="selectedSku && hasSkuPromotionPrice(selectedSku)">
                    <span
                      class="text-lg md:text-xl font-bold"
                      :class="selectedSkuPromotionFinalIsMember ? 'text-amber-600 dark:text-amber-300' : 'text-rose-600 dark:text-rose-400'"
                    >
                      {{ formatPrice(selectedSkuPromotionFinalPrice!, siteCurrency) }}
                    </span>
                    <span class="ml-1.5 text-xs text-muted-foreground line-through">
                      {{ formatPrice(selectedSku.price_amount, siteCurrency) }}
                    </span>
                  </template>
                  <template v-else-if="selectedSku && hasMemberPrice">
                    <span class="text-lg md:text-xl font-bold text-amber-600 dark:text-amber-300">
                      {{ formatPrice(selectedSkuMemberPrice!, siteCurrency) }}
                    </span>
                    <span class="ml-1.5 text-xs text-muted-foreground line-through">
                      {{ formatPrice(selectedSku.price_amount, siteCurrency) }}
                    </span>
                  </template>
                  <template v-else-if="selectedSku">
                    <span class="text-lg md:text-xl font-bold text-primary">
                      {{ formatPrice(selectedSku.price_amount, siteCurrency) }}
                    </span>
                  </template>
                  <template v-else-if="hasPromotionPrice(product)">
                    <span class="text-lg md:text-xl font-bold text-rose-600 dark:text-rose-400">
                      {{ formatPrice(getPromotionPriceAmount(product), siteCurrency) }}
                    </span>
                    <span class="ml-1.5 text-xs text-muted-foreground line-through">
                      {{ formatPrice(product.price_amount, siteCurrency) }}
                    </span>
                  </template>
                  <template v-else>
                    <span class="text-lg md:text-xl font-bold text-primary">
                      {{ formatPrice(product.price_amount, siteCurrency) }}
                    </span>
                  </template>
                </div>
              </div>

              <!-- Mobile close -->
              <Button type="button" variant="ghost" size="icon" class="md:hidden self-start -mr-1 text-muted-foreground" @click="close">
                <X />
              </Button>
            </div>

            <!-- Quantity -->
            <div v-if="!inlineCheckout" class="mb-4 flex items-center gap-3" :class="{ 'quick-buy-checkout__quantity': inlineCheckout }">
              <span class="text-xs font-medium text-muted-foreground">{{ t('quickBuy.quantity') }}</span>
              <div class="flex items-center rounded-lg border overflow-hidden">
                <button
                  type="button"
                  class="w-9 h-9 flex items-center justify-center text-muted-foreground hover:bg-secondary transition-colors cursor-pointer disabled:cursor-not-allowed disabled:opacity-30"
                  :disabled="paymentCreated || checkoutBusy || quantity <= effectiveMin"
                  @click="quantity = Math.max(effectiveMin, quantity - 1)"
                >
                  <Minus class="w-3.5 h-3.5" :stroke-width="2.5" />
                </button>
                <input
                  type="text"
                  inputmode="numeric"
                  :aria-label="t('quickBuy.quantity')"
                  :disabled="paymentCreated || checkoutBusy"
                  class="w-12 h-9 text-center text-sm font-semibold text-foreground border-x bg-transparent outline-none tabular-nums [appearance:textfield] [&::-webkit-outer-spin-button]:appearance-none [&::-webkit-inner-spin-button]:appearance-none"
                  :value="quantity"
                  @change="handleQuantityInput($event)"
                  @keydown.enter.prevent="($event.target as HTMLInputElement)?.blur()"
                />
                <button
                  type="button"
                  class="w-9 h-9 flex items-center justify-center text-muted-foreground hover:bg-secondary transition-colors cursor-pointer disabled:cursor-not-allowed disabled:opacity-30"
                  :disabled="paymentCreated || checkoutBusy || (effectiveLimit !== null && quantity >= effectiveLimit)"
                  @click="quantity = quantity + 1"
                >
                  <Plus class="w-3.5 h-3.5" :stroke-width="2.5" />
                </button>
              </div>
            </div>

            <!-- Divider -->
            <div class="h-px bg-border -mx-4 md:-mx-5 mb-4" />

            <div v-if="inlineCheckout && productContent" class="quick-buy-checkout__description" v-html="productContent" />
            <!-- Product description -->
            <p v-if="productDescription && !productContent" class="mb-4 text-xs leading-relaxed text-muted-foreground" :class="{ 'quick-buy-checkout__description': inlineCheckout }">
              {{ productDescription }}
            </p>

            <section v-if="inlineCheckout && buyingGuide" class="quick-buy-checkout__guide">
              <h4>{{ guideLabels.suitable }}</h4><p>{{ buyingGuide.suitable }}</p>
              <h4>{{ guideLabels.before }}</h4>
              <ul><li v-for="item in buyingGuide.before" :key="item">{{ item }}</li></ul>
              <a :href="appStore.redemptionUrl()" v-if="buyingGuide.kind === 'gpt'" target="_blank" rel="noopener noreferrer">{{ guideLabels.redeem }} ↗</a>
            </section>

            <!-- Promotion rules -->
            <div v-if="selectedSkuWholesaleRules.length" class="mb-4 rounded-lg border border-emerald-200 bg-emerald-50/50 px-3 py-2 dark:border-emerald-800/50 dark:bg-emerald-950/20">
              <div class="mb-1 text-[11px] font-semibold text-emerald-700 dark:text-emerald-300">
                {{ t('products.wholesaleRulesTitle') }}
              </div>
              <div class="flex flex-wrap gap-1">
                <Badge v-for="tier in selectedSkuWholesaleRules" :key="`${tier.sku_id || tier.sku_code || 'all'}-${tier.min_quantity}`" variant="success" size="xs" class="rounded-full">
                  {{ formatWholesaleTier(tier) }}
                </Badge>
              </div>
            </div>

            <div v-if="hasPromotionRules(product)" class="mb-4 rounded-lg border border-orange-200 dark:border-orange-800/50 bg-orange-50/50 dark:bg-orange-950/20 px-3 py-2">
              <div class="flex items-center gap-1 mb-1">
                <Tag class="w-3.5 h-3.5 text-orange-500 dark:text-orange-400 shrink-0" />
                <span class="text-[11px] font-semibold text-orange-700 dark:text-orange-300">
                  {{ t('products.promotionRulesTitle') }}
                </span>
              </div>
              <ul class="space-y-0.5">
                <li v-for="rule in getPromotionRules(product)" :key="rule.id" class="text-[11px] text-orange-600 dark:text-orange-300/90 flex items-center gap-1">
                  <span class="w-1 h-1 rounded-full bg-orange-400 dark:bg-orange-500 shrink-0"></span>
                  <span>{{ formatPromotionRule(rule) }}</span>
                </li>
              </ul>
            </div>

            <!-- SKU Selection -->
            <div v-if="activeSkus.length > 1" class="mb-4">
              <div class="mb-2 text-xs font-medium text-muted-foreground">
                {{ t('quickBuy.selectSku') }}
              </div>
              <div class="flex flex-wrap gap-2">
                <button
                  v-for="sku in activeSkus"
                  :key="sku.id"
                  type="button"
                  class="rounded-lg border px-3 py-1.5 text-[13px] transition-all"
                  :class="[
                    normalizeSkuId(sku.id) === selectedSkuId
                      ? 'border-primary/45 bg-primary/10 ring-1 ring-primary/30 font-semibold'
                      : 'bg-secondary text-secondary-foreground hover:bg-secondary/80 font-medium',
                    isSkuPurchasable(sku) ? 'cursor-pointer' : 'cursor-not-allowed opacity-45 border-dashed',
                  ]"
                  :disabled="paymentCreated || checkoutBusy || !isSkuPurchasable(sku)"
                  @click="selectedSkuId = normalizeSkuId(sku.id)"
                >
                  <span>{{ skuDisplayText(sku) }}</span>
                  <span
                    v-if="!isSkuPurchasable(sku)"
                    class="ml-1 text-[10px] opacity-70"
                  >({{ t('productDetail.skuStockOut') }})</span>
                </button>
              </div>
            </div>

            <!-- Selected SKU stock info -->
            <div v-if="selectedSku && !inlineCheckout" class="mb-4 flex items-center gap-2 text-xs">
              <span class="text-muted-foreground">{{ t('quickBuy.stock') }}:</span>
              <span
                class="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[11px] font-medium"
                :class="skuStockBadgeClass(selectedSku)"
              >
                <span
                  class="w-1.5 h-1.5 rounded-full"
                  :class="skuStockDotClass(selectedSku)"
                />
                {{ skuStockText(selectedSku) }}
              </span>
            </div>


            <!-- Warning -->
            <p v-if="stockBelowMinPurchase" class="mb-4 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2 text-xs font-medium text-warning">
              {{ t('productDetail.stockBelowMinPurchase', { count: effectiveMin }) }}
            </p>
            <p v-else-if="purchaseWarning" class="mb-4 rounded-lg border border-warning/40 bg-warning/10 px-3 py-2 text-xs font-medium text-warning">
              {{ purchaseWarning }}
            </p>
            </div>
            <XgaiCheckout v-show="!loginVisible" @login="loginVisible = true" v-if="inlineCheckout && canPurchase && !requiresLogin && buyNowStore.hasItem" embedded @busy="checkoutBusy = $event" @payment="paymentCreated = $event">
              <template #quantity>
            <!-- Quantity -->
            <div v-if="inlineCheckout" class="mb-4 flex items-center gap-3" :class="{ 'quick-buy-checkout__quantity': inlineCheckout }">
              <span class="text-xs font-medium text-muted-foreground">{{ t('quickBuy.quantity') }}</span>
              <div class="flex items-center rounded-lg border overflow-hidden">
                <button
                  type="button"
                  class="w-9 h-9 flex items-center justify-center text-muted-foreground hover:bg-secondary transition-colors cursor-pointer disabled:cursor-not-allowed disabled:opacity-30"
                  :disabled="paymentCreated || checkoutBusy || quantity <= effectiveMin"
                  @click="quantity = Math.max(effectiveMin, quantity - 1)"
                >
                  <Minus class="w-3.5 h-3.5" :stroke-width="2.5" />
                </button>
                <input
                  type="text"
                  inputmode="numeric"
                  :aria-label="t('quickBuy.quantity')"
                  :disabled="paymentCreated || checkoutBusy"
                  class="w-12 h-9 text-center text-sm font-semibold text-foreground border-x bg-transparent outline-none tabular-nums [appearance:textfield] [&::-webkit-outer-spin-button]:appearance-none [&::-webkit-inner-spin-button]:appearance-none"
                  :value="quantity"
                  @change="handleQuantityInput($event)"
                  @keydown.enter.prevent="($event.target as HTMLInputElement)?.blur()"
                />
                <button
                  type="button"
                  class="w-9 h-9 flex items-center justify-center text-muted-foreground hover:bg-secondary transition-colors cursor-pointer disabled:cursor-not-allowed disabled:opacity-30"
                  :disabled="paymentCreated || checkoutBusy || (effectiveLimit !== null && quantity >= effectiveLimit)"
                  @click="quantity = quantity + 1"
                >
                  <Plus class="w-3.5 h-3.5" :stroke-width="2.5" />
                </button>
              </div>
            </div>

              </template>
            </XgaiCheckout>
            <div v-if="inlineCheckout && loginVisible" class="quick-buy-checkout__login">
              <button type="button" class="x-link" @click="loginVisible = false">← {{ t('checkout.title') }}</button>
              <XgaiLogin embedded @success="loginVisible = false" />
            </div>
          </div>

          <!-- Actions (sticky bottom) -->
          <div v-if="(!inlineCheckout || requiresLogin || !canPurchase)" class="shrink-0 px-4 md:px-5 pt-3 pb-3 md:pb-5 border-t theme-safe-bottom">
            <Button
              v-if="requiresLogin"
              class="w-full py-3 h-auto min-h-[44px] rounded-xl text-sm font-semibold"
              @click="goLogin"
            >
              {{ t('quickBuy.loginToBuy') }}
            </Button>
            <div v-else-if="isSoldOut(product)" class="flex gap-3">
              <Button
                variant="secondary"
                class="flex-1 py-3 h-auto min-h-[44px] rounded-xl text-sm font-semibold"
                @click="goToDetail"
              >
                {{ t('quickBuy.viewDetail') }}
              </Button>
            </div>
            <div v-else class="flex gap-3">
              <Button
                variant="secondary"
                :disabled="!canPurchase"
                class="flex-1 py-3 h-auto min-h-[44px] rounded-xl text-sm font-semibold gap-1.5"
                @click="handleAddToCart"
              >
                <ShoppingCart class="w-4 h-4" />
                {{ t('quickBuy.addToCart') }}
              </Button>
              <Button
                :disabled="!canPurchase"
                class="flex-1 py-3 h-auto min-h-[44px] rounded-xl text-sm font-semibold"
                @click="handleBuyNow"
              >
                {{ t('quickBuy.buyNow') }}
              </Button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>


<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount, nextTick } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'
import { useCartStore } from '../stores/cart'
import { useBuyNowStore } from '../stores/buyNow'
import { useUserAuthStore } from '../stores/userAuth'
import { useUserProfileStore } from '../stores/userProfile'
import { getFirstImageUrl, getImageUrl } from '../utils/image'
import * as aiBrand from '../utils/aiBrand'
import { normalizeSkuId, buildSkuDisplayText } from '../utils/sku'
import { resolveSkuAvailableStock, resolveSkuStockDisplay, type PublicStockDisplay } from '../utils/publicStock'
import { useLocalized, useProductLabels } from '../composables/useProduct'
import { useConfirmDialog } from '../composables/useConfirmDialog'
import { toast } from '../composables/useToast'
import { X, Image as ImageIcon, Tag, Minus, Plus, ShoppingCart } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import XgaiCheckout from '../templates/xgai/Checkout.vue'
import XgaiLogin from '../templates/xgai/auth/Login.vue'
import { getProductGuide, productGuideLabels } from '../utils/productGuide'
import { processHtmlForDisplay } from '../utils/content'
import { productAPI } from '../api/product'

const props = defineProps<{
  product: any
  visible: boolean
  sourceElement?: HTMLElement
  inlineCheckout?: boolean
}>()

const emit = defineEmits<{
  'update:visible': [value: boolean]
}>()

const loadedProduct = ref<any>(null)
const product = computed(() => loadedProduct.value || props.product)
let detailRequest = 0
watch(() => [props.product, props.visible], async () => {
  const request = ++detailRequest
  loadedProduct.value = null
  if (!props.visible || !props.inlineCheckout || !props.product?.slug) return
  try {
    const response = await productAPI.detail(props.product.slug)
    if (request === detailRequest) loadedProduct.value = response.data.data
  } catch { /* Keep the catalog snapshot; checkout still validates availability on the server. */ }
}, { immediate: true })

const router = useRouter()
const route = useRoute()
const { t } = useI18n()
const appStore = useAppStore()
const cartStore = useCartStore()
const buyNowStore = useBuyNowStore()
const userAuthStore = useUserAuthStore()
const userProfileStore = useUserProfileStore()

const { getLocalizedText, siteCurrency, formatPrice } = useLocalized()
const {
  getFulfillmentTypeLabel,
  getStockBadgeVariant,
  getStockStatusLabel,
  isSoldOut,
  hasPromotionPrice,
  getPromotionPriceAmount,
  hasSkuPromotionPrice,
  getSkuPromotionPriceAmount,
  hasPromotionRules,
  getPromotionRules,
  getWholesalePrices,
  resolveWholesalePriceAmount,
  resolveMemberPriceAmount,
} = useProductLabels()

const dialogPanel = ref<HTMLElement | null>(null)
let cardAnimation: Animation | undefined
let hiddenSource: HTMLElement | undefined
const animateCardWindow = (wrapper: Element, done: () => void, entering: boolean) => {
  const source = props.sourceElement
  const panel = wrapper.querySelector<HTMLElement>('[role="dialog"]')
  if (!source || !panel) { done(); return }
  const origin = source.getBoundingClientRect()
  const destination = panel.getBoundingClientRect()
  if (cardAnimation) { cardAnimation.onfinish = null; cardAnimation.cancel() }
  const radius = getComputedStyle(source).borderRadius
  const finalRadius = getComputedStyle(panel).borderRadius
  const duration = window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 0 : entering ? 680 : 480
  // Keep content at its final width while the surrounding card unfolds, avoiding stretched text.
  Array.from(panel.children).forEach(child => { (child as HTMLElement).style.width = `${destination.width - 2}px` })
  const frames = [
    { left: `${origin.left}px`, top: `${origin.top}px`, width: `${origin.width}px`, height: `${origin.height}px`, borderRadius: radius },
    { left: `${destination.left}px`, top: `${destination.top}px`, width: `${destination.width}px`, height: `${destination.height}px`, borderRadius: finalRadius },
  ]
  Object.assign(panel.style, { position: 'fixed', margin: '0', maxHeight: 'none', minHeight: '0', overflow: 'hidden' })
  hiddenSource = source
  source.style.visibility = 'hidden'
  const content = panel.querySelector<HTMLElement>('.quick-buy-checkout__columns')
  content?.animate(entering ? [{ opacity: 0 }, { opacity: 1 }] : [{ opacity: 1 }, { opacity: 0 }], { duration: entering ? 400 : 180, delay: entering ? 180 : 0, fill: 'both' })
  const animation = panel.animate(entering ? frames : [...frames].reverse(), { duration, easing: 'cubic-bezier(.22,1,.36,1)', fill: 'both' })
  cardAnimation = animation
  animation.onfinish = () => {
    if (!entering) { source.dataset.revealCooldown = '1'; source.style.visibility = ''; hiddenSource = undefined }
    done()
    animation.cancel()
    panel.removeAttribute('style')
    Array.from(panel.children).forEach(child => { (child as HTMLElement).style.width = '' })
    cardAnimation = undefined
  }
}
const enterFromCard = (element: Element, done: () => void) => animateCardWindow(element, done, true)
const returnToCard = (element: Element, done: () => void) => animateCardWindow(element, done, false)

const loginVisible = ref(false)
const checkoutBusy = ref(false)
const paymentCreated = ref(false)
const hasCreatedOrder = ref(false)
watch(paymentCreated, value => { if (value) hasCreatedOrder.value = true })
let previousFocus: HTMLElement | null = null
let previousOverflow: string | undefined
const selectedSkuId = ref(0)
const quantity = ref(1)
const purchaseWarning = ref('')

const productTitle = computed(() => getLocalizedText(product.value?.title))
const buyingGuide = computed(() => getProductGuide(product.value?.slug, appStore.locale))
const guideLabels = computed(() => productGuideLabels(appStore.locale))
const productContent = computed(() => props.inlineCheckout ? processHtmlForDisplay(getLocalizedText(product.value?.content) || '') : '')
const productDescription = computed(() => getLocalizedText(product.value?.description) || getProductGuide(product.value?.slug, appStore.locale)?.summary || '')

const formatPromotionRule = (rule: any) => {
  const amount = formatPrice(rule.min_amount, siteCurrency.value)
  const value = rule.type === 'percent' ? String(rule.value) : formatPrice(rule.value, siteCurrency.value)
  const hasMin = Number(rule.min_amount) > 0
  switch (rule.type) {
    case 'percent':
      return hasMin ? t('products.promotionHintPercent', { amount, value }) : t('products.promotionHintPercentNoMin', { value })
    case 'fixed':
      return hasMin ? t('products.promotionHintFixed', { amount, value }) : t('products.promotionHintFixedNoMin', { value })
    case 'special_price':
      return hasMin ? t('products.promotionHintSpecial', { amount, value }) : t('products.promotionHintSpecialNoMin', { value })
    default:
      return rule.name || ''
  }
}

const productImageFailed = ref(false)
const productImage = computed(() => {
  const p = product.value
  if (!p) return ''
  const artwork = aiBrand.productArtworkUrl(p)
  if (artwork) return artwork
  const brandLogo = aiBrand.brandLogoUrl(aiBrand.detectAiBrand(p))
  if (brandLogo) return brandLogo
  if (/claude|anthropic/i.test([getLocalizedText(p.title), p.slug].join(' '))) return '/images/brands/claude.svg'
  if (p.images) {
    const url = getFirstImageUrl(p.images)
    if (url) return url
  }
  if (p.category?.icon) return getImageUrl(p.category.icon)
  // 降级：根据标题推断 AI 品牌图标
  const { detectAiBrand, brandLogoUrl } = aiBrand
  return brandLogoUrl(detectAiBrand(p))
})

watch(productImage, () => { productImageFailed.value = false })

const activeSkus = computed(() => {
  const rows = Array.isArray(product.value?.skus) ? product.value.skus : []
  return rows.filter((sku: any) => Boolean(sku?.is_active))
})

const selectedSku = computed(() => {
  if (selectedSkuId.value <= 0) return null
  return activeSkus.value.find((sku: any) => normalizeSkuId(sku?.id) === selectedSkuId.value) || null
})

const userMemberLevelId = computed(() => Number(userAuthStore.user?.member_level_id || 0))

const currentMemberDiscountRate = computed(() => {
  if (!userMemberLevelId.value) return 0
  const level = userProfileStore.memberLevels.find((item: any) => Number(item?.id || 0) === userMemberLevelId.value)
  return Number(level?.discount_rate || 0)
})

const ensureMemberLevels = () => {
  if (userMemberLevelId.value > 0 && userProfileStore.memberLevels.length === 0) {
    void userProfileStore.loadMemberLevels()
  }
}

const getMemberPriceForSku = (skuId: number, basePrice: any): number | null => {
  const price = resolveMemberPriceAmount(product.value, skuId, basePrice, userMemberLevelId.value, currentMemberDiscountRate.value)
  return price === null ? null : Number(price)
}

const selectedSkuMemberPrice = computed(() => {
  if (!selectedSku.value) return null
  return getMemberPriceForSku(normalizeSkuId(selectedSku.value.id), selectedSku.value.price_amount)
})

const hasMemberPrice = computed(() => {
  if (!selectedSku.value || selectedSkuMemberPrice.value === null) return false
  return selectedSkuMemberPrice.value < Number(selectedSku.value.price_amount || 0)
})

const selectedSkuWholesaleRules = computed(() => {
  if (!product.value || !selectedSku.value) return []
  return getWholesalePrices(
    product.value,
    normalizeSkuId(selectedSku.value.id),
    selectedSku.value.sku_code,
  )
})

const selectedSkuWholesalePrice = computed(() => {
  if (!product.value || !selectedSku.value) return null
  return resolveWholesalePriceAmount(
    product.value,
    selectedSku.value.price_amount,
    quantity.value,
    normalizeSkuId(selectedSku.value.id),
    selectedSku.value.sku_code,
    quantity.value,
  )
})

const hasSelectedSkuWholesalePrice = computed(() => {
  if (!selectedSku.value || !selectedSkuWholesalePrice.value) return false
  const comparisonPrice = hasSkuPromotionPrice(selectedSku.value)
    ? Number(getSkuPromotionPriceAmount(selectedSku.value))
    : Number(selectedSku.value.price_amount || 0)
  return Number(selectedSkuWholesalePrice.value) < comparisonPrice
})

const selectedSkuWholesaleMemberPrice = computed(() => {
  if (!selectedSku.value || !selectedSkuWholesalePrice.value) return null
  return getMemberPriceForSku(normalizeSkuId(selectedSku.value.id), selectedSkuWholesalePrice.value)
})

const selectedSkuWholesaleFinalIsMember = computed(() => hasSelectedSkuWholesalePrice.value && selectedSkuWholesaleMemberPrice.value !== null)

const selectedSkuWholesaleFinalPrice = computed(() => {
  if (!hasSelectedSkuWholesalePrice.value || selectedSkuWholesalePrice.value === null) return null
  return selectedSkuWholesaleMemberPrice.value !== null ? selectedSkuWholesaleMemberPrice.value : selectedSkuWholesalePrice.value
})

const selectedSkuPromotionPrice = computed(() => {
  if (!selectedSku.value || !hasSkuPromotionPrice(selectedSku.value)) return null
  return getSkuPromotionPriceAmount(selectedSku.value)
})

const selectedSkuPromotionMemberPrice = computed(() => {
  if (!selectedSku.value || selectedSkuPromotionPrice.value === null) return null
  return getMemberPriceForSku(normalizeSkuId(selectedSku.value.id), selectedSkuPromotionPrice.value)
})

const selectedSkuPromotionFinalIsMember = computed(() => selectedSkuPromotionMemberPrice.value !== null)

const selectedSkuPromotionFinalPrice = computed(() => {
  if (selectedSkuPromotionPrice.value === null) return null
  return selectedSkuPromotionMemberPrice.value !== null ? selectedSkuPromotionMemberPrice.value : selectedSkuPromotionPrice.value
})

const showSelectedSkuMemberBadge = computed(() => {
  if (!selectedSku.value) return false
  if (hasSelectedSkuWholesalePrice.value) return selectedSkuWholesaleFinalIsMember.value
  if (hasSkuPromotionPrice(selectedSku.value)) return selectedSkuPromotionFinalIsMember.value
  return hasMemberPrice.value
})

const formatWholesaleTier = (tier: any) => t('products.wholesaleTier', {
  count: Number(tier?.min_quantity || 0),
  price: formatPrice(tier?.unit_price, siteCurrency.value),
})

// Stock helpers (same logic as ProductDetail)
const normalizeStockNumber = (value: unknown) => {
  const n = Number(value)
  if (!Number.isFinite(n)) return 0
  return Math.max(Math.floor(n), 0)
}

const normalizeManualStockTotal = (value: unknown) => {
  const n = Number(value)
  if (!Number.isFinite(n)) return 0
  const i = Math.floor(n)
  if (i === -1) return -1
  return Math.max(i, 0)
}

const normalizeOptionalLimitNumber = (value: unknown) => {
  const n = Number(value)
  if (!Number.isFinite(n)) return null
  const i = Math.floor(n)
  if (i <= 0) return null
  return i
}

const effectiveMin = computed(() => {
  const productMin = normalizeOptionalLimitNumber(product.value?.min_purchase_quantity)
  return productMin && productMin > 0 ? productMin : 1
})

const shouldEnforceSkuStock = (sku: any) => {
  if (!sku) return false
  if (sku?.stock_quantity_hidden === true || product.value?.stock_quantity_hidden === true) return false
  if (product.value?.fulfillment_type === 'auto') return true
  if (product.value?.fulfillment_type === 'upstream') return true
  if (product.value?.fulfillment_type !== 'manual') return false
  const total = normalizeManualStockTotal(sku?.manual_stock_total)
  return total !== -1
}

const skuAvailableStock = (sku: any) => {
  if (!sku) return 0
  if (!shouldEnforceSkuStock(sku) && !sku?.stock_quantity_hidden) return null
  return resolveSkuAvailableStock(product.value, sku)
}

const isSkuPurchasable = (sku: any) => {
  const available = skuAvailableStock(sku)
  if (available === null) return true
  return available > 0
}

const syncSelectedSku = () => {
  const rows = activeSkus.value
  if (rows.length === 0) { selectedSkuId.value = 0; return }
  if (rows.length === 1) { selectedSkuId.value = normalizeSkuId(rows[0]?.id); return }
  const firstAvailable = rows.find((sku: any) => isSkuPurchasable(sku))
  if (firstAvailable) { selectedSkuId.value = normalizeSkuId(firstAvailable?.id); return }
  selectedSkuId.value = normalizeSkuId(rows[0]?.id)
}

// Reset state when product changes or drawer opens
watch(() => [props.product, props.visible], () => {
  if (props.visible && product.value) {
    paymentCreated.value = false
    hasCreatedOrder.value = false
    purchaseWarning.value = ''
    quantity.value = effectiveMin.value
    syncSelectedSku()
    ensureMemberLevels()
  }
}, { immediate: true })

watch(activeSkus, () => {
  if (props.visible && !activeSkus.value.some((sku: any) => normalizeSkuId(sku.id) === selectedSkuId.value && isSkuPurchasable(sku))) syncSelectedSku()
})

const skuStockText = (sku: any) => {
  const display = resolveSkuStockDisplay(product.value, sku)
  return formatSkuStockDisplay(display)
}

const formatSkuStockDisplay = (display: PublicStockDisplay) => {
  switch (display.kind) {
    case 'unlimited':
      return t('productDetail.skuStockUnlimited')
    case 'out':
      return t('productDetail.skuStockOut')
    case 'remaining':
      return t('productDetail.skuStockRemaining', { count: display.count })
    case 'low_stock':
      return t('productDetail.skuStockLow')
    case 'hidden':
      return t('productDetail.skuStockHidden')
    case 'range':
      return t('productDetail.skuStockRange', { min: display.min, max: display.max })
    case 'range_plus':
      return t('productDetail.skuStockRangePlus', { min: display.min })
    case 'in_stock':
    default:
      return t('productDetail.skuStockInStock')
  }
}

const skuStockBadgeClass = (sku: any) => {
  const display = resolveSkuStockDisplay(product.value, sku)
  if (display.kind === 'unlimited' || display.kind === 'hidden') return 'border-slate-200 text-slate-600 dark:border-slate-700 dark:text-slate-300'
  if (display.kind === 'out') return 'border-rose-200 bg-rose-50 text-rose-700 dark:border-rose-700 dark:bg-rose-950/30 dark:text-rose-300'
  if (display.kind === 'low_stock' || (display.kind === 'range' && display.max <= 5)) return 'border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-700 dark:bg-amber-950/30 dark:text-amber-300'
  return 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-300'
}

const skuStockDotClass = (sku: any) => {
  const display = resolveSkuStockDisplay(product.value, sku)
  if (display.kind === 'unlimited' || display.kind === 'hidden') return 'bg-slate-400 dark:bg-slate-500'
  if (display.kind === 'out') return 'bg-rose-500 dark:bg-rose-400'
  if (display.kind === 'low_stock' || (display.kind === 'range' && display.max <= 5)) return 'bg-amber-500 dark:bg-amber-400'
  return 'bg-emerald-500 dark:bg-emerald-400'
}

const skuDisplayText = (sku: any) => buildSkuDisplayText({
  skuCode: sku?.sku_code,
  specValues: sku?.spec_values,
  fallback: t('productDetail.skuFallback'),
  locale: appStore.locale,
})

const effectiveLimit = computed(() => {
  const sku = selectedSku.value
  const available = skuAvailableStock(sku)
  const productLimit = normalizeOptionalLimitNumber(product.value?.max_purchase_quantity)
  let limit: number | null = productLimit
  if (available !== null) {
    limit = limit === null ? available : Math.min(limit, available)
  }
  return limit
})

const purchaseType = computed(() => product.value?.purchase_type || 'member')
const requiresLogin = computed(() => purchaseType.value === 'member' && !userAuthStore.isAuthenticated)
const requiresSKUSelection = computed(() => activeSkus.value.length > 1 && !selectedSku.value)
const stockBelowMinPurchase = computed(() => {
  const limit = effectiveLimit.value
  if (limit === null) return false
  return limit < effectiveMin.value
})
const canPurchase = computed(() => {
  if (!product.value) return false
  if (activeSkus.value.length === 0) return false
  if (product.value.is_sold_out) return false
  if (requiresSKUSelection.value) return false
  if (product.value.stock_status === 'out_of_stock') return false
  if (selectedSku.value && !isSkuPurchasable(selectedSku.value)) return false
  if (stockBelowMinPurchase.value) return false
  return true
})

const selectedCartQuantity = () => {
  if (!product.value || !selectedSku.value) return 0
  const productId = Number(product.value.id || 0)
  const skuId = normalizeSkuId(selectedSku.value?.id)
  if (productId <= 0 || skuId <= 0) return 0
  const matched = cartStore.items.find((item) => item.productId === productId && normalizeSkuId(item.skuId) === skuId)
  return Number(matched?.quantity || 0)
}

const handleQuantityInput = (event: Event) => {
  const input = event.target as HTMLInputElement
  const parsed = Math.floor(Number(input.value))
  const minimum = effectiveMin.value
  if (!Number.isFinite(parsed) || parsed < minimum) {
    quantity.value = minimum
    input.value = String(minimum)
    return
  }
  const limit = effectiveLimit.value
  if (limit !== null && parsed > limit) {
    quantity.value = limit
    input.value = String(limit)
    return
  }
  quantity.value = parsed
}

const { confirm: confirmExit, visible: exitDialogVisible } = useConfirmDialog()
let exitPending = false
const allowExit = async () => {
  if (!props.inlineCheckout || !props.visible) return true
  if (checkoutBusy.value || exitPending) return false
  exitPending = true
  try {
    return await confirmExit({
      title: t('quickBuy.exitTitle'),
      message: t(hasCreatedOrder.value ? 'quickBuy.exitPaymentMessage' : 'quickBuy.exitMessage'),
      confirmText: t('quickBuy.exitConfirm'),
      cancelText: t('quickBuy.exitContinue'),
    })
  } finally {
    exitPending = false
    if (props.visible) void nextTick(() => dialogPanel.value?.focus({ preventScroll: true }))
  }
}
const close = async () => {
  if (loginVisible.value) { loginVisible.value = false; return }
  if (checkoutBusy.value || !await allowExit()) return
  emit('update:visible', false)
}
const beforeUnload = (event: BeforeUnloadEvent) => {
  if (!props.inlineCheckout || !props.visible) return
  event.preventDefault()
  event.returnValue = ''
}
const removeExitGuard = router.beforeEach(() => allowExit())

const onKeydown = (e: KeyboardEvent) => {
  if (exitDialogVisible.value) return
  if (e.key === 'Escape') { e.preventDefault(); close() }
  if (e.key === 'Tab') {
    const nodes = Array.from(dialogPanel.value?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), a[href]') || []).filter(node => node.getClientRects().length)
    const first = nodes[0], last = nodes[nodes.length - 1]
    if (!first) { e.preventDefault(); return }
    if (e.shiftKey && (document.activeElement === first || document.activeElement === dialogPanel.value)) { e.preventDefault(); last?.focus() }
    else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus() }
  }
}

watch(
  () => props.visible,
  (isVisible) => {
    if (isVisible) {
      previousFocus = document.activeElement as HTMLElement | null
      previousOverflow = document.body.style.overflow
      document.body.style.overflow = 'hidden'
      document.addEventListener('keydown', onKeydown)
      window.addEventListener('beforeunload', beforeUnload)
      void nextTick(() => dialogPanel.value?.focus({ preventScroll: true }))
    } else {
      document.removeEventListener('keydown', onKeydown)
      window.removeEventListener('beforeunload', beforeUnload)
      if (previousOverflow !== undefined) document.body.style.overflow = previousOverflow
      previousOverflow = undefined
      previousFocus?.focus({ preventScroll: true })
    }
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  removeExitGuard()
  window.removeEventListener('beforeunload', beforeUnload)
  cardAnimation?.cancel()
  if (hiddenSource) hiddenSource.style.visibility = ''
  if (previousOverflow !== undefined) document.body.style.overflow = previousOverflow
  document.removeEventListener('keydown', onKeydown)
})

const handleAddToCart = () => {
  if (!product.value || !canPurchase.value) return
  purchaseWarning.value = ''

  const sku = selectedSku.value
  const available = skuAvailableStock(sku)
  const cartQty = selectedCartQuantity()
  const nextQuantity = cartQty + quantity.value
  const productLimit = normalizeOptionalLimitNumber(product.value?.max_purchase_quantity)
  let limit: number | null = productLimit
  if (available !== null) {
    limit = limit === null ? available : Math.min(limit, available)
  }
  if (limit !== null && nextQuantity > limit) {
    if (available !== null && limit === available && (productLimit === null || available <= productLimit)) {
      purchaseWarning.value = available > 0
        ? (cartQty > 0
            ? t('productDetail.addCartStockExceededWithCart', { count: available, cartCount: cartQty })
            : t('productDetail.addCartStockExceeded', { count: available }))
        : t('productDetail.stockUnavailable')
      return
    }
    purchaseWarning.value = cartQty > 0
      ? t('productDetail.addCartLimitExceededWithCart', { count: limit, cartCount: cartQty })
      : t('productDetail.addCartLimitExceeded', { count: limit })
    return
  }

  const images = getProductImages()
  cartStore.addItem({
    productId: product.value.id,
    skuId: normalizeSkuId(sku?.id),
    skuCode: String(sku?.sku_code || ''),
    skuSpecValues: (sku?.spec_values && typeof sku.spec_values === 'object') ? sku.spec_values : undefined,
    skuManualStockTotal: normalizeManualStockTotal(sku?.manual_stock_total),
    skuManualStockLocked: normalizeStockNumber(sku?.manual_stock_locked),
    skuManualStockSold: normalizeStockNumber(sku?.manual_stock_sold),
    skuAutoStockAvailable: normalizeStockNumber(sku?.auto_stock_available),
    skuUpstreamStock: normalizeManualStockTotal(sku?.upstream_stock),
    skuStockStatus: String(sku?.stock_status || ''),
    skuStockDisplayMode: String(sku?.stock_display_mode || product.value?.stock_display_mode || ''),
    skuStockDisplay: String(sku?.stock_display || ''),
    skuStockRangeMin: normalizeStockNumber(sku?.stock_range_min) || undefined,
    skuStockRangeMax: normalizeStockNumber(sku?.stock_range_max) || undefined,
    skuStockQuantityHidden: Boolean(sku?.stock_quantity_hidden || product.value?.stock_quantity_hidden),
    skuStockEnforced: shouldEnforceSkuStock(sku),
    slug: product.value.slug,
    title: product.value.title,
    priceAmount: String(sku?.price_amount || product.value.price_amount || '0.00'),
    wholesalePrices: Array.isArray(product.value.wholesale_prices) ? product.value.wholesale_prices : undefined,
    image: images[0] || '',
    minPurchaseQuantity: normalizeOptionalLimitNumber(product.value.min_purchase_quantity) ?? undefined,
    maxPurchaseQuantity: normalizeOptionalLimitNumber(product.value.max_purchase_quantity) ?? undefined,
    purchaseType: product.value.purchase_type,
    fulfillmentType: product.value.fulfillment_type,
    manualFormSchema: product.value.manual_form_schema || {},
    paymentChannelIds: Array.isArray(product.value.payment_channel_ids) && product.value.payment_channel_ids.length > 0 ? product.value.payment_channel_ids : undefined,
    quantity: 1,
  }, quantity.value)
  toast.success(t('toast.addedToCart'))
  close()
}

const prepareBuyNow = () => {
  purchaseWarning.value = ''
  if (!canPurchase.value) return
  if (!product.value) return

  const sku = selectedSku.value
  const available = skuAvailableStock(sku)
  const productLimit = normalizeOptionalLimitNumber(product.value?.max_purchase_quantity)
  let limit: number | null = productLimit
  if (available !== null) {
    limit = limit === null ? available : Math.min(limit, available)
  }
  if (limit !== null && quantity.value > limit) {
    purchaseWarning.value = available !== null && limit === available && (productLimit === null || available <= productLimit)
      ? (available > 0 ? t('productDetail.addCartStockExceeded', { count: available }) : t('productDetail.stockUnavailable'))
      : t('productDetail.addCartLimitExceeded', { count: limit })
    return
  }

  const images = getProductImages()
  buyNowStore.setItem({
    productId: product.value.id,
    skuId: normalizeSkuId(sku?.id),
    skuCode: String(sku?.sku_code || ''),
    skuSpecValues: (sku?.spec_values && typeof sku.spec_values === 'object') ? sku.spec_values : undefined,
    skuManualStockTotal: normalizeManualStockTotal(sku?.manual_stock_total),
    skuManualStockLocked: normalizeStockNumber(sku?.manual_stock_locked),
    skuManualStockSold: normalizeStockNumber(sku?.manual_stock_sold),
    skuAutoStockAvailable: normalizeStockNumber(sku?.auto_stock_available),
    skuUpstreamStock: normalizeManualStockTotal(sku?.upstream_stock),
    skuStockStatus: String(sku?.stock_status || ''),
    skuStockDisplayMode: String(sku?.stock_display_mode || product.value?.stock_display_mode || ''),
    skuStockDisplay: String(sku?.stock_display || ''),
    skuStockRangeMin: normalizeStockNumber(sku?.stock_range_min) || undefined,
    skuStockRangeMax: normalizeStockNumber(sku?.stock_range_max) || undefined,
    skuStockQuantityHidden: Boolean(sku?.stock_quantity_hidden || product.value?.stock_quantity_hidden),
    skuStockEnforced: shouldEnforceSkuStock(sku),
    slug: product.value.slug,
    title: product.value.title,
    priceAmount: String(sku?.price_amount || product.value.price_amount || '0.00'),
    wholesalePrices: Array.isArray(product.value.wholesale_prices) ? product.value.wholesale_prices : undefined,
    image: images[0] || '',
    minPurchaseQuantity: normalizeOptionalLimitNumber(product.value.min_purchase_quantity) ?? undefined,
    maxPurchaseQuantity: normalizeOptionalLimitNumber(product.value.max_purchase_quantity) ?? undefined,
    purchaseType: product.value.purchase_type,
    fulfillmentType: product.value.fulfillment_type,
    manualFormSchema: product.value.manual_form_schema || {},
    paymentChannelIds: Array.isArray(product.value.payment_channel_ids) && product.value.payment_channel_ids.length > 0 ? product.value.payment_channel_ids : undefined,
    quantity: quantity.value,
  })
  return true
}
const handleBuyNow = () => {
  if (!prepareBuyNow()) return
  close()
  router.push('/checkout?mode=buynow')
}

watch(() => [props.visible, product.value, selectedSkuId.value, quantity.value], () => {
  if (props.inlineCheckout && props.visible) prepareBuyNow()
}, { immediate: true })

const goLogin = () => {
  if (props.inlineCheckout) { loginVisible.value = true; return }
  close()
  router.push(`/auth/login?redirect=${encodeURIComponent(route.fullPath)}`)
}

const goToDetail = () => {
  close()
  router.push(`/products/${product.value?.slug}`)
}

function getProductImages() {
  if (!product.value?.images) return []
  let imageArray: string[] = []
  if (Array.isArray(product.value.images)) {
    imageArray = product.value.images
  } else if (product.value.images.images && Array.isArray(product.value.images.images)) {
    imageArray = product.value.images.images
  }
  return imageArray.map((img: string) => getImageUrl(img))
}
</script>

<style scoped>
.quick-buy-checkout { background: var(--x-surface); color: var(--x-ink); }
.quick-buy-checkout__info { min-width: 0; }
.quick-buy-checkout__login { animation: checkout-login-in .32s cubic-bezier(.22,1,.36,1) both; overflow-y: auto; min-height: 0; padding: 24px; background: var(--x-bg); border-left: 1px solid var(--x-line); }
.quick-buy-checkout__product { margin-bottom: 24px; }
.quick-buy-checkout__product > div:first-child { width: 64px; height: 64px; border-radius: 16px; }
.quick-buy-checkout__product h3 { -webkit-line-clamp: unset; font-size: 22px; line-height: 1.4; }
.quick-buy-checkout__quantity { justify-content: space-between; margin: 0; padding-block: 16px; border-block: 1px solid var(--x-line); }
.quick-buy-checkout__description { padding-block: 12px; margin-bottom: 16px; color: var(--x-ink-2); font-size: 14px; line-height: 1.9; overflow-wrap: anywhere; }
.quick-buy-checkout__guide { color: var(--x-ink-2); font-size: 14px; line-height: 1.9; }
.quick-buy-checkout__guide h4 { color: var(--x-ink); margin: 24px 0 10px; font-weight: 650; }
.quick-buy-checkout__guide ul { padding-left: 20px; list-style: disc; }
.quick-buy-checkout__guide li { margin-bottom: 12px; }
.quick-buy-checkout__guide a { display: inline-block; color: var(--x-blue); margin-top: 16px; }
.quick-buy-checkout__description :deep(img) { max-width: 100%; height: auto; }
.quick-buy-checkout__description :deep(table) { max-width: 100%; display: block; overflow-x: auto; }
@media (min-width: 768px) {
  .quick-buy-checkout { max-width: 1120px; height: min(780px, 90dvh); max-height: 90dvh; }
  .quick-buy-checkout__columns { display: grid; grid-template-columns: minmax(0, 1.25fr) minmax(340px, 1fr); padding: 0; overflow: hidden; min-height: 0; }
  .quick-buy-checkout__info { overflow-y: auto; overscroll-behavior: contain; padding: 28px; }
  .quick-buy-checkout__columns > :deep(.xck:has(.inline-pay)) { overflow: hidden; }
  .quick-buy-checkout__columns > :deep(.xck) { overflow-y: auto; overscroll-behavior: contain; padding: 24px; border-left: 1px solid var(--x-line); background: var(--x-bg); }
}
@media (prefers-reduced-motion: reduce) { .quick-buy-enter-active, .quick-buy-leave-active { transition: none !important; } }
@keyframes checkout-login-in { from { opacity: 0; transform: translateX(12px); } to { opacity: 1; transform: translateX(0); } }
@media(prefers-reduced-motion: reduce) { .quick-buy-checkout__login { animation: none; } }
/* Mobile: slide up from bottom */
.quick-buy-enter-active {
  transition: transform 280ms cubic-bezier(0.32, 0.72, 0, 1), opacity 280ms ease-out;
}
.quick-buy-leave-active {
  transition: transform 200ms ease-in, opacity 200ms ease-in;
}
.quick-buy-enter-from {
  transform: translateY(100%);
  opacity: 1;
}
.quick-buy-enter-to {
  transform: translateY(0);
  opacity: 1;
}
.quick-buy-leave-from {
  transform: translateY(0);
  opacity: 1;
}
.quick-buy-leave-to {
  transform: translateY(100%);
  opacity: 1;
}

/* Desktop: centered scale + fade */
@media (min-width: 768px) {
  .quick-buy-enter-active {
    transition: transform 420ms cubic-bezier(.22,1,.36,1), opacity 240ms ease-out;
  }
  .quick-buy-leave-active {
    transition: transform 150ms ease-in, opacity 150ms ease-in;
  }
  .quick-buy-enter-from {
    transform: scale(0.96);
    opacity: 0;
  }
  .quick-buy-enter-to {
    transform: scale(1);
    opacity: 1;
  }
  .quick-buy-leave-from {
    transform: scale(1);
    opacity: 1;
  }
  .quick-buy-leave-to {
    transform: scale(0.96);
    opacity: 0;
  }
}
</style>

<style scoped>
@media(max-width:767px) {
 .quick-buy-checkout--paying { height:90dvh;max-height:90dvh; }
 .quick-buy-checkout--paying .quick-buy-checkout__columns { display:flex;flex-direction:column;min-height:0;overflow:auto;padding:0 16px; }
 .quick-buy-checkout--paying .quick-buy-checkout__info { flex:none; }
 .quick-buy-checkout--paying .quick-buy-checkout__info > :not(.quick-buy-checkout__product) { display:none; }
 .quick-buy-checkout--paying .quick-buy-checkout__product { margin-bottom:6px;padding-bottom:6px;border-bottom:1px solid var(--x-line); }
 .quick-buy-checkout--paying .quick-buy-checkout__product h3 { font-size:16px; }
 .quick-buy-checkout--paying .quick-buy-checkout__columns > :deep(.xck),
 .quick-buy-checkout--paying .quick-buy-checkout__columns > :deep(.resume-payment) { flex:1;min-height:530px;overflow:visible; }
 .quick-buy-checkout--paying :deep(.xck__payment-panel) { inset:12px 0; }
}
</style>
