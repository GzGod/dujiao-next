<template>
  <div class="x-container xpost xpost--narrow">
    <div v-if="loading" class="xpost__detail-sk">
      <div class="xpost__sk" style="height:24px;width:40%" />
      <div class="xpost__sk" style="height:44px;width:75%" />
      <div class="xpost__sk" style="height:320px" />
    </div>

    <article v-else-if="post">
      <nav class="xpd__crumb">
        <RouterLink to="/">{{ t('nav.home') }}</RouterLink>
        <span>/</span>
        <RouterLink :to="backLink">{{ backText }}</RouterLink>
        <span>/</span>
        <b>{{ getLocalizedText(post.title) }}</b>
      </nav>

      <div class="xck__card xpost__article">
        <div v-if="post.thumbnail" class="xpost__cover">
          <img :src="getImageUrl(post.thumbnail)" :alt="getLocalizedText(post.title)" loading="lazy" />
        </div>

        <header class="xpost__article-head">
          <small>{{ post.type === 'blog' ? t('nav.blog') : t('nav.notice') }} · {{ formatDate(post.published_at) }}</small>
          <h1>{{ getLocalizedText(post.title) }}</h1>
          <p v-if="post.summary">{{ getLocalizedText(post.summary) }}</p>
        </header>

        <div class="prose max-w-none prose-a:text-primary prose-img:rounded-md" v-html="processHtmlForDisplay(getLocalizedText(post.content))" />

        <section v-if="relatedProducts.length" class="xpost__related">
          <h2>{{ t('blog.relatedProducts') }}</h2>
          <div>
            <RouterLink v-for="rp in relatedProducts" :key="rp.id" :to="`/products/${rp.slug}`">
              <img v-if="rp.image" :src="getImageUrl(rp.image)" :alt="getLocalizedText(rp.title)" loading="lazy" />
              <span>
                <b>{{ getLocalizedText(rp.title) }}</b>
                <small>{{ formatPrice(rp.price_amount) }}</small>
              </span>
            </RouterLink>
          </div>
        </section>

        <footer class="xpost__foot">
          <RouterLink :to="backLink" class="x-btn x-btn--ghost"><ArrowLeft /> {{ backText }}</RouterLink>
        </footer>
      </div>
    </article>

    <div v-else class="xpost__empty">
      <AlertCircle />
      <p>{{ t('blogDetail.notFound') }}</p>
      <RouterLink to="/blog" class="x-btn x-btn--primary">{{ t('blogDetail.backToBlog') }}</RouterLink>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { AlertCircle, ArrowLeft } from 'lucide-vue-next'
import { getImageUrl } from '../../utils/image'
import { processHtmlForDisplay } from '../../utils/content'
import { useBlogDetail } from '../../composables/useBlogDetail'
import './styles/xgai-post.css'

const { t } = useI18n()

const {
  loading, post, relatedProducts, getLocalizedText, formatDate, formatPrice, backLink, backText,
} = useBlogDetail()
</script>
