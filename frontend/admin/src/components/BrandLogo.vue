<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { adminAPI } from '@/api/admin'
import { getImageUrl } from '@/utils/image'
defineProps<{ compact?: boolean }>()
const configured = ref('')
onMounted(async () => {
  try {
    const res = await adminAPI.getPublicConfig()
    const raw = String(res.data?.data?.brand?.site_logo || '').trim()
    configured.value = raw ? getImageUrl(raw) : ''
  } catch { /* Built-in brand remains available offline. */ }
})
</script>
<template>
  <img v-if="configured" :src="configured" alt="XUEGAOAI" class="h-10 max-w-full object-contain" />
  <img v-else-if="compact" src="/brand/xuegaoai-symbol.svg" alt="XUEGAOAI" class="h-10 w-10 object-contain" />
  <template v-else>
    <img src="/brand/xuegaoai-horizontal.svg" alt="XUEGAOAI" class="brand-logo-light h-10 max-w-full object-contain" />
    <img src="/brand/xuegaoai-horizontal-dark.svg" alt="XUEGAOAI" class="brand-logo-dark h-10 max-w-full object-contain" />
  </template>
</template>
