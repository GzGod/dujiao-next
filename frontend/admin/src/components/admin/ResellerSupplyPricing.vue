<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { AdminResellerProfile, ResellerSupplyPricing } from '@/api/types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { getLocalizedText } from '@/utils/format'
import { isSupplyProductAllowed } from '@/utils/resellerReadiness'
import { notifyError, notifySuccess } from '@/utils/notify'
const props = defineProps<{ profile: AdminResellerProfile }>()
const emit = defineEmits<{ saved: [] }>()
const { t } = useI18n()
const key = 'admin.resellerSupply'
const grade = ref(props.profile.supply_pricing?.grade || 'custom')
const saving = ref(false)
const loaded = ref(false)
const products = ref<any[]>([])
const search = ref('')
const blockedProductIDs = ref<number[]>([...(props.profile.supply_pricing?.blocked_product_ids || [])])
const allowedProductIDs = ref<number[] | undefined>(props.profile.supply_pricing?.allowed_product_ids == null ? undefined : [...props.profile.supply_pricing.allowed_product_ids])
const canSell = (id: number) => isSupplyProductAllowed({ allowed_product_ids: allowedProductIDs.value, blocked_product_ids: blockedProductIDs.value }, id)
const setCanSell = (id: number, enabled: boolean) => {
 if (allowedProductIDs.value != null) allowedProductIDs.value = enabled ? [...new Set([...allowedProductIDs.value, id])] : allowedProductIDs.value.filter(value => value !== id)
 blockedProductIDs.value = enabled ? blockedProductIDs.value.filter(value => value !== id) : [...new Set([...blockedProductIDs.value, id])]
}
const catalogRows = computed(() => products.value.flatMap(p => (p.skus?.length ? p.skus : [{ id: 0, sku_code: '', price_amount: p.price_amount, cost_price_amount: p.cost_price_amount }]).map((sku: any) => ({ product: p, sku }))).filter(row => `${title(row.product)} ${row.sku.sku_code}`.toLowerCase().includes(search.value.toLowerCase())))
const fixedRule = (productId: number, skuId: number) => rules.value.find(r => r.product_id === productId && r.sku_id === skuId && r.mode === 'fixed')
const setFixedPrice = (productId: number, skuId: number, value: string | number) => {
 const index = rules.value.findIndex(r => r.product_id === productId && r.sku_id === skuId)
 const amount = String(value).trim()
 if (!amount) { if (index >= 0) rules.value.splice(index, 1); return }
 const rule: ResellerSupplyPricing['overrides'][number] = { product_id: productId, sku_id: skuId, mode: 'fixed', value: amount }
 if (index >= 0) rules.value[index] = rule
 else rules.value.push(rule)
}

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
   supply_pricing: { grade: grade.value === 'custom' ? '' : grade.value as ResellerSupplyPricing['grade'], allowed_product_ids: allowedProductIDs.value, blocked_product_ids: blockedProductIDs.value, default_markup_percent: defaultMarkup.value === '' ? null : defaultMarkup.value, overrides: rules.value },
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
  await Promise.all(products.value.map(p => loadProduct(p.id)))
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
  <div class="max-w-md space-y-2"><Label>{{t('admin.resellerGrade.title')}}</Label><Select v-model="grade"><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem v-for="value in ['','basic','intermediate','advanced']" :key="value" :value="value || 'custom'">{{t('admin.resellerGrade.'+(value||'custom'))}}</SelectItem></SelectContent></Select><p class="text-xs text-muted-foreground">{{t('admin.resellerGrade.hint')}}</p></div>
  <div v-if="!grade || grade === 'custom'" class="max-w-md space-y-2"><Label>{{ t(`${key}.default`) }}</Label><Input v-model="defaultMarkup" type="number" min="0" step="0.01" :placeholder="t(`${key}.inherit`)" /><p class="text-xs text-muted-foreground">{{ t(`${key}.unknownCost`) }} {{t('admin.resellerGrade.basis')}}</p></div>
  <p class="text-sm font-medium">{{ t(`${key}.owner`, { id: profile.user_id, email: profile.user?.email || '' }) }}</p>
  <Input v-model="search" :placeholder="t(`${key}.search`)" :aria-label="t(`${key}.search`)" />
  <div class="overflow-auto rounded-lg border border-border max-h-[560px]">
   <table class="w-full text-sm">
    <thead class="sticky top-0 bg-card"><tr class="border-b border-border"><th class="p-3 text-left">{{ t(`${key}.product`) }}</th><th class="p-3 text-left">{{ t(`${key}.sku`) }}</th><th class="p-3 text-left min-w-[150px]">{{ t(`${key}.availability`) }}</th><th class="p-3 text-right">{{ t(`${key}.retail`) }}</th><th class="p-3 text-right">{{ t(`${key}.cost`) }}</th><th class="p-3 text-left min-w-[200px]">{{ t(`${key}.fixed`) }}</th></tr></thead>
    <tbody><tr v-for="row in catalogRows" :key="`${row.product.id}-${row.sku.id}`" class="border-b border-border last:border-0">
     <td class="p-3">{{ title(row.product) }}</td><td class="p-3">{{ row.sku.sku_code || t(`${key}.allSkus`) }}</td><td class="p-3"><div class="flex items-center gap-2"><Switch :model-value="canSell(row.product.id)" :aria-label="`${title(row.product)} ${t(`${key}.availability`)}`" @update:model-value="value => setCanSell(row.product.id, Boolean(value))" /><span :class="canSell(row.product.id) ? 'text-foreground' : 'text-destructive'">{{ t(`${key}.${canSell(row.product.id) ? 'allowed' : 'blocked'}`) }}</span></div></td><td class="p-3 text-right font-mono">{{ row.sku.price_amount }}</td><td class="p-3 text-right font-mono">{{ row.sku.cost_price_amount }}</td>
     <td class="p-3"><Input type="number" min="0.01" step="0.01" :aria-label="`${title(row.product)} ${row.sku.sku_code} ${t(`${key}.fixed`)}`" :model-value="fixedRule(row.product.id, row.sku.id)?.value ?? ''" :placeholder="t(`${key}.tableInherit`)" @update:model-value="value => setFixedPrice(row.product.id, row.sku.id, value)" /></td>
    </tr><tr v-if="loaded && !catalogRows.length"><td colspan="6" class="p-4 text-muted-foreground">{{ t(`${key}.empty`) }}</td></tr></tbody>
   </table>
  </div>
  <p class="text-xs text-muted-foreground">{{ t(`${key}.tableHint`) }}</p>
  <p class="text-xs text-muted-foreground">{{ t(`${key}.availabilityHint`) }}</p>
  <details><summary class="cursor-pointer text-sm font-medium">{{ t(`${key}.advanced`) }}</summary>
  <div v-for="(rule, index) in rules" :key="index" class="rounded-lg border border-border p-3 grid gap-3 md:grid-cols-2 xl:grid-cols-5">
   <div class="space-y-2"><Label>{{ t(`${key}.product`) }}</Label><Select :model-value="String(rule.product_id)" @update:model-value="value => changeProduct(rule, value)"><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem v-for="p in products" :key="p.id" :value="String(p.id)">{{ title(p) }}</SelectItem></SelectContent></Select></div>
   <div class="space-y-2"><Label>{{ t(`${key}.sku`) }}</Label><Select :model-value="String(rule.sku_id)" @update:model-value="value => rule.sku_id = Number(value)"><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value="0">{{ t(`${key}.allSkus`) }}</SelectItem><SelectItem v-for="s in skusFor(rule.product_id)" :key="s.id" :value="String(s.id)">{{ s.sku_code || `#${s.id}` }}</SelectItem></SelectContent></Select></div>
   <div class="space-y-2"><Label>{{ t(`${key}.mode`) }}</Label><Select v-model="rule.mode"><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value="fixed">{{ t(`${key}.fixed`) }}</SelectItem><SelectItem value="cost_markup">{{ t(`${key}.markup`) }}</SelectItem></SelectContent></Select></div>
   <div class="space-y-2"><Label>{{ rule.mode === 'fixed' ? t(`${key}.amount`) : t(`${key}.percent`) }}</Label><Input v-model="rule.value" type="number" min="0" step="0.01" /></div>
   <div class="flex items-end"><Button variant="outline" @click="rules.splice(index, 1)">{{ t(`${key}.remove`) }}</Button></div>
  </div>
  <Button class="mt-3" variant="outline" :disabled="!loaded" @click="addRule">{{ t(`${key}.add`) }}</Button></details>
  <div class="flex flex-wrap gap-2"><Button :disabled="saving || !loaded || !valid" @click="save">{{ t(`${key}.save`) }}</Button></div>
  <p class="text-xs text-muted-foreground">{{ t(`${key}.priority`) }}</p>
 </section>
</template>
