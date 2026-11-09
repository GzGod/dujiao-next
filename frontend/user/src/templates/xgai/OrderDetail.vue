<template>
  <div class="xord">
    <nav class="x-container xpd__crumb xck__crumb" :aria-label="t('xgai.detail.breadcrumb')">
      <RouterLink to="/">{{ t('nav.home') }}</RouterLink>
      <span>/</span>
      <RouterLink to="/me/orders">{{ t('orders.title') }}</RouterLink>
      <span>/</span>
      <b>{{ t('orderDetail.title') }}</b>
    </nav>

    <div class="x-container">
      <header class="xck__head xord__head">
        <div>
          <h1>{{ t('orderDetail.title') }}</h1>
          <p class="xck__sub">{{ t('orderDetail.subtitle') }}</p>
        </div>
      </header>

      <!-- Loading -->
      <div v-if="loading" class="xord__loading">
        <div class="xord__sk" />
        <div class="xord__sk xord__sk--lg" />
      </div>

      <!-- 不存在 -->
      <div v-else-if="!order" class="xck__empty">
        <AlertCircle />
        <p>{{ t('orderDetail.notFound') }}</p>
        <button type="button" class="x-btn x-btn--primary" @click="debouncedLoadOrder()">{{ t('errorBoundary.retry') }}</button>
      </div>

      <template v-else>
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
            <RouterLink v-if="order.status === 'pending_payment'" :to="`/pay?order_no=${order.order_no}`" class="x-btn x-btn--primary">{{ t('orderDetail.payNow') }}</RouterLink>
            <button v-if="order.status === 'pending_payment'" type="button" class="x-btn x-btn--ghost xord__danger" @click="cancelOrder">{{ t('orderDetail.cancel') }}</button>
          </div>
        </div>

        <VaultOrderBody
          :order="order"
          variant="user"
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
import { useOrderDetail } from '../../composables/useOrderDetail'
import './styles/xgai-orders.css'

const { t } = useI18n()

const {
  loading, order, debouncedLoadOrder, cancelOrder, fulfillmentDownloading, handleDownloadFulfillment,
  statusLabel, statusVariant, formatDate, formatMoney,
} = useOrderDetail()
</script>
