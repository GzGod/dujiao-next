<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import ListPagination from '@/components/ListPagination.vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { formatDate } from '@/utils/format'

interface Evidence {
  id: number
  created_at: string
  code_hash: string
  operation: string
  outcome: string
  client_ip: string
  user_agent: string
  target_email?: string
}

const props = defineProps<{ initialOrderNo?: string }>()
const { t } = useI18n()
const label = (key: string) => t(`admin.orders.evidence.${key}`)
const columns = ['time', 'stage', 'outcome', 'digest', 'ip', 'agent', 'email']
const digest = ref('')
const activeDigest = ref('')
const orderNo = ref(props.initialOrderNo || '')
const activeOrderNo = ref(orderNo.value)
const loading = ref(false)
const error = ref('')
const rows = ref<Evidence[]>([])
const pagination = ref({ page: 1, total: 0, total_page: 1 })

async function load(page = 1) {
  if (loading.value) return
  loading.value = true
  error.value = ''
  rows.value = []
  try {
    const response = await adminAPI.getRedemptionEvidence({ page, code_hash: activeDigest.value || undefined, order_no: activeOrderNo.value || undefined })
    rows.value = response.data.data || []
    pagination.value = response.data.pagination
  } catch {
    error.value = label('error')
    pagination.value = { page: 1, total: 0, total_page: 1 }
  } finally {
    loading.value = false
  }
}

function search() {
  if (loading.value) return
  const value = digest.value.trim()
  if (value && !/^[a-f0-9]{64}$/.test(value)) {
    error.value = label('invalid')
    return
  }
  activeDigest.value = value
  activeOrderNo.value = orderNo.value.trim()
  void load()
}

onMounted(() => void load())
</script>

<template>
  <section class="space-y-4 rounded-xl border border-border bg-card p-4">
    <form class="flex flex-wrap gap-2" @submit.prevent="search">
      <Input v-model="digest" class="w-full sm:w-96" :aria-label="label('digest')" :placeholder="label('digest')" maxlength="64" />
      <Input v-model="orderNo" class="w-full sm:w-52" :aria-label="t('admin.orders.table.orderNo')" :placeholder="t('admin.orders.table.orderNo')" maxlength="64" />
      <Button type="submit" :disabled="loading">{{ label('search') }}</Button>
    </form>
    <p v-if="error" role="alert" class="text-sm text-destructive">{{ error }}</p>
    <Table class="min-w-[920px]" :aria-busy="loading">
      <TableHeader>
        <TableRow>
          <TableHead v-for="column in columns" :key="column">{{ label(column) }}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        <TableRow v-for="row in rows" :key="row.id">
          <TableCell class="whitespace-nowrap">{{ formatDate(row.created_at) }}</TableCell>
          <TableCell>{{ label(row.operation) }}</TableCell>
          <TableCell>{{ label(row.outcome) }}</TableCell>
          <TableCell class="max-w-48 break-all font-mono text-xs">{{ row.code_hash }}</TableCell>
          <TableCell>{{ row.client_ip }}</TableCell>
          <TableCell class="max-w-64 break-all text-xs">{{ row.user_agent || '—' }}</TableCell>
          <TableCell class="max-w-48 break-all">{{ row.target_email || '—' }}</TableCell>
        </TableRow>
        <TableRow v-if="!loading && !error && !rows.length">
          <TableCell :colspan="columns.length" class="py-8 text-center text-muted-foreground">{{ label('empty') }}</TableCell>
        </TableRow>
      </TableBody>
    </Table>
    <ListPagination :page="pagination.page" :total-page="pagination.total_page" :total="pagination.total" @change-page="load" />
  </section>
</template>
