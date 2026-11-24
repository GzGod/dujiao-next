<template>
  <div class="xck" :class="{ 'xck--embedded': embedded }">
    <canvas v-if="paymentStage === 'loading' || gathering" ref="dustCanvas" class="xck__dust" aria-hidden="true" />

    <div v-if="paymentStage === 'payment'" ref="paymentPanel" class="xck__payment-panel" :inert="gathering" :class="{ 'xck__gathering': gathering }"><InlinePayment @back="returnToForm" @ready="gatherPayment" v-if="paymentStage === 'payment'" :result="createdPayment" :channels="paymentChannels" :initial-channel-id="selectedChannelId" :guest="createdAsGuest" :email="guestEmail" :password="guestPassword" /></div>
    <div ref="purchaseForm" :class="{ 'xck__form--dissolved': paymentStage !== 'form' }" :inert="paymentStage !== 'form'">
    <!-- 面包屑 -->
    <nav v-if="!embedded" class="x-container xck__crumb" :aria-label="t('checkout.title')">
      <RouterLink to="/">{{ t('nav.home') }}</RouterLink>
      <span>/</span>
      <RouterLink v-if="!isBuyNowMode" to="/cart">{{ t('cart.title') }}</RouterLink>
      <span v-if="!isBuyNowMode">/</span>
      <b>{{ t('checkout.title') }}</b>
    </nav>

    <div v-if="!embedded" class="x-container xck__head">
      <h1>{{ t('checkout.title') }}</h1>
      <p class="xck__sub">{{ t('checkout.subtitle') }}</p>
    </div>

    <!-- 步骤条 -->
    <div v-if="!embedded" class="x-container xck__steps">
      <XgaiCheckoutSteps current="checkout" :skip-cart="isBuyNowMode" />
    </div>

    <!-- 空车 -->
    <div v-if="cartItems.length === 0" class="x-container xck__empty">
      <ShoppingCart />
      <p>{{ t('checkout.empty') }}</p>
      <RouterLink to="/products" class="x-btn x-btn--primary">{{ t('checkout.emptyAction') }}</RouterLink>
    </div>

    <div v-else class="x-container xck__layout">
      <!-- 左栏 -->
      <div class="xck__main">
        <!-- 商品列表 -->
        <section v-if="!embedded" class="xck__card">
          <h2>{{ t('checkout.itemsTitle') }}</h2>
          <div class="xck__items">
            <div
              v-for="item in cartItems"
              :key="cartItemKey(item)"
              class="xck__item"
              :class="{ 'is-warn': itemStockExceeded(item) }"
            >
              <div class="xck__thumb">
                <img v-if="checkoutItemImage(item)" :src="checkoutItemImage(item)" :alt="getLocalizedText(item.title)" loading="lazy" />
                <Package v-else />
              </div>
              <div class="xck__item-info">
                <RouterLink :to="`/products/${item.slug}`" class="x-link">{{ getLocalizedText(item.title) }}</RouterLink>
                <small>{{ t('checkout.quantityLabel') }}：{{ item.quantity }}</small>
                <small v-if="itemSkuDisplay(item)">{{ t('checkout.skuLabel') }}：{{ itemSkuDisplay(item) }}</small>
                <small v-if="itemStockHint(item)" :class="{ 'is-warn': itemStockExceeded(item) }">{{ itemStockHint(item) }}</small>
              </div>
              <div class="xck__item-price" :class="{ 'is-active': checkoutItemHasPriceDiscount(item) }">
                <strong>{{ checkoutItemPriceParts(item).integer }}{{ checkoutItemPriceParts(item).decimal }}</strong>
                <small>{{ checkoutItemCurrency }}</small>
                <del v-if="checkoutItemHasPriceDiscount(item)">{{ checkoutItemOriginalPriceParts(item).integer }}{{ checkoutItemOriginalPriceParts(item).decimal }} {{ checkoutItemCurrency }}</del>
              </div>
            </div>
          </div>
        </section>

        <!-- 自定义表单 -->
        <CheckoutManualForm
          v-if="manualFormProducts.length"
          :manual-form-products="manualFormProducts"
          v-model="manualFormData"
          :submit-attempted="submitAttempted"
          :get-manual-field-label="getManualFieldLabel"
          :get-manual-field-placeholder="getManualFieldPlaceholder"
          :manual-field-error="manualFieldError"
        />

        <!-- 优惠码 -->
        <section v-if="!isResellerTenant" class="xck__card" :class="{ 'xck__coupon--compact': embedded }">
          <h2>{{ t('checkout.couponTitle') }}</h2>
          <input
            v-model="couponCode"
            type="text"
            class="xck__input"
            :placeholder="t('checkout.couponPlaceholder')"
          />
        </section>

        <!-- 购买方式（未登录） -->
        <section v-if="!userAuthStore.isAuthenticated" class="xck__card" :class="{ 'xck__purchase-mode': embedded }">
          <h2>{{ t('checkout.modeTitle') }}</h2>
          <div class="xck__mode-tabs">
            <button
              type="button"
              :class="{ 'is-active': checkoutMode === 'guest' }"
              @click="checkoutMode = 'guest'"
            >{{ t('checkout.guestPurchase') }}</button>
            <button v-if="embedded" type="button" class="xck__mode-login" @click="emit('login')">{{ t('checkout.memberPurchase') }}</button>
            <RouterLink v-else to="/auth/login" class="xck__mode-login">{{ t('checkout.memberPurchase') }}</RouterLink>
          </div>

          <template v-if="checkoutMode === 'guest'">
            <div class="xck__guest">
              <input v-model="guestEmail" type="email" :disabled="submitting" class="xck__input" :placeholder="t('checkout.guestEmailPlaceholder')" />
              <input v-model="guestPassword" type="password" :disabled="submitting" class="xck__input" :placeholder="t('checkout.guestPasswordPlaceholder')" />
            </div>
            <div v-if="guestCaptchaEnabled" class="xck__captcha">
              <ImageCaptcha
                v-if="captchaProvider === 'image'"
                ref="guestImageCaptchaRef"
                v-model="guestCaptchaPayload"
                :disabled="submitting"
                @config-stale="handleGuestCaptchaConfigStale"
              />
              <TurnstileCaptcha
                v-else-if="captchaProvider === 'turnstile'"
                ref="guestTurnstileRef"
                v-model="guestTurnstileToken"
                :site-key="guestTurnstileSiteKey"
              />
            </div>
            <component :is="embedded ? 'details' : 'div'" class="xck__guest-note">
              <summary v-if="embedded">{{ t('checkout.guestInstructions.title') }}</summary>
              <p v-else><b>{{ t('checkout.guestInstructions.title') }}</b></p>
              <ul>
                <li>{{ t('checkout.guestInstructions.email') }}</li>
                <li>{{ t('checkout.guestInstructions.password') }}</li>
              </ul>
            </component>
            <p v-if="guestEmail && !guestEmailValid" class="xck__field-error">{{ t('error.email_invalid') }}</p>
          </template>
        </section>
      </div>

      <!-- 右栏：汇总 + 支付方式 + 提交 -->
      <aside class="xck__aside">
        <div class="xck__card xck__summary">
          <h2>{{ t('checkout.submitTitle') }}</h2>
          <p class="xck__summary-hint">{{ t('checkout.submitHint') }}</p>

          <div class="xck__rows">
            <div v-if="!embedded"><span>{{ t('cart.itemsCount') }}</span><span>{{ totalItems }}</span></div>
            <div v-if="!embedded"><span>{{ t('checkout.previewOriginal') }}</span><span>{{ formatPrice(previewOriginal, previewCurrency) }}</span></div>
            <template v-if="!isResellerTenant">
              <div v-if="!embedded || hasPositiveAmount(previewCoupon)"><span>{{ t('checkout.previewCoupon') }}</span><span :class="{ 'is-blue': hasPositiveAmount(previewCoupon) }">{{ formatDiscountPrice(previewCoupon, previewCurrency) }}</span></div>
              <div v-if="!embedded || hasPositiveAmount(previewPromotion)"><span>{{ t('checkout.previewPromotion') }}</span><span :class="{ 'is-blue': hasPositiveAmount(previewPromotion) }">{{ formatDiscountPrice(previewPromotion, previewCurrency) }}</span></div>
              <div v-if="!embedded || hasPositiveAmount(previewWholesale)"><span>{{ t('checkout.previewWholesale') }}</span><span :class="{ 'is-green': hasPositiveAmount(previewWholesale) }">{{ formatDiscountPrice(previewWholesale, previewCurrency) }}</span></div>
            </template>
            <div v-if="Number(previewMemberDiscount) > 0"><span>{{ t('checkout.previewMemberDiscount') }}</span><span class="is-gold">-{{ formatPrice(previewMemberDiscount, previewCurrency) }}</span></div>
            <div class="is-total"><span>{{ t('checkout.previewTotal') }}</span><span>{{ formatPrice(previewTotal, previewCurrency) }}</span><slot name="quantity" /></div>
          </div>

          <div v-if="previewLoading || couponRefreshing" class="xck__preview-status">{{ previewStatusText }}</div>
          <div v-if="checkoutAlert" class="xck__alert" :class="checkoutAlert.level === 'error' ? 'is-error' : 'is-warn'">{{ checkoutAlert.message }}</div>

          <!-- 支付方式 -->
          <div class="xck__pay-section">
            <h3>{{ t('checkout.paymentMethod') }}</h3>

            <!-- 钱包余额 -->
            <div v-if="showBalanceOption" class="xck__wallet">
              <div>
                <small>{{ t('payment.walletBalanceLabel') }}</small>
                <b>{{ walletLoading ? t('common.loading') : formatPrice(walletBalance, previewCurrency) }}</b>
              </div>
              <label>
                <input v-model="useBalance" type="checkbox" :disabled="walletOnlyPayment" />
                {{ t('payment.useBalance') }}
              </label>
              <div v-if="walletOnlyPayment" class="is-warn">{{ t('payment.walletOnlyHint') }}</div>
              <div v-if="useBalance" class="xck__wallet-detail">
                <div>{{ t('payment.walletDeductLabel') }}：{{ expectedWalletPaidDisplay }}</div>
                <div v-if="!walletOnlyPayment">{{ t('payment.onlinePayLabel') }}：{{ expectedOnlinePayDisplay }}</div>
                <div v-if="walletOnlyPayment && expectedOnlinePayCents > 0" class="is-warn">{{ t('payment.walletInsufficientHint') }}</div>
              </div>
            </div>

            <template v-if="!walletOnlyPayment">
              <div v-if="requiresOnlineChannel && paymentChannels.length > 0" class="xck__channels">
                <button
                  v-for="channel in paymentChannels"
                  :key="channel.id"
                  type="button"
                  :aria-label="channel.name"
                  :aria-pressed="selectedChannelId === channel.id"
                  :class="{ 'is-active': selectedChannelId === channel.id && !isChannelDisabledForAmount(channel), 'is-disabled': isChannelDisabledForAmount(channel) }"
                  :disabled="isChannelDisabledForAmount(channel)"
                  :title="isChannelDisabledForAmount(channel) ? channelAmountLimitHint(channel) : channel.name"
                  @click="handleSelectChannel(channel)"
                >
                  <template v-if="embedded && String(channel.channel_type).includes('alipay')">
                    <img src="/images/payments/alipay.ico" alt="" /><span class="xck__channel-name">{{ channel.name }}</span>
                  </template>
                  <span v-else-if="embedded && /usdt|usdc|crypto|epusdt|bepusdt|tokenpay/i.test([channel.name, channel.provider_type, channel.channel_type].join(' '))" class="xck__coin-pair" aria-hidden="true"><img src="/images/payments/usdt.svg" alt="" /><span>/</span><img src="/images/payments/usdc.svg" alt="" /><b class="xck__coin-label">USDT / USDC</b></span>
                  <template v-else><img v-if="channel.icon" :src="getImageUrl(channel.icon)" alt="" /><span>{{ channel.name }}</span></template>
                  <small v-if="channel.fee_policy === 'customer_surcharge'" class="is-warn">{{ t('payment.feeLabel') }}：{{ formatChannelFeeRate(channel) }}</small>
                  <small v-if="isChannelDisabledForAmount(channel)" class="is-warn">{{ channelAmountLimitHint(channel) }}</small>
                </button>
              </div>
              <div v-else-if="requiresOnlineChannel && paymentChannels.length === 0" class="xck__channels-empty">{{ t('checkout.noPaymentChannels') }}</div>
            </template>
            <div v-if="!requiresOnlineChannel" class="xck__wallet-covers">{{ t('checkout.walletCoversAll') }}</div>
          </div>

          <label class="xck__agreement">
            <input v-model="agreementAccepted" type="checkbox" :disabled="submitting" />
            <i18n-t keypath="checkout.agreement" tag="span">
              <template #terms><RouterLink to="/terms" target="_blank" rel="noopener noreferrer">{{ t('checkout.agreementTerms') }}</RouterLink></template>
              <template #privacy><RouterLink to="/privacy" target="_blank" rel="noopener noreferrer">{{ t('checkout.agreementPrivacy') }}</RouterLink></template>
            </i18n-t>
          </label>
          <button
            type="button"
            class="x-btn x-btn--primary x-btn--lg x-btn--block"
            :disabled="!canSubmit"
            @click="submitWithMotion"
          >
            {{ submitting ? t('checkout.submitting') : t('checkout.submitButton') }}
          </button>
        </div>
      </aside>
    </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { watch, ref, nextTick, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { Package, ShoppingCart } from 'lucide-vue-next'
import ImageCaptcha from '../../components/captcha/ImageCaptcha.vue'
import TurnstileCaptcha from '../../components/captcha/TurnstileCaptcha.vue'
import CheckoutManualForm from '../../components/checkout/CheckoutManualForm.vue'
import InlinePayment from './components/InlinePayment.vue'
import { dissolvePaymentPanel } from '../../utils/paymentDust'
import XgaiCheckoutSteps from './components/XgaiCheckoutSteps.vue'
import { useCheckout } from '../../composables/useCheckout'
import './styles/xgai-checkout.css'

const props = defineProps<{ embedded?: boolean }>()
const emit = defineEmits<{ busy: [value: boolean]; login: []; payment: [value: boolean] }>()
const { t } = useI18n()
const purchaseForm = ref<HTMLElement | null>(null)
const dustCanvas = ref<HTMLCanvasElement | null>(null)
const paymentPanel = ref<HTMLElement | null>(null)
const gathering = ref(false)
let transitionVersion = 0
let gatherTimer: ReturnType<typeof setTimeout> | undefined
const gatherPayment = async () => {
  if (!gathering.value || disposed) return
  const version = transitionVersion
  clearTimeout(gatherTimer)
  await nextTick()
  paymentPanel.value?.classList.add('xck__payment-emerging')
  if (paymentPanel.value && dustCanvas.value && dust) { await dust.reverse(paymentPanel.value) }
  if (disposed || version !== transitionVersion) return
  gatherTimer = setTimeout(() => { dust?.stop(); gathering.value = false }, 50)
}
const returnToForm = () => {
  transitionVersion++
  dust?.stop(); clearTimeout(gatherTimer); gathering.value = false; paymentStage.value = 'form'; emit('payment', false)
}
const paymentStage = ref<'form' | 'loading' | 'payment'>('form')
const createdPayment = ref<any>(null)
const createdAsGuest = ref(false)
let createdFingerprint = ''
const orderFingerprint = () => JSON.stringify({ email: guestEmail.value.trim(), password: guestPassword.value, coupon: couponCode.value, items: cartItems.value, manual: manualFormData.value })
let disposed = false
let dust: import('../../utils/paymentDust').PaymentDust | undefined
let animationDone = Promise.resolve()
const onOrderCreated = async (result: any, guest: boolean) => {
  createdFingerprint = orderFingerprint()
  createdPayment.value = result
  createdAsGuest.value = guest
  await animationDone
  if (!disposed) { gathering.value = !window.matchMedia('(prefers-reduced-motion: reduce)').matches; paymentStage.value = 'payment'; emit('payment', true) }
}
const submitWithMotion = async () => {
  if (!props.embedded) return handleSubmit()
  if (!canSubmit.value) return
  transitionVersion++
  paymentStage.value = 'loading'
  animationDone = new Promise<void>(resolve => setTimeout(resolve, window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 0 : 1400))
  await nextTick()
  if (purchaseForm.value && dustCanvas.value && !window.matchMedia('(prefers-reduced-motion: reduce)').matches) dust = dissolvePaymentPanel(purchaseForm.value, dustCanvas.value)
  await handleSubmit()
  await animationDone
  if (!disposed && paymentStage.value === 'loading') { dust?.stop(); paymentStage.value = 'form' }
}
onBeforeUnmount(() => { disposed = true; clearTimeout(gatherTimer); dust?.stop() })

const {
  agreementAccepted, userAuthStore, getLocalizedText, formatPrice, getImageUrl,
  isBuyNowMode, cartItems, totalItems, cartItemKey, checkoutItemImage, itemSkuDisplay,
  itemStockExceeded, itemStockHint,
  checkoutItemCurrency, checkoutItemPriceParts, checkoutItemOriginalPriceParts, checkoutItemHasPriceDiscount,
  manualFormProducts, manualFormData, submitAttempted, getManualFieldLabel, getManualFieldPlaceholder, manualFieldError,
  couponCode, isResellerTenant,
  checkoutMode, guestEmail, guestPassword, guestEmailValid,
  guestCaptchaEnabled, captchaProvider, guestCaptchaPayload, guestTurnstileToken, guestTurnstileSiteKey,
  guestImageCaptchaRef, guestTurnstileRef, handleGuestCaptchaConfigStale,
  previewCurrency, previewOriginal, previewCoupon, previewPromotion, previewWholesale, previewMemberDiscount, previewTotal,
  previewLoading, couponRefreshing, previewStatusText, hasPositiveAmount, formatDiscountPrice, checkoutAlert,
  showBalanceOption, walletLoading, walletBalance, useBalance, walletOnlyPayment,
  expectedWalletPaidDisplay, expectedOnlinePayDisplay, expectedOnlinePayCents,
  requiresOnlineChannel, paymentChannels, selectedChannelId, isChannelDisabledForAmount, channelAmountLimitHint,
  handleSelectChannel, formatChannelFeeRate, formatChannelFixedFee,
  submitting, canSubmit, handleSubmit,
} = useCheckout({ buyNow: props.embedded, requireAgreement: true, existingOrder: () => createdFingerprint === orderFingerprint() ? createdPayment.value : null, onCreated: props.embedded ? onOrderCreated : undefined })

watch(submitting, value => emit('busy', value))

void guestImageCaptchaRef
void guestTurnstileRef
void formatChannelFixedFee
</script>

<style scoped>
.xck--embedded .x-container { width: 100%; max-width: none; padding-inline: 0; }
.xck--embedded .xck__layout { padding-bottom: 76px !important; display: flex; flex-direction: column; align-items: stretch; gap: 14px; padding-bottom: 0; }
.xck--embedded .xck__main, .xck--embedded .xck__aside, .xck--embedded .xck__summary { display: contents; }
.xck--embedded .xck__rows { order: 0; gap: 8px; }
.xck--embedded :deep(.quick-buy-checkout__quantity) { order: 1; }
.xck--embedded .xck__pay-section { order: 2; margin: 0; padding: 0; border: 0; }
.xck--embedded .xck__main > * { order: 3; }
.xck--embedded .xck__main > .xck__coupon--compact { order: 4; }
.xck--embedded .xck__summary > button { order: 6; }
.xck--embedded .xck__alert, .xck--embedded .xck__preview-status { order: 5; margin: 0; padding: 8px 10px; font-size: 12px; }
.xck--embedded .xck__card { padding: 0; border: 0; box-shadow: none; background: transparent; }
.xck--embedded .xck__summary > h2, .xck--embedded .xck__summary-hint { display: none; }
.xck--embedded .xck__rows .is-total { margin-top: 0; padding-top: 0; border-top: 0; }
.xck--embedded .xck__rows .is-total span:last-child { font-size: 32px; }
.xck--embedded .xck__guest { grid-template-columns: minmax(0,1fr) minmax(0,1fr); gap: 8px; }
.xck--embedded .xck__guest-note { font-size: 11px; margin-top: 8px; }
.xck--embedded .xck__card h2, .xck--embedded .xck__pay-section h3 { margin-bottom: 10px; font-size: 14px; }
.xck--embedded .xck__channels { grid-template-columns: repeat(2, minmax(0,1fr)); gap: 8px; }
.xck--embedded .xck__channels button { min-height: 44px; padding: 8px 10px; gap: 8px; flex-wrap: nowrap; }
.xck--embedded .xck__rows > div:not(.is-total) { font-size: 12px; }
.xck--embedded .xck__rows > .is-total { display: flex; flex-wrap: wrap; align-items: center; justify-content: flex-start; gap: 8px 12px; }
.xck--embedded .xck__rows > .is-total > span:first-child { flex-basis: 100%; font-size: 12px; }
.xck--embedded .xck__rows > .is-total > span:nth-child(2) { font-size: 34px; color: var(--x-blue); font-weight: 700; letter-spacing: -.04em; }
.xck--embedded :deep(.quick-buy-checkout__quantity) { margin-left: auto; padding: 0; border: 0; gap: 8px; }
.xck--embedded :deep(.quick-buy-checkout__quantity > span) { display: none; }
.xck--embedded :deep(.quick-buy-checkout__quantity button) { width: 28px; height: 30px; }
.xck--embedded :deep(.quick-buy-checkout__quantity input) { width: 32px; height: 30px; }
.xck--embedded .xck__input { height: 36px; padding: 0 10px; border-radius: 10px; font-size: 12px; min-width: 0; }
.xck--embedded .xck__mode-tabs { padding: 3px; margin-bottom: 10px; }
.xck--embedded .xck__mode-tabs button, .xck--embedded .xck__mode-login { padding: 5px 10px; font-size: 12px; }
.xck--embedded .xck__guest-note { padding: 6px 10px; }
.xck--embedded .xck__payment-icon { width: 28px; height: 28px; flex: 0 0 28px; display: grid; place-items: center; }
.xck__payment-icon svg { width: 28px; height: 28px; }
.xck__payment-icon.is-alipay { color: #1677ff; }
.xck__payment-icon.is-crypto { color: #26a17b; }
.xck--embedded .xck__channel-name { font-size: 11px; line-height: 1.4; }
.xck--embedded .xck__summary > button { position: absolute; left: 24px; right: 24px; width: calc(100% - 48px); bottom: 20px; z-index: 5; min-height: 42px; padding-block: 10px; box-shadow: 0 0 0 10px var(--x-bg); }
.xck__coin-pair { display: inline-flex; align-items: center; justify-content: center; gap: 12px; width: 100%; color: var(--x-ink-4); }
.xck--embedded .xck__channels img { width: 26px; height: 26px; }
.xck__agreement { display: flex; align-items: flex-start; gap: 8px; margin-block: 14px; font-size: 12px; line-height: 1.7; color: var(--x-ink-2); }
.xck__agreement input { flex: 0 0 16px; width: 16px; height: 16px; margin-top: 3px; accent-color: var(--x-blue); }
.xck__agreement a { color: var(--x-blue); }
.xck--embedded .xck__agreement { order: 5; margin: 0; }
.xck--embedded .xck__purchase-mode { display: grid; grid-template-columns: auto minmax(0, 1fr); align-items: center; gap: 10px; }
.xck--embedded .xck__purchase-mode > h2 { margin: 0; white-space: nowrap; }
.xck--embedded .xck__purchase-mode .xck__mode-tabs { margin: 0; justify-self: end; }
.xck--embedded .xck__purchase-mode > :not(h2):not(.xck__mode-tabs) { grid-column: 1 / -1; }
.xck--embedded .xck__channels { display: flex; flex-wrap: wrap; gap: 8px; }
.xck--embedded .xck__channels button { width: auto; min-height: 38px; padding: 7px 12px; border-radius: 10px; }
.xck--embedded .xck__channels img { width: 22px; height: 22px; }
.xck--embedded .xck__channel-name { font-size: 12px; }
.xck--embedded .xck__coin-pair { width: auto; gap: 6px; }
.xck__coin-label { margin-left: 3px; font-size: 12px; font-weight: 550; color: var(--x-ink); }
.xck { position: relative; }
.xck__form--dissolved { opacity: 0; pointer-events: none; }
.xck__dust { position: absolute; inset: 24px; width: calc(100% - 48px); height: calc(100% - 48px); z-index: 3; pointer-events: none; }
.xck__payment-loading { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 20px; color: var(--x-ink-3); font-size: 13px; animation: loading-emerge .5s .45s both; }
.xck__loading-orbit { width: 42px; height: 42px; border: 2px solid var(--x-line); border-top-color: var(--x-blue); border-radius: 50%; animation: pay-spin .9s linear infinite; }
.xck:has(.inline-pay) > .xck__form--dissolved { display: none; }
@keyframes pay-spin { to { transform: rotate(360deg); } }
@keyframes loading-emerge { from { opacity: 0; } to { opacity: 1; } }
@media(prefers-reduced-motion: reduce) { .xck__payment-loading { animation: none; } .xck__loading-orbit { animation: none; } }
.xck__gathering { opacity: 0; }
.xck__gathering.xck__payment-emerging { animation: payment-light-reveal 1.05s cubic-bezier(.22,1,.36,1) both; }
@keyframes payment-light-reveal { from { opacity: 1; clip-path: circle(0% at 50% 92%); filter: blur(3px); } to { opacity: 1; clip-path: circle(145% at 50% 92%); filter: blur(0); } }
@media(prefers-reduced-motion: reduce) { .xck__gathering.xck__payment-emerging { animation: none; opacity: 1; } }
.xck:has(.inline-pay) > [ref], .xck:has(.inline-pay) > .xck__gathering { height: 100%; }
.xck:has(.inline-pay) > div:has(> .inline-pay) { height: 100%; }
.xck__payment-panel { position: absolute; inset: 24px; height: auto !important; min-height: 0; }
</style>
