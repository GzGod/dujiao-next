<template>
  <div class="x-container xmisc">
    <div v-if="loading" class="xmisc__spin">
      <Loader2 aria-hidden="true" />
    </div>

    <div v-else class="xmisc__body xmisc__legal">
      <h1 class="xmisc__legal-title">{{ title }}</h1>
      <article v-if="type === 'terms'" class="xlegal-terms">
        <section v-for="section in publishedTerms" :key="section[0]">
          <h2>{{ section[0] }}</h2>
          <p v-for="paragraph in section.slice(1)" :key="paragraph">{{ paragraph }}</p>
        </section>
      </article>
      <div
        v-else-if="content"
        class="prose max-w-none prose-a:text-primary prose-img:rounded-md"
        v-html="content"
      ></div>
      <div v-else class="xmisc__empty">
        <FileText aria-hidden="true" />
        <p>{{ t('common.noContent') }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { FileText, Loader2 } from 'lucide-vue-next'
import { serviceTerms } from '../../content/serviceTerms'
import { useLegal } from '../../composables/useLegal'
import './styles/xgai-misc.css'

const { t, locale } = useI18n()
const publishedTerms = computed(() => serviceTerms(locale.value))

const props = defineProps<{
  type: 'terms' | 'privacy'
}>()

const { loading, title, content } = useLegal(() => props.type)
</script>

<style scoped>
.xlegal-terms { max-width: 850px; margin-inline: auto; }
.xlegal-terms h2 { margin: 32px 0 14px; font-size: 20px; font-weight: 650; color: var(--x-ink); }
.xlegal-terms p { color: var(--x-ink-2); line-height: 1.95; margin: 12px 0; }
.xlegal-terms section:nth-child(2) p:first-of-type { font-weight: 700; color: var(--x-ink); padding: 16px; border: 1px solid var(--x-line-strong); border-radius: 12px; }
</style>
