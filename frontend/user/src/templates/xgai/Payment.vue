<template>
  <div class="xpay">
    <!-- 面包屑 -->
    <nav class="x-container xpay__crumb">
      <RouterLink to="/">{{ t('nav.home') }}</RouterLink>
      <span>/</span>
      <RouterLink :to="backLink">{{ t('payment.backToOrders') }}</RouterLink>
      <span>/</span>
      <b>{{ t('payment.title') }}</b>
    </nav>

    <div class="x-container xpay__head">
      <div>
        <h1>{{ t('payment.title') }}</h1>
        <p class="xpay__sub">{{ t('payment.subtitle') }}</p>
      </div>
      <RouterLink :to="backLink" class="xpay__back">{{ t('payment.backToOrders') }}</RouterLink>
    </div>

    <div class="x-container xpay__steps">
      <XgaiCheckoutSteps current="payment" />
    </div>

    <!-- 加载 -->
    <div v-if="loading" class="x-container xpay__loading">
      <div class="xpay__sk" /><div class="xpay__sk xpay__sk--lg" />
    </div>

    <!-- 游客补凭证 -->
    <div v-else-if="showGuestAuthForm" class="x-container xpay__card">
      <h2>{{ t('payment.guestAuthTitle') }}</h2>
      <p class="xpay__card-sub">{{ t('payment.guestAuthHint') }}</p>
      <div class="xpay__guest">
        <input v-model="guestAuth.email" type="email" class="xck__input" :placeholder="t('guestOrders.emailPlaceholder')" />
        <input v-model="guestAuth.order_password" type="password" class="xck__input" :placeholder="t('guestOrders.passwordPlaceholder')" />
      </div>
      <div v-if="guestAuthError" class="xck__alert is-error">{{ guestAuthError }}</div>
      <button type="button" class="x-btn x-btn--primary" style="margin-top:16px" @click="handleGuestAuthSubmit">{{ t('payment.guestAuthSubmit') }}</button>
    </div>

    <!-- 订单不存在 -->
    <div v-else-if="!order" class="x-container xpay__missing">
      <AlertCircle />
      <p>{{ t('payment.orderNotFound') }}</p>
      <RouterLink :to="backLink" class="x-btn x-btn--ghost">{{ t('payment.backToOrders') }}</RouterLink>
    </div>

    <!-- 已支付 / 已发送结果视图 -->
    <template v-else-if="showResultView">
      <div class="x-container xpay__result">
        <div class="xpay__result-head">
          <div>
            <h2>{{ paymentResultTitle }}</h2>
            <p>{{ paymentGuideTip }}</p>
            <small>{{ t('payment.methodLabel') }}：{{ resultChannelName }}</small>
          </div>
          <div class="xpay__result-actions">
            <button type="button" class="x-btn x-btn--ghost" :disabled="loading" @click="handleRefresh">{{ t('payment.refreshStatus') }}</button>
            <button type="button" class="x-btn x-btn--ghost" @click="handleChangePaymentMethod">{{ t('payment.changeMethod') }}</button>
          </div>
        </div>

        <div class="xpay__result-body">
          <!-- 左：二维码 / x402 / 跳转链接 -->
          <div class="xpay__result-left">
            <!-- x402 钱包支付 -->
            <div v-if="isX402Payment" class="xpay__x402">
              <p class="xpay__x402-label">{{ t('payment.x402.title') }}</p>
              <p class="xpay__x402-amount">{{ x402AmountDisplay }} USDC</p>
              <small>{{ t('payment.x402.network') }}：{{ x402NetworkLabel }}</small>
              <button type="button" class="x-btn x-btn--primary" :disabled="x402Signing" @click="handleX402Pay">
                <Wallet />
                {{ x402Signing ? t('payment.x402.signing') : t('payment.x402.payWithWallet') }}
              </button>
              <p class="xpay__x402-tip">{{ t('payment.x402.tip') }}</p>
              <div v-if="x402Error" class="xck__alert is-error">{{ x402Error }}</div>
            </div>

            <!-- 二维码 -->
            <div v-else-if="showQRCode" class="xpay__qr">
              <p class="xpay__qr-label">{{ paymentGuideTitle }}</p>
              <div class="xpay__qr-img"><img :src="qrImageUrl" alt="QR Code" /></div>
              <p v-if="qrUsingPayLinkFallback" class="xpay__qr-note">{{ t('payment.qrFallbackHint') }}</p>
              <div v-if="hasCryptoPaymentDetails" class="xpay__crypto">
                <div v-for="item in cryptoPaymentDetails" :key="item.key" class="xpay__crypto-row">
                  <span>{{ item.label }}</span>
                  <b>{{ item.value }}<span v-if="item.detail"> ({{ item.detail }})</span></b>
                </div>
                <div v-if="cryptoWalletAddress" class="xpay__crypto-copy">
                  <button type="button" class="x-btn x-btn--ghost" @click="handleCopyWalletAddress">{{ t('payment.copyWalletAddress') }}</button>
                  <span v-if="walletAddressCopied" class="is-success">{{ t('payment.copied') }}</span>
                </div>
              </div>
            </div>

            <!-- 跳转链接 -->
            <div v-else class="xpay__link-pay">
              <p>{{ t('payment.openPayLink') }}</p>
              <button type="button" class="x-btn x-btn--primary" @click="handleOpenPayLink">
                <ExternalLink /> {{ t('payment.openPayLink') }}
              </button>
              <p v-if="openedPayWindow" class="is-success">{{ payLinkOpenedTip }}</p>
              <p v-if="showTelegramPayHint" class="xpay__note">{{ t('payment.telegramExternalHint') }}</p>
              <div class="xpay__copy-row">
                <button type="button" class="x-btn x-btn--ghost" @click="handleCopyPayLink">
                  <Copy /> {{ t('payment.copyPayLink') }}
                </button>
                <span v-if="copied" class="is-success">{{ t('payment.copied') }}</span>
              </div>
            </div>
          </div>

          <!-- 右：订单摘要 -->
          <div class="xpay__result-right">
            <div class="xpay__order-no">
              <small>{{ t('payment.orderNo') }}</small>
              <b>{{ order.order_no }}</b>
              <small>{{ t('payment.orderStatus') }}：{{ statusLabel(order.status) }}</small>
              <small>{{ t('payment.methodLabel') }}：{{ resultChannelName }}</small>
            </div>
            <PaymentAmountBreakdown
              :order="order"
              :payment-result="paymentResult"
              :customer-fee-applied="customerFeeApplied"
              :customer-fee-amount-display="customerFeeAmountDisplay"
              :payable-amount-display="payableAmountDisplay"
              :wallet-paid-display="paymentWalletPaidDisplay"
              :online-pay-display="paymentOnlinePayDisplay"
              :show-countdown="showCountdown"
              :countdown-text="countdownText"
              :polling-active="pollingActive"
              :format-money="formatMoney"
              :format-discount-money="formatDiscountMoney"
              :has-discount-amount="hasDiscountAmount"
            />
            <div v-if="paymentResult.expires_at" class="xpay__expires">{{ t('payment.expiresAt') }}：{{ formatDate(paymentResult.expires_at) }}</div>
          </div>
        </div>
      </div>
    </template>

    <!-- 过期 / 取消 -->
    <div v-else-if="orderExpired || orderCanceled" class="x-container xpay__card">
      <div class="xpay__status-head">
        <div>
          <h2>{{ orderCanceled ? t('payment.orderCanceled') : t('payment.orderExpired') }}</h2>
          <p>{{ order.order_no }}</p>
        </div>
        <RouterLink :to="backLink" class="x-btn x-btn--ghost">{{ t('payment.backToOrders') }}</RouterLink>
      </div>
      <div class="xpay__stat-row">
        <div><small>{{ t('payment.orderNo') }}</small><b>{{ order.order_no }}</b></div>
        <div><small>{{ t('payment.orderStatus') }}</small><b>{{ statusLabel(order.status) }}</b></div>
        <div><small>{{ t('orderDetail.amountTotal') }}</small><b>{{ formatMoney(order.total_amount, order.currency) }}</b></div>
      </div>
    </div>

    <!-- 正常支付流程 -->
    <div v-else class="x-container xpay__layout">
      <div class="xpay__main">
        <!-- 订单信息 -->
        <section class="xpay__card">
          <h2>{{ t('payment.orderInfo') }}</h2>
          <div class="xpay__order-info">
            <div>
              <small>{{ t('payment.orderNo') }}</small>
              <b>{{ order.order_no }}</b>
              <small>{{ t('orderDetail.createdAtLabel') }}：{{ formatDate(order.created_at) }}</small>
            </div>
            <div class="xpay__payable">
              <small>{{ t('payment.payableAmountLabel') }}</small>
              <strong>{{ payableAmountDisplay }}</strong>
              <div class="xpay__payable-rows">
                <div><span>{{ t('orderDetail.amountTotal') }}</span><span>{{ formatMoney(order.total_amount, order.currency) }}</span></div>
                <div v-if="showBalanceOption && useBalance"><span>{{ t('payment.walletDeductLabel') }}</span><span>{{ expectedWalletPaidDisplay }}</span></div>
                <div v-if="showBalanceOption && useBalance"><span>{{ t('payment.onlinePayLabel') }}</span><span>{{ expectedOnlinePayDisplay }}</span></div>
                <div><span>{{ t('payment.orderStatus') }}</span><span>{{ statusLabel(order.status) }}</span></div>
              </div>
            </div>
          </div>
          <div class="xpay__stat-row">
            <div><small>{{ t('orderDetail.amountOriginal') }}</small><b>{{ formatMoney(order.original_amount, order.currency) }}</b></div>
            <div><small>{{ t('orderDetail.amountDiscount') }}</small><b :class="{ 'is-red': hasDiscountAmount(order.discount_amount) }">{{ formatDiscountMoney(order.discount_amount, order.currency) }}</b></div>
            <div><small>{{ t('orderDetail.promotionDiscountLabel') }}</small><b :class="{ 'is-red': hasDiscountAmount(order.promotion_discount_amount) }">{{ formatDiscountMoney(order.promotion_discount_amount, order.currency) }}</b></div>
            <div v-if="hasDiscountAmount(order.wholesale_discount_amount)" class="is-teal"><small>{{ t('orderDetail.amountWholesaleDiscount') }}</small><b>{{ formatDiscountMoney(order.wholesale_discount_amount, order.currency) }}</b></div>
          </div>
          <div v-if="order.expires_at" class="xpay__expires">{{ t('payment.expiresAt') }}：{{ formatDate(order.expires_at) }}</div>
          <div v-if="showCountdown" class="xpay__countdown" :class="{ 'is-expired': countdownExpired }">
            {{ t('payment.countdownLabel') }} <span>{{ countdownText }}</span>
          </div>
          <div v-if="pollingActive" class="xpay__polling">{{ t('payment.pollingHint') }}</div>
        </section>

        <!-- 商品明细 -->
        <section v-if="orderItems.length" class="xpay__card">
          <h2>{{ t('payment.itemsTitle') }}</h2>
          <div class="xpay__items">
            <div v-for="(item, idx) in orderItems" :key="idx" class="xpay__item-row">
              <div>
                <b>{{ getLocalizedText(item.title) }}</b>
                <small>{{ t('orderDetail.quantityLabel') }}：{{ item.quantity }} · {{ t('orderDetail.itemFulfillmentLabel') }}：{{ fulfillmentTypeLabelText(item.fulfillment_type) }}</small>
                <small v-if="orderItemSkuText(item)">{{ t('orderDetail.itemSkuLabel') }}：{{ orderItemSkuText(item) }}</small>
              </div>
              <small>{{ t('orderDetail.totalPriceLabel') }}：{{ formatMoney(item.total_price, order.currency) }}</small>
            </div>
          </div>
        </section>

        <!-- 支付渠道 -->
        <section class="xpay__card">
          <h2>{{ t('payment.channelTitle') }}</h2>
          <div v-if="!configReady" class="xpay__loading-text">{{ t('common.loading') }}</div>
          <template v-else>
            <!-- 余额 -->
            <div v-if="showBalanceOption" class="xck__wallet">
              <div>
                <small>{{ t('payment.walletBalanceLabel') }}</small>
                <b>{{ walletLoading ? t('common.loading') : walletBalanceDisplay }}</b>
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

            <!-- 缓存支付 -->
            <div v-if="cachedPayment" class="xpay__cached">
              <b>{{ t('payment.cachedTitle') }}</b>
              <p>{{ t('payment.cachedHint', { channel: cachedChannelName }) }}</p>
              <button type="button" class="x-btn x-btn--ghost" @click="restoreCachedPayment">{{ t('payment.useCached') }}</button>
              <small>{{ t('payment.cachedCreateHint') }}</small>
            </div>

            <PaymentChannelSelector
              v-if="!walletOnlyPayment"
              :channels="channels"
              :model-value="selectedChannelId"
              :show-balance-option="showBalanceOption"
              :is-channel-disabled-for-amount="isChannelDisabledForAmount"
              :channel-amount-limit-hint="channelAmountLimitHint"
              @update:model-value="selectedChannelId = $event"
            />
          </template>
        </section>

        <!-- 支付信息（已生成未完成） -->
        <section v-if="paymentResult" class="xpay__card">
          <h2>{{ t('payment.infoTitle') }}</h2>
          <div class="xpay__info-rows">
            <div>{{ t('payment.methodLabel') }}：{{ resultChannelName }}</div>
            <div>{{ t('payment.interactionLabel') }}：{{ interactionLabel }}</div>
            <div v-if="paymentResult.expires_at">{{ t('payment.expiresAt') }}：{{ formatDate(paymentResult.expires_at) }}</div>
          </div>
          <div v-if="showPayLink" class="xpay__copy-row" style="margin-top:14px">
            <button type="button" class="x-btn x-btn--primary" @click="handleOpenPayLink"><ExternalLink /> {{ t('payment.openPayLink') }}</button>
            <button type="button" class="x-btn x-btn--ghost" @click="handleCopyPayLink"><Copy /> {{ t('payment.copyPayLink') }}</button>
          </div>
        </section>
      </div>

      <!-- 右栏 -->
      <aside class="xpay__aside">
        <div class="xpay__card">
          <h2>{{ t('payment.actionTitle') }}</h2>
          <div v-if="showCountdown" class="xpay__countdown-sm">{{ t('payment.countdownLabel') }}：<span>{{ countdownText }}</span></div>
          <div v-if="paymentAlert" class="xck__alert" :class="paymentAlert.level === 'error' ? 'is-error' : paymentAlert.level === 'success' ? 'is-success-box' : 'is-warn'">{{ paymentAlert.message }}</div>

          <div v-if="selectedChannel" class="xpay__method-tag is-ok">{{ t('payment.methodLabel') }}：{{ selectedChannelName }}</div>
          <div v-else-if="!requiresOnlineChannel && !orderExpired && !orderCanceled" class="xpay__method-tag is-ok">{{ t('payment.walletPayOnly') }}</div>
          <div v-else-if="walletOnlyPayment && expectedOnlinePayCents > 0 && !orderExpired && !orderCanceled" class="xpay__method-tag is-warn">{{ t('payment.walletInsufficientHint') }}</div>
          <div v-else-if="!walletOnlyPayment && requiresOnlineChannel && !orderExpired && !orderCanceled" class="xpay__method-tag is-warn">{{ t('payment.selectChannelError') }}</div>

          <button type="button" class="x-btn x-btn--primary x-btn--lg x-btn--block" :disabled="!canSubmitPayment" @click="handlePayment">
            {{ submitting ? t('payment.submitting') : t('payment.submitButton') }}
          </button>
          <button type="button" class="x-btn x-btn--ghost x-btn--lg x-btn--block" style="margin-top:10px" :disabled="loading" @click="handleRefresh">
            {{ t('payment.refreshStatus') }}
          </button>
        </div>
      </aside>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { AlertCircle, Copy, ExternalLink, Wallet } from 'lucide-vue-next'
import PaymentAmountBreakdown from '../../components/payment/PaymentAmountBreakdown.vue'
import PaymentChannelSelector from '../../components/payment/PaymentChannelSelector.vue'
import XgaiCheckoutSteps from './components/XgaiCheckoutSteps.vue'
import { usePayment } from '../../composables/usePayment'
import './styles/xgai-checkout.css'

const { t } = useI18n()

const {
  loading, submitting, order, paymentResult, selectedChannelId, copied, walletAddressCopied,
  openedPayWindow, cachedPayment, guestAuth, guestAuthError, walletLoading, useBalance,
  backLink, showGuestAuthForm, walletOnlyPayment, showBalanceOption, configReady, channels,
  selectedChannel, selectedChannelName, cachedChannelName, resultChannelName, interactionLabel,
  paymentResultTitle, paymentGuideTitle, paymentGuideTip, showPayLink, showTelegramPayHint, payLinkOpenedTip,
  cryptoWalletAddress, cryptoPaymentDetails, hasCryptoPaymentDetails, qrUsingPayLinkFallback, showQRCode, qrImageUrl,
  orderExpired, orderCanceled, paymentAlert, countdownExpired, countdownText, showCountdown, showResultView, pollingActive, orderItems,
  customerFeeApplied, customerFeeAmountDisplay, payableAmountDisplay, walletBalanceDisplay,
  expectedWalletPaidDisplay, expectedOnlinePayDisplay, expectedOnlinePayCents, requiresOnlineChannel,
  paymentWalletPaidDisplay, paymentOnlinePayDisplay, isChannelDisabledForAmount, channelAmountLimitHint, canSubmitPayment,
  formatDate, statusLabel, formatMoney, hasDiscountAmount, formatDiscountMoney, getLocalizedText, orderItemSkuText, fulfillmentTypeLabelText,
  handleCopyPayLink, handleCopyWalletAddress, handleOpenPayLink, restoreCachedPayment, handleChangePaymentMethod,
  handlePayment, handleGuestAuthSubmit, handleRefresh,
  isX402Payment, x402Signing, x402Error, x402AmountDisplay, x402NetworkLabel, handleX402Pay,
} = usePayment()
</script>
