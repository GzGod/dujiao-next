<template>
  <div class="xcart">
    <nav class="x-container xpd__crumb xck__crumb" :aria-label="t('xgai.detail.breadcrumb')">
      <RouterLink to="/">{{ t('nav.home') }}</RouterLink>
      <span>/</span>
      <b>{{ t('cart.title') }}</b>
    </nav>

    <div class="x-container">
      <header class="xck__head">
        <h1>{{ t('cart.title') }}</h1>
        <p class="xck__sub">{{ t('cart.subtitle') }}</p>
      </header>

      <div class="xck__steps">
        <XgaiCheckoutSteps current="cart" />
      </div>

      <!-- 空购物车 -->
      <div v-if="cartItems.length === 0" class="xck__empty">
        <ShoppingCart />
        <p>{{ t('cart.empty') }}</p>
        <RouterLink to="/products" class="x-btn x-btn--primary">{{ t('cart.emptyAction') }}</RouterLink>
      </div>

      <div v-else class="xck__layout">
        <!-- 商品列表 -->
        <div class="xcart__list">
          <article v-for="item in cartItems" :key="cartItemKey(item)" class="xck__card xcart__item">
            <RouterLink :to="`/products/${item.slug}`" class="xcart__thumb">
              <img v-if="cartItemImage(item)" :src="cartItemImage(item)" :alt="getLocalizedText(item.title)" loading="lazy" />
              <Package v-else />
            </RouterLink>

            <div class="xcart__body">
              <div class="xcart__top">
                <div class="xcart__info">
                  <RouterLink :to="`/products/${item.slug}`" class="xcart__name">{{ getLocalizedText(item.title) }}</RouterLink>
                  <p class="xcart__meta">{{ t('cart.priceLabel') }}：{{ formatPrice(item.priceAmount, totalCurrency) }}</p>
                  <p v-if="itemSkuDisplay(item)" class="xcart__meta">{{ t('cart.skuLabel') }}：{{ itemSkuDisplay(item) }}</p>
                  <p v-if="itemStockHint(item)" class="xcart__meta">{{ itemStockHint(item) }}</p>
                  <div class="xcart__tags">
                    <Badge :variant="item.purchaseType === 'guest' ? 'warning' : 'info'" size="sm" class="rounded-full">{{ item.purchaseType === 'guest' ? t('productPurchase.guest') : t('productPurchase.member') }}</Badge>
                    <Badge variant="success" size="sm" class="rounded-full">{{ item.fulfillmentType === 'auto' ? t('products.fulfillmentType.auto') : t('products.fulfillmentType.manual') }}</Badge>
                  </div>
                </div>
                <button type="button" class="xcart__remove" @click="removeWithUndo(item)">
                  <Trash2 />
                  <span>{{ t('cart.remove') }}</span>
                </button>
              </div>

              <div class="xcart__bottom">
                <div class="xcart__qty">
                  <button type="button" :aria-label="t('cart.remove')" :disabled="item.quantity <= itemPurchaseMin(item)" @click="updateQty(item, item.quantity - 1)"><Minus /></button>
                  <input
                    type="number"
                    :value="item.quantity"
                    :min="itemPurchaseMin(item)"
                    :max="itemMaxQuantity(item)"
                    @change="handleQtyChange(item, $event)"
                  />
                  <button type="button" :aria-label="t('cart.remove')" :disabled="item.quantity >= itemMaxQuantity(item)" @click="updateQty(item, item.quantity + 1)"><Plus /></button>
                </div>
                <div class="xcart__subtotal">
                  <small>{{ t('checkout.totalPriceLabel') }}</small>
                  <strong>{{ itemSubtotal(item) }}</strong>
                </div>
              </div>

              <div v-if="quantityWarning(item)" class="xck__alert is-warn">{{ quantityWarning(item) }}</div>
            </div>
          </article>
        </div>

        <!-- 汇总 -->
        <aside class="xck__aside">
          <div class="xck__card">
            <h2>{{ t('cart.summaryTitle') }}</h2>
            <div class="xck__rows">
              <div>
                <span>{{ t('cart.itemsCount') }}</span>
                <span>{{ totalItems }}</span>
              </div>
              <div class="is-total">
                <span>{{ t('cart.totalLabel') }}</span>
                <span>{{ formatPrice(totalAmount, totalCurrency) }}</span>
              </div>
            </div>
            <p class="xcart__note">{{ t('cart.disclaimer') }}</p>
            <div class="xcart__actions">
              <RouterLink to="/checkout" class="x-btn x-btn--primary x-btn--lg x-btn--block">{{ t('cart.checkout') }} <ArrowRight /></RouterLink>
              <RouterLink to="/products" class="x-btn x-btn--ghost x-btn--block">{{ t('cart.emptyAction') }}</RouterLink>
            </div>
          </div>
        </aside>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ArrowRight, Minus, Package, Plus, ShoppingCart, Trash2 } from 'lucide-vue-next'
import { Badge } from '@/components/ui/badge'
import XgaiCheckoutSteps from './components/XgaiCheckoutSteps.vue'
import { useCart } from '../../composables/useCart'
import './styles/xgai-cart.css'

const { t } = useI18n()

const {
  getLocalizedText, formatPrice, totalCurrency,
  cartItems, totalItems, totalAmount,
  cartItemKey, cartItemImage, itemSkuDisplay, itemSubtotal, itemStockHint, quantityWarning,
  itemPurchaseMin, itemMaxQuantity,
  removeWithUndo, updateQty, handleQtyChange,
} = useCart()
</script>
