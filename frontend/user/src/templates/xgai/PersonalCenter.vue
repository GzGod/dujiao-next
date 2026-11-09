<template>
  <div class="x-container xme">
    <!-- Account header -->
    <header class="xme__head">
      <div class="xme__who">
        <div class="xme__avatar">{{ displayInitial }}</div>
        <div class="xme__who-text">
          <span class="x-eyebrow">{{ t('personalCenter.title') }}</span>
          <h1>{{ userProfileStore.displayName }}</h1>
          <p>{{ userProfileStore.profile?.email || t('personalCenter.subtitle') }}</p>
        </div>
      </div>
      <div class="xme__tags">
        <span class="xme__pill" :class="emailVerifiedVariant === 'success' ? 'is-ok' : 'is-warn'">{{ emailVerifiedLabel }}</span>
        <span v-if="userProfileStore.currentLevel" class="xme__pill is-gold">
          <img v-if="isImagePath(userProfileStore.currentLevel?.icon)" :src="getImageUrl(userProfileStore.currentLevel!.icon)" alt="" />
          <span v-else-if="userProfileStore.currentLevel?.icon">{{ userProfileStore.currentLevel.icon }}</span>
          <Crown v-else aria-hidden="true" />
          {{ levelName(userProfileStore.currentLevel) }}
        </span>
      </div>
    </header>

    <div class="xme__layout">
      <!-- Sidebar -->
      <aside class="xme__aside">
        <nav class="xme__nav">
          <button
            v-for="item in visibleSectionItems"
            :key="item.key"
            type="button"
            :class="{ 'is-active': currentSection === item.key }"
            @click="switchSection(item.key)"
          >
            <component :is="item.icon" aria-hidden="true" />
            <span>{{ t(item.label) }}</span>
          </button>
        </nav>
      </aside>

      <!-- Content -->
      <section class="xme__content">
        <div
          v-if="globalAlert"
          class="xme__notice"
          :class="globalAlert.level === 'error' ? 'is-error' : (globalAlert.level === 'success' ? 'is-success' : 'is-warn')"
        >
          {{ globalAlert.message }}
        </div>

        <!-- Overview -->
        <template v-if="currentSection === 'overview'">
          <div class="xme__stats">
            <div class="xme__stat">
              <div class="xme__stat-icon is-blue"><ShoppingBag aria-hidden="true" /></div>
              <div>
                <small>{{ t('personalCenter.tabs.orders') }}</small>
                <strong>{{ userProfileStore.loadingOrders ? '—' : userProfileStore.ordersTotal }}</strong>
              </div>
            </div>
            <div class="xme__stat">
              <div class="xme__stat-icon is-violet"><Crown aria-hidden="true" /></div>
              <div>
                <small>{{ t('personalCenter.memberLevel.currentLevel') }}</small>
                <strong>
                  <img v-if="isImagePath(userProfileStore.currentLevel?.icon)" :src="getImageUrl(userProfileStore.currentLevel!.icon)" alt="" />
                  <span>{{ levelName(userProfileStore.currentLevel) }}</span>
                </strong>
              </div>
            </div>
            <div class="xme__stat">
              <div class="xme__stat-icon is-gold"><Percent aria-hidden="true" /></div>
              <div>
                <small>{{ t('personalCenter.memberLevel.discountRate') }}</small>
                <strong>{{ discountText }}</strong>
              </div>
            </div>
            <div class="xme__stat">
              <div class="xme__stat-icon is-green"><ShieldCheck aria-hidden="true" /></div>
              <div>
                <small>{{ t('personalCenter.overview.accountLabel') }}</small>
                <strong>
                  <span class="xme__pill" :class="emailVerifiedVariant === 'success' ? 'is-ok' : 'is-warn'">{{ emailVerifiedLabel }}</span>
                </strong>
              </div>
            </div>
          </div>

          <!-- Member level -->
          <div v-if="userProfileStore.memberLevels.length > 0" class="xck__card">
            <div class="xme__level-top">
              <div class="xme__level-who">
                <div class="xme__level-icon">
                  <img v-if="isImagePath(userProfileStore.currentLevel?.icon)" :src="getImageUrl(userProfileStore.currentLevel!.icon)" alt="" />
                  <span v-else>{{ userProfileStore.currentLevel?.icon || '👤' }}</span>
                </div>
                <div>
                  <p class="xme__caption">{{ t('personalCenter.memberLevel.currentLevel') }}</p>
                  <p class="xme__level-name">{{ levelName(userProfileStore.currentLevel) }}</p>
                </div>
              </div>
              <div class="xme__tags">
                <span class="xme__pill is-blue">
                  {{ t('personalCenter.memberLevel.discountRate') }}
                  {{ userProfileStore.currentLevel && userProfileStore.currentLevel.discount_rate < 100
                    ? t('personalCenter.memberLevel.discountOff', { n: userProfileStore.currentLevel.discount_rate / 10 })
                    : t('personalCenter.memberLevel.noDiscount') }}
                </span>
                <span v-if="!userProfileStore.nextLevel && userProfileStore.currentLevel" class="xme__pill is-ok">
                  {{ t('personalCenter.memberLevel.highestLevel') }}
                </span>
              </div>
            </div>

            <div v-if="userProfileStore.nextLevel" class="xme__next">
              <div class="xme__next-head">
                <div class="xme__level-icon is-sm">
                  <img v-if="isImagePath(userProfileStore.nextLevel.icon)" :src="getImageUrl(userProfileStore.nextLevel.icon)" alt="" />
                  <span v-else>{{ userProfileStore.nextLevel.icon || '⭐' }}</span>
                </div>
                <div>
                  <p class="xme__caption">{{ t('personalCenter.memberLevel.nextLevel') }}</p>
                  <div class="xme__next-name">
                    <b>{{ levelName(userProfileStore.nextLevel) }}</b>
                    <span v-if="userProfileStore.nextLevel.discount_rate < 100">
                      {{ t('personalCenter.memberLevel.discountOff', { n: userProfileStore.nextLevel.discount_rate / 10 }) }}
                    </span>
                  </div>
                </div>
              </div>

              <div v-if="userProfileStore.upgradeProgress" class="xme__progress">
                <div v-if="userProfileStore.upgradeProgress.rechargePercent !== null">
                  <div class="xme__progress-row">
                    <span>{{ t('personalCenter.memberLevel.rechargeProgress') }}</span>
                    <span>{{ userProfileStore.upgradeProgress.recharged.toFixed(2) }} / {{ userProfileStore.upgradeProgress.rechargeThreshold.toFixed(2) }}</span>
                  </div>
                  <div class="xme__bar"><div :style="{ width: userProfileStore.upgradeProgress.rechargePercent + '%' }"></div></div>
                </div>
                <div v-if="userProfileStore.upgradeProgress.spendPercent !== null">
                  <div class="xme__progress-row">
                    <span>{{ t('personalCenter.memberLevel.spendProgress') }}</span>
                    <span>{{ userProfileStore.upgradeProgress.spent.toFixed(2) }} / {{ userProfileStore.upgradeProgress.spendThreshold.toFixed(2) }}</span>
                  </div>
                  <div class="xme__bar"><div :style="{ width: userProfileStore.upgradeProgress.spendPercent + '%' }"></div></div>
                </div>
              </div>
            </div>
          </div>

          <!-- Recent orders -->
          <div class="xck__card">
            <div class="xme__card-head">
              <h2>{{ t('personalCenter.overview.recentOrdersTitle') }}</h2>
              <RouterLink to="/me/orders" class="x-link">{{ t('personalCenter.overview.viewAllOrders') }}</RouterLink>
            </div>
            <div v-if="userProfileStore.loadingOrders" class="xme__orders">
              <div v-for="idx in 3" :key="idx" class="xme__sk"></div>
            </div>
            <div v-else-if="userProfileStore.recentOrders.length === 0" class="xme__empty">
              {{ t('personalCenter.overview.emptyOrders') }}
            </div>
            <div v-else class="xme__orders">
              <div v-for="order in userProfileStore.recentOrders" :key="order.order_no" class="xme__order">
                <div>
                  <small>{{ t('orders.orderNo') }}：{{ order.order_no }}</small>
                  <strong>{{ formatMoney(order.total_amount, order.currency) }}</strong>
                  <small>{{ formatDate(order.created_at) }}</small>
                </div>
                <div class="xme__order-actions">
                  <Badge :variant="statusVariant(order.status)" class="rounded-full">{{ statusLabel(order.status) }}</Badge>
                  <RouterLink :to="`/orders/${order.order_no}`" class="x-btn x-btn--ghost">{{ t('orders.viewDetails') }}</RouterLink>
                  <RouterLink v-if="order.status === 'pending_payment'" :to="`/pay?order_no=${order.order_no}`" class="x-btn x-btn--primary">
                    {{ t('orders.payNow') }}
                  </RouterLink>
                </div>
              </div>
            </div>
          </div>
        </template>

        <!-- Other panels: shared components -->
        <div v-else class="xme__panel">
          <ProfilePanel v-if="currentSection === 'profile'" />
          <SecurityPanel v-else-if="currentSection === 'security'" />
          <OrdersPanel v-else-if="currentSection === 'orders'" />
          <WalletPanel v-else-if="currentSection === 'wallet'" />
          <AffiliatePanel v-else-if="currentSection === 'affiliate'" />
          <div v-else-if="currentSection === 'reseller' && canAccessResellerConsole" class="xck__card xme__reseller">
            <h2>{{ t('resellerConsole.title') }}</h2>
            <p>{{ t('resellerConsole.dashboard.description') }}</p>
            <RouterLink to="/reseller" class="x-btn x-btn--primary">{{ t('resellerConsole.nav.dashboard') }}</RouterLink>
          </div>
          <GiftCardPanel v-else-if="currentSection === 'giftCard'" />
          <ApiPanel v-else-if="currentSection === 'api'" />
          <OrdersPanel v-else />
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Crown, Percent, ShieldCheck, ShoppingBag } from 'lucide-vue-next'
import { Badge } from '@/components/ui/badge'
import { getImageUrl } from '../../utils/image'
import ProfilePanel from '../../views/personal/ProfilePanel.vue'
import SecurityPanel from '../../views/personal/SecurityPanel.vue'
import OrdersPanel from '../../views/personal/OrdersPanel.vue'
import WalletPanel from '../../views/personal/WalletPanel.vue'
import GiftCardPanel from '../../views/personal/GiftCardPanel.vue'
import AffiliatePanel from '../../views/personal/AffiliatePanel.vue'
import ApiPanel from '../../views/personal/ApiPanel.vue'
import { usePersonalCenter, type PersonalSection } from '../../composables/usePersonalCenter'
import './styles/xgai-me.css'

const { t } = useI18n()

const props = withDefaults(defineProps<{ section?: PersonalSection }>(), {
  section: 'overview',
})

const {
  userProfileStore, canAccessResellerConsole, visibleSectionItems, currentSection, globalAlert,
  displayInitial, switchSection, statusLabel, statusVariant, formatMoney, formatDate,
  emailVerifiedLabel, emailVerifiedVariant, discountText, isImagePath, levelName,
} = usePersonalCenter(() => props.section)
</script>
