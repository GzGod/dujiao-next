<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { AdminResellerProfile, ResellerSupplyPricing } from '@/api/types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { getLocalizedText } from '@/utils/format'
import { notifyError, notifySuccess } from '@/utils/notify'
const props = defineProps<{ profile: AdminResellerProfile }>()
const emit = defineEmits<{ saved: [] }>()
const { t } = useI18n()
const key = 'admin.resellerSupply'
const saving = ref(false)
const loaded = ref(false)
const products = ref<any[]>([])
const defaultMarkup = ref(props.profile.supply_pricing?.default_markup_percent ?? '')
const rules = ref<ResellerSupplyPricing['overrides']>(JSON.parse(JSON.stringify(props.profile.supply_pricing?.overrides || [])))
const productFor = (id: number) => products.value.find(p => p.id === id)
const skusFor = (id: number) => productFor(id)?.skus || []
const title = (p: any) => getLocalizedText(p?.title || p?.title_json) || `#${p?.id}`
const addRule = async () => {
 const id = products.value[0]?.id || 0
 rules.value.push({ product_id: id, sku_id: 0, mode: 'fixed', value: '' })
 try { await loadProduct(id) } catch (e: any) { notifyError(e?.message || t(`${key}.failed`)) }
}
const valid = computed(() => (defaultMarkup.value === '' || (Number.isFinite(Number(defaultMarkup.value)) && Number(defaultMarkup.value) >= 0)) && rules.value.every(r => r.product_id > 0 && r.value.trim() !== '' && Number.isFinite(Number(r.value)) && Number(r.value) >= 0 && (r.mode !== 'fixed' || Number(r.value) > 0)))
const save = async () => {
 if (!valid.value) { notifyError(t(`${key}.invalid`)); return }
 saving.value = true
 try {
  await adminAPI.updateResellerProfile(props.profile.id, {
   default_markup_percent: props.profile.default_markup_percent,
   max_markup_percent: props.profile.max_markup_percent,
   settlement_status: props.profile.settlement_status,
   supply_pricing: { default_markup_percent: defaultMarkup.value === '' ? null : defaultMarkup.value, overrides: rules.value },
  })
  notifySuccess(t(`${key}.saved`)); emit('saved')
 } catch (e: any) { notifyError(e?.message || t(`${key}.failed`)) }
 finally { saving.value = false }
}
onMounted(async () => {
 try {
  let page = 1
  while (true) {
   const res = await adminAPI.getProducts({ page, page_size: 100 })
   const data = res.data.data
   const rows = Array.isArray(data) ? data : data?.items || []
   products.value.push(...rows)
   if (rows.length < 100) break
   page++
  }
  // The catalogue list may omit SKUs; load them when a rule is selected.
  for (const r of rules.value) await loadProduct(r.product_id)
  loaded.value = true
 } catch (e: any) { notifyError(e?.message || t(`${key}.failed`)) }
})
const loadProduct = async (id: number) => {
 if (!id) return
 const res = await adminAPI.getProduct(id)
 const p = res.data.data
 const index = products.value.findIndex(v => v.id === id)
 if (index >= 0) products.value[index] = p
 else products.value.push(p)
}
const changeProduct = async (rule: ResellerSupplyPricing['overrides'][number], value: unknown) => {
 rule.product_id = Number(value); rule.sku_id = 0
 try { await loadProduct(rule.product_id) } catch (e: any) { notifyError(e?.message || t(`${key}.failed`)) }
}
</script>

<template>
 <section class="rounded-xl border border-border bg-card p-4 space-y-4">
  <div><h3 class="text-sm font-medium">{{ t(`${key}.title`) }}</h3><p class="mt-1 text-sm text-muted-foreground">{{ t(`${key}.hint`) }}</p></div>
  <div class="max-w-md space-y-2"><Label>{{ t(`${key}.default`) }}</Label><Input v-model="defaultMarkup" type="number" min="0" step="0.01" :placeholder="t(`${key}.inherit`)" /><p class="text-xs text-muted-foreground">{{ t(`${key}.unknownCost`) }}</p></div>
  <div v-for="(rule, index) in rules" :key="index" class="rounded-lg border border-border p-3 grid gap-3 md:grid-cols-2 xl:grid-cols-5">
   <div class="space-y-2"><Label>{{ t(`${key}.product`) }}</Label><Select :model-value="String(rule.product_id)" @update:model-value="value => changeProduct(rule, value)"><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem v-for="p in products" :key="p.id" :value="String(p.id)">{{ title(p) }}</SelectItem></SelectContent></Select></div>
   <div class="space-y-2"><Label>{{ t(`${key}.sku`) }}</Label><Select :model-value="String(rule.sku_id)" @update:model-value="value => rule.sku_id = Number(value)"><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value="0">{{ t(`${key}.allSkus`) }}</SelectItem><SelectItem v-for="s in skusFor(rule.product_id)" :key="s.id" :value="String(s.id)">{{ s.sku_code || `#${s.id}` }}</SelectItem></SelectContent></Select></div>
   <div class="space-y-2"><Label>{{ t(`${key}.mode`) }}</Label><Select v-model="rule.mode"><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value="fixed">{{ t(`${key}.fixed`) }}</SelectItem><SelectItem value="cost_markup">{{ t(`${key}.markup`) }}</SelectItem></SelectContent></Select></div>
   <div class="space-y-2"><Label>{{ rule.mode === 'fixed' ? t(`${key}.amount`) : t(`${key}.percent`) }}</Label><Input v-model="rule.value" type="number" min="0" step="0.01" /></div>
   <div class="flex items-end"><Button variant="outline" @click="rules.splice(index, 1)">{{ t(`${key}.remove`) }}</Button></div>
  </div>
  <div class="flex flex-wrap gap-2"><Button variant="outline" :disabled="!loaded" @click="addRule">{{ t(`${key}.add`) }}</Button><Button :disabled="saving || !valid" @click="save">{{ t(`${key}.save`) }}</Button></div>
  <p class="text-xs text-muted-foreground">{{ t(`${key}.priority`) }}</p>
 </section>
</template>
