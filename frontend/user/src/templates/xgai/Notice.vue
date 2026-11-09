<template>
  <div class="x-container xmisc">
    <header class="xmisc__head" style="max-width:880px;margin-inline:auto">
      <h1>{{ t('nav.notice') }}</h1>
      <p>{{ t('notice.subtitle') }}</p>
    </header>

    <!-- Loading -->
    <div v-if="loading" class="xmisc__notices">
      <div v-for="i in 6" :key="i" class="xmisc__sk"></div>
    </div>

    <!-- Notices -->
    <template v-else-if="notices.length > 0">
      <div class="xmisc__notices">
        <button
          v-for="notice in notices"
          :key="notice.id"
          type="button"
          class="xmisc__notice"
          @click="goToNotice(notice.slug)"
        >
          <div class="xmisc__notice-thumb">
            <img v-if="notice.thumbnail" :src="getImageUrl(notice.thumbnail)" :alt="getLocalizedText(notice.title)" loading="lazy" />
            <Bell v-else aria-hidden="true" />
          </div>
          <div class="xmisc__notice-body">
            <div class="xmisc__notice-meta">
              <span class="xmisc__notice-tag">{{ t('nav.notice') }}</span>
              <span class="xmisc__notice-date">{{ formatDate(notice.published_at) }}</span>
            </div>
            <p class="xmisc__notice-title">{{ getLocalizedText(notice.title) }}</p>
            <p class="xmisc__notice-summary">{{ getLocalizedText(notice.summary) }}</p>
          </div>
          <ChevronRight class="xmisc__notice-arrow" aria-hidden="true" />
        </button>
      </div>

      <div v-if="totalPages > 1" class="xmisc__pager">
        <button type="button" class="x-btn x-btn--ghost" :disabled="currentPage <= 1" @click="changePage(currentPage - 1)">
          <ChevronLeft aria-hidden="true" />
        </button>
        <span>{{ currentPage }} / {{ totalPages }}</span>
        <button type="button" class="x-btn x-btn--ghost" :disabled="currentPage >= totalPages" @click="changePage(currentPage + 1)">
          <ChevronRight aria-hidden="true" />
        </button>
      </div>
    </template>

    <!-- Empty -->
    <div v-else class="xmisc__empty" style="border:1px dashed var(--x-line-strong);border-radius:var(--x-r-xl);max-width:880px;margin-inline:auto">
      <Bell aria-hidden="true" />
      <p>{{ t('notice.empty') }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Bell, ChevronLeft, ChevronRight } from 'lucide-vue-next'
import { getImageUrl } from '../../utils/image'
import { usePostList } from '../../composables/usePostList'
import './styles/xgai-misc.css'

const { t } = useI18n()

const {
  loading, posts: notices, currentPage, totalPages,
  getLocalizedText, formatDate, goToPost: goToNotice, changePage,
} = usePostList('notice', { title: () => t('nav.notice'), canonicalPath: '/notice' })
</script>
