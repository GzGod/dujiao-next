<template>
  <div class="xord">
    <nav class="x-container xpd__crumb xck__crumb" :aria-label="t('xgai.detail.breadcrumb')">
      <RouterLink to="/">{{ t('nav.home') }}</RouterLink>
      <span>/</span>
      <b>{{ t('guestOrders.title') }}</b>
    </nav>

    <div class="x-container">
      <header class="xck__head xord__head">
        <div>
          <h1>{{ t('guestOrders.title') }}</h1>
          <p class="xck__sub">{{ t('guestOrders.subtitle') }}</p>
        </div>
      </header>

      <!-- 查询表单 -->
      <form class="xck__card xord__form" @submit.prevent="handleSearch">
        <div v-if="hasSavedAuth" class="xord__saved">
          <span>{{ t('guestOrders.savedHint', { email: savedAuth.email || '-' }) }}</span>
          <button type="button" class="xord__text-btn" @click="clearSaved">{{ t('guestOrders.clearSaved') }}</button>
        </div>
        <div class="xord__fields">
          <input v-model="email" type="email" class="xck__input" :placeholder="t('guestOrders.emailPlaceholder')" />
          <input v-model="orderPassword" type="password" class="xck__input" :placeholder="t('guestOrders.passwordPlaceholder')" />
          <input v-model="orderNo" type="text" class="xck__input" :placeholder="t('guestOrders.orderNoPlaceholder')" />
          <button type="submit" class="x-btn x-btn--primary" :disabled="loading">
            <Search /> {{ loading ? t('guestOrders.searching') : t('guestOrders.search') }}
          </button>
        </div>
        <p class="xord__tip">{{ t('guestOrders.tip') }}</p>
        <div v-if="error" class="xck__alert is-error">{{ error }}</div>
      </form>

      <!-- 空态 -->
      <div v-if="orders.length === 0 && !loading" class="xck__empty">
        <ClipboardList />
        <p>{{ emptyMessage }}</p>
      </div>

      <!-- 列表 -->
      <div v-else class="xord__list">
        <div v-for="order in orders" :key="order.order_no" class="xck__card xord__row">
          <div class="xord__row-info">
            <div class="xord__label">{{ t('orders.orderNo') }}：{{ order.order_no }}</div>
            <div class="xord__amount">{{ formatMoney(order.total_amount, order.currency) }}</div>
            <div v-if="hasDiscount(order)" class="xord__discounts">
              <span v-if="hasDiscountAmount(order.discount_amount)">{{ t('orderDetail.couponDiscountLabel') }}：{{ formatDiscountMoney(order.discount_amount, order.currency) }}</span>
              <span v-if="hasDiscountAmount(order.promotion_discount_amount)">{{ t('orderDetail.promotionDiscountLabel') }}：{{ formatDiscountMoney(order.promotion_discount_amount, order.currency) }}</span>
            </div>
            <div class="xord__date">{{ formatDate(order.created_at) }}</div>
          </div>
          <div class="xord__row-actions">
            <Badge :variant="statusVariant(order.status)" class="rounded-full">{{ statusLabel(order.status) }}</Badge>
            <RouterLink :to="{ name: 'guest-order-detail', params: { order_no: order.order_no } }" class="x-btn x-btn--ghost">
              <Eye /> {{ t('guestOrders.viewDetails') }}
            </RouterLink>
            <RouterLink v-if="order.status === 'pending_payment'" :to="`/pay?guest=1&order_no=${order.order_no}`" class="x-btn x-btn--primary">
              <CreditCard /> {{ t('guestOrders.payNow') }}
            </RouterLink>
          </div>
        </div>

        <div v-if="pagination.total_page > 1" class="xord__pager">
          <button type="button" class="x-btn x-btn--ghost" :disabled="loading || pagination.page <= 1" @click="changePage(pagination.page - 1)"><ChevronLeft /></button>
          <span>{{ pagination.page }} / {{ pagination.total_page }}</span>
          <button type="button" class="x-btn x-btn--ghost" :disabled="loading || pagination.page >= pagination.total_page" @click="changePage(pagination.page + 1)"><ChevronRight /></button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ChevronLeft, ChevronRight, ClipboardList, Search, Eye, CreditCard } from 'lucide-vue-next'
import { Badge } from '@/components/ui/badge'
import { useGuestOrders } from '../../composables/useGuestOrders'
import './styles/xgai-orders.css'

const { t } = useI18n()

const {
  savedAuth, email, orderPassword, orderNo, loading, error, orders, pagination,
  hasSavedAuth, clearSaved, handleSearch, emptyMessage, changePage,
  statusLabel, statusVariant, formatMoney, formatDiscountMoney, hasDiscountAmount, hasDiscount, formatDate,
} = useGuestOrders()
</script>
