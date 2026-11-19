<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Copy, RefreshCw } from 'lucide-vue-next'
import { adminAPI } from '@/api/admin'
import type { AdminOrder, AdminProcurementOrder } from '@/api/types'
import { Button } from '@/components/ui/button'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { formatDate, getLocalizedText } from '@/utils/format'
import { adminUrl } from '@/utils/adminBase'
import { copyText } from '@/utils/clipboard'
import { notifyError } from '@/utils/notify'
import { formatSkuDisplayLabel } from '@/utils/sku'
import { issuedCardRecords } from '@/utils/issuedCards'
import TableSkeleton from '@/components/TableSkeleton.vue'

const props = defineProps<{ orders: AdminOrder[]; loading: boolean }>()
const emit = defineEmits<{ (e: 'view', order: AdminOrder): void }>()
const { t, te, locale } = useI18n()
const rows = computed(() => issuedCardRecords(props.orders))
const upstreamRows = computed(() => rows.value.filter(row => row.fulfillment.type === 'upstream'))
const states = ref<Record<number, { statuses: string[]; error: boolean; checked: string }>>({})
const refreshing = ref(false)
const copied = ref('')
let generation = 0
let timer: ReturnType<typeof setTimeout> | undefined
let copyTimer: ReturnType<typeof setTimeout> | undefined
const statusLabel = (status: string) => t(te(`redemption.${status}`) ? `redemption.${status}` : 'redemption.unknown')
function schedule() {
  if (!upstreamRows.value.length) return
  timer = setTimeout(() => { if (!document.hidden) void refreshProgress(); else schedule() }, 30000)
}
async function refreshProgress() {
  if (refreshing.value) return
  const current = generation
  const queue = [...new Map(upstreamRows.value.map(row => [row.order.id, row.order])).values()]
  refreshing.value = true
  clearTimeout(timer)
  async function worker() {
    while (queue.length && current === generation) {
      const order = queue.shift()!
      try {
        const response = await adminAPI.getProcurementOrders({ order_no: order.order_no, page_size: 100 })
        if (current !== generation) return
        const procurements = (response.data.data || []) as AdminProcurementOrder[]
        const statuses: string[] = []
        let error = false
        for (const procurement of procurements.filter(p => p.upstream_order_no)) {
          if (current !== generation) return
          try {
            const result = await adminAPI.getProcurementRedemption(procurement.id)
            statuses.push(String(result.data.data?.progress_status || 'unknown'))
          } catch { error = true }
        }
        if (current === generation) states.value[order.id] = { statuses, error, checked: new Date().toLocaleTimeString() }
      } catch {
        if (current === generation) states.value[order.id] = { statuses: [], error: true, checked: new Date().toLocaleTimeString() }
      }
    }
  }
  await Promise.all(Array.from({ length: Math.min(3, queue.length) }, worker))
  if (current === generation) { refreshing.value = false; schedule() }
}
async function copy(content: string, key: string) {
  try {
    await copyText(content)
    copied.value = key
    clearTimeout(copyTimer)
    copyTimer = setTimeout(() => { copied.value = '' }, 2000)
  } catch { notifyError(t('admin.common.copyFailed')) }
}
watch(() => props.orders, () => {
  generation++; clearTimeout(timer); states.value = {}; refreshing.value = false; copied.value = ''; void refreshProgress()
}, { immediate: true })
onUnmounted(() => { generation++; clearTimeout(timer); clearTimeout(copyTimer) })
</script>

<template>
  <div class="space-y-3">
    <div class="flex flex-wrap items-center justify-between gap-3 px-4 pt-4">
      <p class="max-w-3xl text-xs leading-relaxed text-muted-foreground">{{ t('issuedCards.hint') }}</p>
      <Button v-if="upstreamRows.length" size="sm" variant="outline" :disabled="refreshing || loading" @click="refreshProgress">
        <RefreshCw class="size-3.5" :class="{ 'animate-spin': refreshing }" />{{ t('redemption.refresh') }}
      </Button>
    </div>
    <Table class="min-w-[1120px]">
      <TableHeader>
        <TableRow>
          <TableHead class="px-4">{{ t('admin.orders.table.orderNo') }}</TableHead>
          <TableHead class="px-4">{{ t('admin.orders.table.items') }}</TableHead>
          <TableHead class="px-4">{{ t('issuedCards.buyer') }}</TableHead>
          <TableHead class="px-4">{{ t('issuedCards.content') }}</TableHead>
          <TableHead class="px-4">{{ t('issuedCards.deliveredAt') }}</TableHead>
          <TableHead class="px-4">{{ t('redemption.title') }}</TableHead>
          <TableHead class="px-4 text-right">{{ t('admin.orders.table.action') }}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        <TableRow v-if="loading"><TableCell :colspan="7" class="p-0"><TableSkeleton :columns="7" :rows="5" /></TableCell></TableRow>
        <TableRow v-else-if="!rows.length"><TableCell :colspan="7" class="p-8 text-center text-muted-foreground">{{ t('issuedCards.empty') }}</TableCell></TableRow>
        <TableRow v-for="row in loading ? [] : rows" :key="row.key">
          <TableCell class="min-w-40 px-4 align-top">
            <button class="text-left font-mono text-xs text-primary hover:underline break-all" @click="emit('view', row.buyer)">{{ row.buyer.order_no }}</button>
            <div v-if="row.order.id !== row.buyer.id" class="mt-1 break-all font-mono text-xs text-muted-foreground">{{ row.order.order_no }}</div>
          </TableCell>
          <TableCell class="min-w-36 px-4 align-top">
            <div v-for="item in row.order.items || []" :key="item.id" class="text-xs">{{ getLocalizedText(item.title) || '-' }} <span v-if="formatSkuDisplayLabel(item.sku_snapshot, locale)" class="text-muted-foreground">({{ formatSkuDisplayLabel(item.sku_snapshot, locale) }})</span> <span class="text-muted-foreground">×{{ item.quantity }}</span></div>
          </TableCell>
          <TableCell class="min-w-44 px-4 align-top text-xs">
            <template v-if="row.buyer.user_id">
              <a :href="adminUrl(`/users/${row.buyer.user_id}`)" target="_blank" rel="noopener" class="text-primary hover:underline">{{ row.buyer.user_display_name || `#${row.buyer.user_id}` }}</a>
              <div class="mt-1 break-all text-muted-foreground">{{ row.buyer.user_email || '-' }}</div>
            </template>
            <template v-else><span class="text-muted-foreground">{{ t('admin.orders.guestLabel') }}</span><div class="mt-1 break-all">{{ row.buyer.guest_email || '-' }}</div></template>
          </TableCell>
          <TableCell class="min-w-64 max-w-96 px-4 align-top">
            <div v-if="row.content" class="space-y-2">
              <pre class="max-h-32 overflow-y-auto whitespace-pre-wrap break-all rounded-xl border border-border bg-muted/40 p-3 font-mono text-xs">{{ row.content }}</pre>
              <Button size="xs" variant="outline" @click="copy(row.content, row.key)"><Copy class="size-3" />{{ copied === row.key ? t('issuedCards.copied') : t('admin.common.copy') }}</Button>
            </div>
            <span v-else class="text-xs text-muted-foreground">{{ t('issuedCards.noContent') }}</span>
          </TableCell>
          <TableCell class="whitespace-nowrap px-4 align-top text-xs text-muted-foreground">{{ formatDate(row.fulfillment.delivered_at || row.fulfillment.created_at) }}</TableCell>
          <TableCell class="min-w-44 px-4 align-top text-xs">
            <span v-if="row.fulfillment.type !== 'upstream'" class="text-muted-foreground">{{ t('issuedCards.unsupported') }}</span>
            <template v-else-if="states[row.order.id]">
              <div v-for="(status, index) in states[row.order.id]!.statuses" :key="index" class="mb-1 inline-flex rounded-full bg-accent px-2.5 py-1 text-accent-foreground">{{ statusLabel(status) }}</div>
              <p v-if="states[row.order.id]!.error" class="text-destructive">{{ t('redemption.error') }}</p>
              <p v-else-if="!states[row.order.id]!.statuses.length" class="text-muted-foreground">{{ t('redemption.unknown') }}</p>
              <p class="mt-1 text-[11px] text-muted-foreground">{{ t('redemption.checked') }} {{ states[row.order.id]!.checked }}</p>
            </template>
            <span v-else class="text-muted-foreground" role="status">{{ t('issuedCards.loading') }}</span>
          </TableCell>
          <TableCell class="px-4 align-top text-right"><Button size="xs" variant="outline" @click="emit('view', row.buyer)">{{ t('admin.orders.view') }}</Button></TableCell>
        </TableRow>
      </TableBody>
    </Table>
  </div>
</template>
