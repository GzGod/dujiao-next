<template>
  <div class="xck">
    <!-- 面包屑 -->
    <nav class="x-container xck__crumb" :aria-label="t('checkout.title')">
      <RouterLink to="/">{{ t('nav.home') }}</RouterLink>
      <span>/</span>
      <RouterLink v-if="!isBuyNowMode" to="/cart">{{ t('cart.title') }}</RouterLink>
      <span v-if="!isBuyNowMode">/</span>
      <b>{{ t('checkout.title') }}</b>
    </nav>

    <div class="x-container xck__head">
      <h1>{{ t('checkout.title') }}</h1>
      <p class="xck__sub">{{ t('checkout.subtitle') }}</p>
    </div>

    <!-- 步骤条 -->
    <div class="x-container xck__steps">
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
        <section class="xck__card">
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
        <section v-if="!isResellerTenant" class="xck__card">
          <h2>{{ t('checkout.couponTitle') }}</h2>
          <input
            v-model="couponCode"
            type="text"
            class="xck__input"
            :placeholder="t('checkout.couponPlaceholder')"
          />
        </section>

        <!-- 购买方式（未登录） -->
        <section v-if="!userAuthStore.isAuthenticated" class="xck__card">
          <h2>{{ t('checkout.modeTitle') }}</h2>
          <div class="xck__mode-tabs">
            <button
              type="button"
              :class="{ 'is-active': checkoutMode === 'guest' }"
              @click="checkoutMode = 'guest'"
            >{{ t('checkout.guestPurchase') }}</button>
            <RouterLink to="/auth/login" class="xck__mode-login">{{ t('checkout.memberPurchase') }}</RouterLink>
          </div>

          <template v-if="checkoutMode === 'guest'">
            <div class="xck__guest">
              <input v-model="guestEmail" type="email" class="xck__input" :placeholder="t('checkout.guestEmailPlaceholder')" />
              <input v-model="guestPassword" type="password" class="xck__input" :placeholder="t('checkout.guestPasswordPlaceholder')" />
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
            <div class="xck__guest-note">
              <p><b>{{ t('checkout.guestInstructions.title') }}</b></p>
              <ul>
                <li>{{ t('checkout.guestInstructions.email') }}</li>
                <li>{{ t('checkout.guestInstructions.password') }}</li>
              </ul>
            </div>
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
            <div><span>{{ t('cart.itemsCount') }}</span><span>{{ totalItems }}</span></div>
            <div><span>{{ t('checkout.previewOriginal') }}</span><span>{{ formatPrice(previewOriginal, previewCurrency) }}</span></div>
            <template v-if="!isResellerTenant">
              <div><span>{{ t('checkout.previewCoupon') }}</span><span :class="{ 'is-blue': hasPositiveAmount(previewCoupon) }">{{ formatDiscountPrice(previewCoupon, previewCurrency) }}</span></div>
              <div><span>{{ t('checkout.previewPromotion') }}</span><span :class="{ 'is-blue': hasPositiveAmount(previewPromotion) }">{{ formatDiscountPrice(previewPromotion, previewCurrency) }}</span></div>
              <div><span>{{ t('checkout.previewWholesale') }}</span><span :class="{ 'is-green': hasPositiveAmount(previewWholesale) }">{{ formatDiscountPrice(previewWholesale, previewCurrency) }}</span></div>
            </template>
            <div v-if="Number(previewMemberDiscount) > 0"><span>{{ t('checkout.previewMemberDiscount') }}</span><span class="is-gold">-{{ formatPrice(previewMemberDiscount, previewCurrency) }}</span></div>
            <div class="is-total"><span>{{ t('checkout.previewTotal') }}</span><span>{{ formatPrice(previewTotal, previewCurrency) }}</span></div>
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
                  :class="{ 'is-active': selectedChannelId === channel.id && !isChannelDisabledForAmount(channel), 'is-disabled': isChannelDisabledForAmount(channel) }"
                  :disabled="isChannelDisabledForAmount(channel)"
                  :title="isChannelDisabledForAmount(channel) ? channelAmountLimitHint(channel) : ''"
                  @click="handleSelectChannel(channel)"
                >
                  <img v-if="channel.icon" :src="getImageUrl(channel.icon)" loading="lazy" alt="" />
                  <span>{{ channel.name }}</span>
                  <small v-if="channel.fee_policy === 'customer_surcharge'" class="is-warn">{{ t('payment.feeLabel') }}：{{ formatChannelFeeRate(channel) }}</small>
                  <small v-if="isChannelDisabledForAmount(channel)" class="is-warn">{{ channelAmountLimitHint(channel) }}</small>
                </button>
              </div>
              <div v-else-if="requiresOnlineChannel && paymentChannels.length === 0" class="xck__channels-empty">{{ t('checkout.noPaymentChannels') }}</div>
            </template>
            <div v-if="!requiresOnlineChannel" class="xck__wallet-covers">{{ t('checkout.walletCoversAll') }}</div>
          </div>

          <button
            type="button"
            class="x-btn x-btn--primary x-btn--lg x-btn--block"
            :disabled="!canSubmit"
            @click="handleSubmit"
          >
            {{ submitting ? t('checkout.submitting') : t('checkout.submitButton') }}
          </button>
        </div>
      </aside>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Package, ShoppingCart } from 'lucide-vue-next'
import ImageCaptcha from '../../components/captcha/ImageCaptcha.vue'
import TurnstileCaptcha from '../../components/captcha/TurnstileCaptcha.vue'
import CheckoutManualForm from '../../components/checkout/CheckoutManualForm.vue'
import XgaiCheckoutSteps from './components/XgaiCheckoutSteps.vue'
import { useCheckout } from '../../composables/useCheckout'
import './styles/xgai-checkout.css'

const { t } = useI18n()

const {
  userAuthStore, getLocalizedText, formatPrice, getImageUrl,
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
} = useCheckout()

void guestImageCaptchaRef
void guestTurnstileRef
void formatChannelFixedFee
</script>
