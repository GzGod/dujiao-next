<template>
  <div class="x404">
    <div class="x404__card">
      <span class="x404__badge">
        <Info aria-hidden="true" /> {{ brandSiteName }}
      </span>
      <Frown class="x404__icon" aria-hidden="true" />
      <p class="x404__num">404</p>
      <h1 class="x404__title">{{ t('notFoundPage.title') }}</h1>
      <p class="x404__desc">{{ t('notFoundPage.description', { site: brandSiteName }) }}</p>

      <div class="x404__links">
        <p class="x404__links-label">{{ t('notFoundPage.quickLinksTitle') }}</p>
        <div class="x404__quick">
          <RouterLink to="/products" class="x-btn x-btn--ghost">
            <ShoppingBag aria-hidden="true" /> {{ t('nav.products') }}
          </RouterLink>
          <RouterLink to="/notice" class="x-btn x-btn--ghost">
            <Bell aria-hidden="true" /> {{ t('nav.notice') }}
          </RouterLink>
          <RouterLink to="/blog" class="x-btn x-btn--ghost">
            <BookOpen aria-hidden="true" /> {{ t('nav.blog') }}
          </RouterLink>
        </div>
      </div>

      <div class="x404__actions">
        <RouterLink to="/" class="x-btn x-btn--primary x-btn--lg">
          <Home aria-hidden="true" /> {{ t('notFoundPage.backHome') }}
        </RouterLink>
        <button type="button" class="x-btn x-btn--ghost x-btn--lg" @click="goBack">
          <ArrowLeft aria-hidden="true" /> {{ t('notFoundPage.backPrevious') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ArrowLeft, Bell, BookOpen, Frown, Home, Info, ShoppingBag } from 'lucide-vue-next'
import { useNotFound } from '../../composables/useNotFound'
import './styles/xgai-misc.css'

const { t } = useI18n()

const { brandSiteName, goBack } = useNotFound()
</script>
