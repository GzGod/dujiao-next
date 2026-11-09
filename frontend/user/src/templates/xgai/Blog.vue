<template>
  <div class="x-container xpost">
    <header class="xpost__head">
      <h1>{{ t('nav.blog') }}</h1>
      <p>{{ t('blog.subtitle') }}</p>
    </header>

    <div class="xpost__search">
      <Search aria-hidden="true" />
      <input v-model="searchKeyword" type="text" class="xck__input" :placeholder="t('blog.searchPlaceholder')" />
      <button v-if="searchKeyword" type="button" class="xpost__clear" @click="searchKeyword = ''">{{ t('blog.searchClear') }}</button>
    </div>

    <div v-if="loading" class="xpost__grid">
      <div v-for="i in 6" :key="i" class="xpost__sk" />
    </div>

    <template v-else-if="posts.length > 0">
      <div class="xpost__grid">
        <RouterLink v-for="post in posts" :key="post.id" :to="`/blog/${post.slug}`" class="xpost__card">
          <div v-if="post.thumbnail" class="xpost__thumb">
            <img :src="getImageUrl(post.thumbnail)" :alt="getLocalizedText(post.title)" loading="lazy" />
          </div>
          <div class="xpost__body">
            <small>{{ post.type === 'blog' ? t('nav.blog') : t('nav.notice') }} · {{ formatDate(post.published_at) }}</small>
            <h2>{{ getLocalizedText(post.title) }}</h2>
            <p>{{ getLocalizedText(post.summary) }}</p>
            <span class="xpost__more">{{ t('blog.readMore') }} <ArrowRight /></span>
          </div>
        </RouterLink>
      </div>

      <div v-if="totalPages > 1" class="xpost__pager">
        <button type="button" class="x-btn x-btn--ghost" :disabled="currentPage <= 1" :aria-label="t('blog.searchClear')" @click="changePage(currentPage - 1)"><ChevronLeft /></button>
        <span>{{ currentPage }} / {{ totalPages }}</span>
        <button type="button" class="x-btn x-btn--ghost" :disabled="currentPage >= totalPages" @click="changePage(currentPage + 1)"><ChevronRight /></button>
      </div>
    </template>

    <div v-else class="xpost__empty">
      <BookOpen />
      <p>{{ searchKeyword.trim() ? t('blog.noResults') : t('blog.empty') }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ArrowRight, BookOpen, ChevronLeft, ChevronRight, Search } from 'lucide-vue-next'
import { getImageUrl } from '../../utils/image'
import { usePostList } from '../../composables/usePostList'
import './styles/xgai-post.css'

const { t } = useI18n()

const {
  loading, posts, currentPage, totalPages, searchKeyword,
  getLocalizedText, formatDate, changePage,
} = usePostList('blog', { title: () => t('nav.blog'), canonicalPath: '/blog' })
</script>
