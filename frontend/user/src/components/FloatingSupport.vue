<template>
  <a v-if="supportUrl" :href="supportUrl" target="_blank" rel="noopener noreferrer" class="floating-support" :aria-label="t('floatingSupport.label')">
    <span class="floating-support__icon"><MessageCircle :size="22" aria-hidden="true" /></span>
    <span class="floating-support__copy"><strong>{{ t('floatingSupport.title') }}</strong><small>Telegram ↗</small></span>
  </a>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessageCircle } from 'lucide-vue-next'
import { useAppStore } from '../stores/app'

const { t } = useI18n()
const appStore = useAppStore()
const supportUrl = computed(() => {
  const configured = String(appStore.config?.contact?.telegram || '').trim()
  if (/^https:\/\/(?:t\.me|telegram\.me)\//i.test(configured)) return configured
  return appStore.isResellerTenant ? '' : 'https://t.me/xuegaozhanshen'
})
</script>

<style scoped>
.floating-support { position: fixed; right: max(20px, env(safe-area-inset-right)); top: 58%; z-index: 45; display: flex; align-items: center; gap: 10px; padding: 12px 17px 12px 12px; border: 1px solid rgba(255,255,255,.3); border-radius: 18px; background: #1768d8; color: #fff; box-shadow: 0 8px 28px rgba(0,65,160,.25); text-decoration: none; transition: background .15s, box-shadow .15s; }
.floating-support:hover { background: #105bc3; box-shadow: 0 10px 32px rgba(0,65,160,.35); }
.floating-support:focus-visible { outline: 3px solid #8cc5ff; outline-offset: 4px; }
.floating-support__icon { display: grid; place-items: center; width: 38px; height: 38px; border-radius: 12px; background: rgba(255,255,255,.15); }
.floating-support__copy { display: flex; flex-direction: column; gap: 3px; }
.floating-support strong { color: #fff; font-size: 14px; font-weight: 650; line-height: 1.3; }
.floating-support small { color: #e4f0ff; font-size: 11px; line-height: 1.3; }
@media (max-width: 640px) {
  .floating-support { right: max(10px, env(safe-area-inset-right)); top: auto; bottom: calc(130px + env(safe-area-inset-bottom)); gap: 6px; padding: 10px; border-radius: 15px; }
  .floating-support__icon { width: 30px; height: 30px; border-radius: 9px; }
  .floating-support strong { font-size: 12px; }
  .floating-support small { font-size: 10px; }
}
</style>
