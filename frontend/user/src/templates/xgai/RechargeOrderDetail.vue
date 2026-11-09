<template>
  <div class="x-container xrech">
    <div class="xrech__top">
      <div>
        <h1>{{ t('rechargeOrder.title') }}</h1>
        <p>{{ t('rechargeOrder.subtitle') }}</p>
      </div>
      <RouterLink to="/me/orders">{{ t('rechargeOrder.backList') }}</RouterLink>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="xrech__card xck__card">
      <div class="xrech__sk"></div>
    </div>

    <!-- Not found -->
    <div v-else-if="!recharge" class="xrech__not-found">
      <AlertCircle aria-hidden="true" />
      <p>{{ t('rechargeOrder.notFound') }}</p>
      <button type="button" class="x-btn x-btn--ghost" @click="loadDetail()">{{ t('errorBoundary.retry') }}</button>
    </div>

    <template v-else>
      <!-- Header -->
      <div class="xck__card xrech__card">
        <div class="xrech__header">
          <div>
            <p class="xrech__label">{{ t('personalCenter.wallet.rechargeNoLabel') }}</p>
            <p class="xrech__value">{{ recharge.recharge_no }}</p>
            <p class="xrech__date">{{ t('rechargeOrder.createdAtLabel') }}：{{ formatDate(recharge.created_at) }}</p>
          </div>
          <div>
            <p class="xrech__label">{{ t('rechargeOrder.rechargeAmount') }}</p>
            <p class="xrech__amount">{{ formatMoney(recharge.amount, recharge.currency) }}</p>
          </div>
          <Badge :variant="rechargeStatusVariant(recharge.status)" class="rounded-full">{{ rechargeStatusText(recharge.status) }}</Badge>
        </div>
      </div>

      <!-- Amount detail -->
      <div class="xck__card xrech__card">
        <h2>{{ t('rechargeOrder.amountTitle') }}</h2>
        <div class="xrech__grid">
          <div class="xrech__cell">
            <small>{{ t('rechargeOrder.rechargeAmount') }}</small>
            <strong>{{ formatMoney(recharge.amount, recharge.currency) }}</strong>
          </div>
          <div v-if="customerFeeApplied" class="xrech__cell is-warn">
            <small>{{ t('payment.feeAmountLabel') }}</small>
            <strong>{{ formatMoney(recharge.fee_amount, recharge.currency) }}</strong>
          </div>
          <div class="xrech__cell is-primary">
            <small>{{ t('personalCenter.wallet.payAmountLabel') }}</small>
            <strong>{{ formatMoney(recharge.payable_amount, recharge.currency) }}</strong>
          </div>
        </div>
      </div>

      <!-- Time -->
      <div class="xck__card xrech__card">
        <h2>{{ t('rechargeOrder.timeTitle') }}</h2>
        <div class="xrech__grid">
          <div class="xrech__cell">
            <small>{{ t('rechargeOrder.createdAtLabel') }}</small>
            <strong>{{ formatDate(recharge.created_at) }}</strong>
          </div>
          <div v-if="recharge.paid_at" class="xrech__cell">
            <small>{{ t('rechargeOrder.paidAtLabel') }}</small>
            <strong>{{ formatDate(recharge.paid_at) }}</strong>
          </div>
          <div v-if="payment?.expires_at" class="xrech__cell">
            <small>{{ t('payment.expiresAt') }}</small>
            <strong>{{ formatDate(payment.expires_at) }}</strong>
          </div>
        </div>
      </div>

      <!-- Remark -->
      <div v-if="recharge.remark" class="xck__card xrech__card">
        <h2>{{ t('rechargeOrder.remarkLabel') }}</h2>
        <p style="margin:0;color:var(--x-ink-2);font-size:14px;line-height:1.7">{{ recharge.remark }}</p>
      </div>

      <!-- Success banner -->
      <div v-if="recharge.status === 'success'" class="xrech__success">
        <CheckCircle2 aria-hidden="true" />
        <p>{{ t('personalCenter.wallet.rechargeSuccess') }}</p>
      </div>

      <!-- Payment area -->
      <div v-if="isPending" class="xck__card xrech__card">
        <h2>{{ t('rechargeOrder.paymentTitle') }}</h2>
        <p class="xrech__pay-hint">{{ t('personalCenter.wallet.pendingHint') }}</p>
        <div class="xrech__pay-grid">
          <div v-if="showQRCode" class="xrech__qr">
            <small>{{ t('payment.qrTitle') }}</small>
            <div class="xrech__qr-img"><img :src="qrImageUrl" alt="Recharge QR" /></div>
            <p v-if="qrUsingPayLinkFallback" class="xrech__qr-hint">{{ t('payment.qrFallbackHint') }}</p>
          </div>
          <div class="xrech__actions-panel">
            <div v-if="hasCryptoPaymentDetails" class="xrech__crypto">
              <div v-for="item in cryptoPaymentDetails" :key="item.key" class="xrech__crypto-row">
                <span class="xrech__crypto-key">{{ item.label }}</span>
                <span class="xrech__crypto-val">
                  {{ item.value }}<span v-if="item.detail" class="xrech__crypto-detail"> ({{ item.detail }})</span>
                </span>
              </div>
              <div v-if="cryptoWalletAddress" class="xrech__copy-row">
                <button type="button" class="x-btn x-btn--ghost" @click="handleCopyWalletAddress">{{ t('payment.copyWalletAddress') }}</button>
                <span v-if="walletAddressCopied" class="xrech__copy-ok">{{ t('payment.copied') }}</span>
              </div>
            </div>
            <div class="xrech__btns">
              <button v-if="payLink" type="button" class="x-btn x-btn--ghost" @click="handleOpenPayLink">{{ t('payment.openPayLink') }}</button>
              <button type="button" class="x-btn x-btn--primary" :disabled="checkingPayment" @click="checkPayment">
                {{ checkingPayment ? t('personalCenter.wallet.checkingPayStatus') : t('personalCenter.wallet.checkPayStatus') }}
              </button>
            </div>
            <p v-if="showTelegramPayHint" class="xrech__tg-hint">{{ t('payment.telegramExternalHint') }}</p>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { AlertCircle, CheckCircle2 } from 'lucide-vue-next'
import { Badge } from '@/components/ui/badge'
import { useRechargeOrderDetail } from '../../composables/useRechargeOrderDetail'
import './styles/xgai-misc.css'

const { t } = useI18n()

const {
  loading, checkingPayment, recharge, payment, walletAddressCopied, qrImageUrl,
  isPending, payLink, showTelegramPayHint, qrUsingPayLinkFallback, showQRCode,
  cryptoWalletAddress, cryptoPaymentDetails, hasCryptoPaymentDetails, customerFeeApplied,
  rechargeStatusText, rechargeStatusVariant, formatMoney, formatDate,
  loadDetail, checkPayment, handleOpenPayLink, handleCopyWalletAddress,
} = useRechargeOrderDetail()
</script>
