<script setup lang="ts">
import { onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { RefreshCw } from 'lucide-vue-next'
import { adminAPI } from '@/api/admin'
import type { AdminOrder, AdminProcurementOrder } from '@/api/types'
import { Button } from '@/components/ui/button'
const props = defineProps<{ order: AdminOrder }>()
const { t, te } = useI18n()
const rows = ref<{ id: number; orderNo: string; status: string; error: boolean }[]>([])
const loading = ref(false)
const failed = ref(false)
const checked = ref('')
let generation = 0
let timer: ReturnType<typeof setTimeout> | undefined
const label = (status: string) => t(te(`redemption.${status}`) ? `redemption.${status}` : 'redemption.unknown')
function schedule() { timer = setTimeout(() => { if (!document.hidden) void refresh(); else schedule() }, 30000) }
async function refresh() {
  if (loading.value) return
  const current = generation
  loading.value = true
  failed.value = false
  try {
    const orders = props.order.children?.length ? props.order.children : [props.order]
    const lists = await Promise.all(orders.map(async order => {
      const response = await adminAPI.getProcurementOrders({ order_no: order.order_no, page_size: 100 })
      return (response.data.data || []) as AdminProcurementOrder[]
    }))
    const results = await Promise.all(lists.flat().filter(p => p.upstream_order_no).map(async p => {
      try {
        const response = await adminAPI.getProcurementRedemption(p.id)
        return { id: p.id, orderNo: p.local_order_no, status: response.data.data?.progress_status || 'unknown', error: false }
      } catch { return { id: p.id, orderNo: p.local_order_no, status: 'unknown', error: true } }
    }))
    if (current === generation) { rows.value = results; checked.value = new Date().toLocaleTimeString() }
  } catch { if (current === generation) failed.value = true }
  finally { if (current === generation) { loading.value = false; clearTimeout(timer); schedule() } }
}
watch(() => props.order.id, () => {
  generation++; clearTimeout(timer); loading.value = false; rows.value = []; checked.value = ''; void refresh()
}, { immediate: true })
onUnmounted(() => { generation++; clearTimeout(timer) })
</script>
<template>
  <section class="rounded-2xl border border-border bg-card p-4 space-y-3">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <h3 class="text-sm font-semibold">{{ t('redemption.title') }}</h3>
      <Button variant="outline" size="sm" :disabled="loading" @click="refresh"><RefreshCw class="size-3.5" :class="{ 'animate-spin': loading }" />{{ t('redemption.refresh') }}</Button>
    </div>
    <p class="text-xs text-muted-foreground">{{ t('redemption.hint') }}</p>
    <p v-if="failed" role="status" class="text-sm text-destructive">{{ t('redemption.error') }}</p>
    <p v-else-if="!rows.length && !loading" class="text-sm text-muted-foreground">{{ t('redemption.notApplicable') }}</p>
    <div v-for="row in rows" :key="row.id" class="flex flex-wrap justify-between items-center gap-2 rounded-xl bg-muted/50 px-3 py-3">
      <span class="font-mono text-xs break-all">{{ row.orderNo }}</span>
      <span class="rounded-full bg-accent px-3 py-1 text-xs font-medium text-accent-foreground" :class="{ 'text-destructive': row.error }">{{ row.error ? t('redemption.error') : label(row.status) }}</span>
    </div>
    <p v-if="checked" class="text-xs text-muted-foreground">{{ t('redemption.checked') }} · {{ checked }}</p>
  </section>
</template>
