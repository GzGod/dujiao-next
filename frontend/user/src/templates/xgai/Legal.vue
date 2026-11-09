<template>
  <div class="x-container xmisc">
    <div v-if="loading" class="xmisc__spin">
      <Loader2 aria-hidden="true" />
    </div>

    <div v-else class="xmisc__body xmisc__legal">
      <h1 class="xmisc__legal-title">{{ title }}</h1>
      <div
        v-if="content"
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
import { useI18n } from 'vue-i18n'
import { FileText, Loader2 } from 'lucide-vue-next'
import { useLegal } from '../../composables/useLegal'
import './styles/xgai-misc.css'

const { t } = useI18n()

const props = defineProps<{
  type: 'terms' | 'privacy'
}>()

const { loading, title, content } = useLegal(() => props.type)
</script>
