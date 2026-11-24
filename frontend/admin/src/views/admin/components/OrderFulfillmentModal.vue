<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { AdminOrder } from '@/api/types'
import IdCell from '@/components/IdCell.vue'
import { Button } from '@/components/ui/button'
import { Dialog, DialogScrollContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Card, CardContent } from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'

const props = defineProps<{
  modelValue: boolean
  order: AdminOrder | null
  siteCurrency: string
  parentId?: number | null
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'success', parentId?: number | null): void
}>()

const { t } = useI18n()

const deliveredViaTG = ref(false)
const botStatus = ref<any>(null)
const botLoading = ref(false)
const botPayload = ref('')
const sendViaBot = ref(false)
const loadBotStatus = async (id: number) => {
 botLoading.value = true
 try { botStatus.value = (await adminAPI.getTelegramDelivery(id)).data.data; sendViaBot.value = Boolean(botStatus.value?.enabled) }
 catch { botStatus.value = null }
 finally { botLoading.value = false }
}
const isClaude = computed(() => selectedOrder.value?.items?.some((item) => /claude/i.test(JSON.stringify(item.title))) || false)
const detailLoading = ref(false)
const detailError = ref('')
const selectedOrder = ref<AdminOrder | null>(null)
const fulfillmentSubmitting = ref(false)
const fulfillmentError = ref('')
const fulfillmentSuccess = ref('')
const fulfillmentForm = reactive({
  note: '',
  entries: [] as Array<{ key: string; value: string }>,
})

const createEmptyDeliveryEntry = () => ({ key: '', value: '' })

const addDeliveryEntry = () => {
  fulfillmentForm.entries.push(createEmptyDeliveryEntry())
}

const removeDeliveryEntry = (index: number) => {
  fulfillmentForm.entries.splice(index, 1)
}

const buildDeliveryDataPayload = () => {
  const note = deliveredViaTG.value ? t('admin.orders.tgDeliveryRecord') : String(fulfillmentForm.note || '').trim()
  const entries = fulfillmentForm.entries
    .map((item) => ({
      key: String(item.key || '').trim(),
      value: String(item.value || '').trim(),
    }))
    .filter((item) => item.key || item.value)

  const payload: Record<string, unknown> = {}
  if (note) {
    payload.note = note
  }
  if (entries.length) {
    payload.entries = entries
  }
  return payload
}

const hasFulfillmentSubmitData = () => {
  const data = buildDeliveryDataPayload()
  return Object.keys(data).length > 0
}

const resetFulfillmentForm = () => {
  deliveredViaTG.value = false
 botStatus.value = null
 botPayload.value = ''
 sendViaBot.value = false
  fulfillmentForm.note = ''
  fulfillmentForm.entries = [createEmptyDeliveryEntry()]
  fulfillmentError.value = ''
  fulfillmentSuccess.value = ''
}

const fetchOrderDetail = async (orderId: number) => {
  detailLoading.value = true
  detailError.value = ''
  selectedOrder.value = null
  try {
    const response = await adminAPI.getOrder(orderId)
    selectedOrder.value = response.data.data
 await loadBotStatus(orderId)
  } catch (err: any) {
    detailError.value = err?.message || t('admin.orders.detailFetchFailed')
  } finally {
    detailLoading.value = false
  }
}

const submitFulfillment = async () => {
  if (!selectedOrder.value) return
  fulfillmentError.value = ''
  fulfillmentSuccess.value = ''
  if (sendViaBot.value ? !botPayload.value.trim() : !hasFulfillmentSubmitData()) {
    fulfillmentError.value = t('admin.orders.fulfillmentSubmitRequired')
    return
  }
  fulfillmentSubmitting.value = true
  try {
    if (sendViaBot.value) {
     await adminAPI.sendTelegramDelivery(selectedOrder.value.id, botPayload.value)
    } else await adminAPI.createFulfillment({
      order_id: Number(selectedOrder.value.id),
      delivery_data: buildDeliveryDataPayload(),
    })
    fulfillmentSuccess.value = t('admin.orders.fulfillmentSuccess')
    await loadBotStatus(selectedOrder.value.id)
    emit('success', props.parentId)
  } catch (err: any) {
    fulfillmentError.value = err?.message || t('admin.orders.fulfillmentFailed')
  } finally {
    fulfillmentSubmitting.value = false
  }
}

const handleClose = () => {
  emit('update:modelValue', false)
  selectedOrder.value = null
  detailError.value = ''
  resetFulfillmentForm()
}

// Watch for the order prop to trigger detail fetch
watch(
  () => props.order,
  (newOrder) => {
    if (newOrder && props.modelValue) {
      resetFulfillmentForm()
      fetchOrderDetail(newOrder.id)
    }
  }
)

watch(
  () => props.modelValue,
  (open) => {
    if (open && props.order) {
      resetFulfillmentForm()
      fetchOrderDetail(props.order.id)
    }
    if (!open) {
      handleClose()
    }
  }
)
</script>

<template>
  <Dialog :open="modelValue" @update:open="(value) => { if (!value) handleClose() }">
    <DialogScrollContent class="w-[calc(100vw-1rem)] max-w-2xl p-4 sm:p-6">
      <DialogHeader>
        <DialogTitle>{{ t('admin.orders.fulfillmentModalTitle') }}</DialogTitle>
      </DialogHeader>
      <div class="space-y-4">
        <div v-if="detailLoading" class="h-24 rounded-lg border border-border bg-muted/40 animate-pulse"></div>
        <div v-else-if="detailError" class="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
          {{ detailError }}
        </div>
        <div v-else-if="selectedOrder" class="space-y-4">
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <Card class="rounded-lg border-border bg-background shadow-none">
              <CardContent class="p-3">
                <div class="text-xs text-muted-foreground">{{ t('admin.orders.table.id') }}</div>
                <div class="text-foreground font-mono mt-1">
                  <IdCell :value="selectedOrder.id" />
                </div>
              </CardContent>
            </Card>
            <Card class="rounded-lg border-border bg-background shadow-none">
              <CardContent class="p-3">
                <div class="text-xs text-muted-foreground">{{ t('admin.orders.detailOrderNo') }}</div>
                <div class="text-foreground mt-1 font-mono">{{ selectedOrder.order_no }}</div>
              </CardContent>
            </Card>
          </div>

          <form class="space-y-4" @submit.prevent="submitFulfillment">
            <div v-if="botLoading" class="text-sm text-muted-foreground">{{ t('admin.orders.botLoading') }}</div>
            <div v-if="botStatus?.enabled" class="space-y-3 rounded-xl border border-primary/30 p-4">
              <label class="flex gap-2 text-sm"><input type="checkbox" v-model="sendViaBot" />{{ t('admin.orders.botSend') }}</label>
              <p class="text-sm">{{ botStatus.binding?.bound_at ? t('admin.orders.botBound', { username: [botStatus.binding.username, botStatus.recipient].filter(Boolean).join(' · '), state: botStatus.binding.state }) : t('admin.orders.botUnbound') }}</p>
              <Textarea v-if="sendViaBot" v-model="botPayload" rows="8" maxlength="3000" :placeholder="t('admin.orders.botPayload')" />
              <p class="text-xs text-muted-foreground">{{ t('admin.orders.botHint') }}</p>
            </div>
            <template v-if="!sendViaBot">
            <label v-if="isClaude" class="flex items-start gap-3 rounded-xl border border-primary/30 bg-primary/5 p-4 text-sm">
              <input v-model="deliveredViaTG" type="checkbox" class="mt-1" />
              <span><strong>{{ t('admin.orders.tgDeliveryConfirm') }}</strong><span class="mt-1 block text-muted-foreground">{{ t('admin.orders.tgDeliveryHint') }}</span></span>
            </label>
            <div>
              <label class="block text-xs font-medium text-muted-foreground mb-1.5">{{ t('admin.orders.fulfillmentNote') }}</label>
              <Textarea
                v-model="fulfillmentForm.note"
                rows="3"
                :placeholder="t('admin.orders.fulfillmentNotePlaceholder')"
              />
            </div>

            <div class="rounded-lg border border-border bg-muted/20 p-3 space-y-3">
              <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                <div class="text-xs font-medium text-muted-foreground">{{ t('admin.orders.fulfillmentDeliveryData') }}</div>
                <Button class="w-full sm:w-auto" type="button" size="sm" variant="outline" @click="addDeliveryEntry">
                  {{ t('admin.orders.fulfillmentAddDeliveryField') }}
                </Button>
              </div>
              <div class="space-y-2">
                <div v-for="(entry, entryIndex) in fulfillmentForm.entries" :key="entryIndex" class="grid grid-cols-1 md:grid-cols-[1fr_1fr_auto] gap-2">
                  <Input v-model="entry.key" :placeholder="t('admin.orders.fulfillmentDeliveryKeyPlaceholder')" />
                  <Input v-model="entry.value" :placeholder="t('admin.orders.fulfillmentDeliveryValuePlaceholder')" />
                  <Button class="w-full md:w-auto" type="button" size="sm" variant="destructive" @click="removeDeliveryEntry(entryIndex)">
                    {{ t('admin.common.delete') }}
                  </Button>
                </div>
              </div>
            </div>

            </template>
            <div v-if="fulfillmentError" class="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">
              {{ fulfillmentError }}
            </div>
            <div v-if="fulfillmentSuccess" class="rounded-lg border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-700">
              {{ fulfillmentSuccess }}
            </div>

            <div class="flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
              <Button class="w-full sm:w-auto" type="button" variant="outline" size="sm" @click="resetFulfillmentForm">
                {{ t('admin.common.reset') }}
              </Button>
              <Button class="w-full sm:w-auto" type="submit" size="sm" :disabled="fulfillmentSubmitting || botLoading || (sendViaBot && !botStatus?.binding?.bound_at)">
                {{ fulfillmentSubmitting ? t('admin.orders.fulfillmentSubmitting') : t('admin.orders.fulfillmentSubmit') }}
              </Button>
            </div>
          </form>
        </div>
      </div>
    </DialogScrollContent>
  </Dialog>
</template>
