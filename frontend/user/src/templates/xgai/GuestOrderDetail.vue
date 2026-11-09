<template>
  <div class="xord">
    <nav class="x-container xpd__crumb xck__crumb" :aria-label="t('xgai.detail.breadcrumb')">
      <RouterLink to="/">{{ t('nav.home') }}</RouterLink>
      <span>/</span>
      <RouterLink to="/guest/orders">{{ t('guestOrders.title') }}</RouterLink>
      <span>/</span>
      <b>{{ t('guestOrderDetail.title') }}</b>
    </nav>

    <div class="x-container">
      <header class="xck__head xord__head">
        <div>
          <h1>{{ t('guestOrderDetail.title') }}</h1>
          <p class="xck__sub">{{ t('guestOrderDetail.subtitle') }}</p>
        </div>
        <RouterLink class="xord__back" to="/guest/orders">{{ t('guestOrderDetail.backSearch') }}</RouterLink>
      </header>

      <!-- 游客验证 -->
      <form v-if="viewState === 'auth'" class="xck__card xord__form" @submit.prevent="handleAuthSubmit">
        <h2>{{ t('guestOrderDetail.authTitle') }}</h2>
        <p class="xord__hint">{{ t('guestOrderDetail.authHint') }}</p>
        <div class="xord__fields xord__fields--two">
          <input v-model="auth.email" type="email" class="xck__input" :placeholder="t('guestOrders.emailPlaceholder')" />
          <input v-model="auth.order_password" type="password" class="xck__input" :placeholder="t('guestOrders.passwordPlaceholder')" />
        </div>
        <div v-if="authError" class="xck__alert is-error">{{ authError }}</div>
        <div class="xord__actions">
          <button type="submit" class="x-btn x-btn--primary">{{ t('guestOrderDetail.authSubmit') }}</button>
          <button type="button" class="x-btn x-btn--ghost" @click="clearAuth">{{ t('guestOrderDetail.authClear') }}</button>
        </div>
      </form>

      <!-- Loading -->
      <div v-if="viewState === 'loading'" class="xord__loading">
        <div class="xord__sk" />
        <div class="xord__sk xord__sk--lg" />
      </div>

      <!-- 不存在 -->
      <div v-else-if="viewState === 'empty'" class="xck__empty">
        <AlertCircle />
        <p>{{ t('guestOrderDetail.notFound') }}</p>
      </div>

      <template v-else-if="viewState === 'detail' && order">
        <div class="xck__card xord__summary">
          <div>
            <div class="xord__label">{{ t('orders.orderNo') }}</div>
            <b>{{ order.order_no }}</b>
            <div class="xord__date">{{ t('orderDetail.createdAtLabel') }}：{{ formatDate(order.created_at) }}</div>
          </div>
          <div>
            <div class="xord__label">{{ t('orderDetail.amountTotal') }}</div>
            <div class="xord__amount">{{ formatMoney(order.total_amount, order.currency) }}</div>
          </div>
          <div class="xord__summary-actions">
            <Badge :variant="statusVariant(order.status)" class="rounded-full">{{ statusLabel(order.status) }}</Badge>
            <RouterLink v-if="order.status === 'pending_payment'" :to="`/pay?guest=1&order_no=${order.order_no}`" class="x-btn x-btn--primary">{{ t('orders.payNow') }}</RouterLink>
          </div>
        </div>

        <VaultOrderBody
          :order="order"
          variant="guest"
          :fulfillment-downloading="fulfillmentDownloading"
          @download="handleDownloadFulfillment"
        />
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { AlertCircle } from 'lucide-vue-next'
import { Badge } from '@/components/ui/badge'
import VaultOrderBody from '../vault/components/VaultOrderBody.vue'
import { useGuestOrderDetail } from '../../composables/useGuestOrderDetail'
import './styles/xgai-orders.css'

const { t } = useI18n()

const {
  order, authError, auth, viewState, handleAuthSubmit, clearAuth,
  fulfillmentDownloading, handleDownloadFulfillment,
  statusLabel, statusVariant, formatDate, formatMoney,
} = useGuestOrderDetail()
</script>
