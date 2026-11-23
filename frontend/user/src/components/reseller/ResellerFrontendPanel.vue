<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { resellerAPI } from '../../api'
import { useConfirmDialog } from '../../composables/useConfirmDialog'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
const { t } = useI18n()
const key = 'resellerFrontend'
const { confirm } = useConfirmDialog()
const props = defineProps<{ domainUrl: string }>()
const redemptionUrl = computed(() => {try{return 'https://redeem.xuegaogx.cc/?site='+encodeURIComponent(new URL(props.domainUrl).hostname)}catch{return ''}})
const state = ref<any>({})
const config = ref<any>(null)
const template = ref('xgai')
const color = ref('#0071e3')
const busy = ref(false)
const error = ref('')
const success = ref('')
const previewUrl = ref('')
const available = ref(false)
const active = computed(() => Boolean(state.value.published_hash))
async function run(action: () => Promise<void>) {
 busy.value = true; error.value = ''; success.value = ''
 try { await action() } catch(e:any) { error.value = e?.message || t(`${key}.failed`) } finally { busy.value = false }
}
async function load() {
 await run(async()=>{
  const [front,site] = await Promise.all([resellerAPI.frontendStatus(),resellerAPI.siteConfig()])
  state.value = front.data.data || {};config.value = site.data.data?.config || {site_name:''}
  available.value=site.data.data?.can_edit===true
  template.value = config.value.theme?.template || 'xgai';color.value = config.value.theme?.primary_color || '#0071e3'
 })
}
async function saveTheme() {
 await run(async()=>{
  if(!/^#[\da-f]{6}$/i.test(color.value)) throw Error(t(`${key}.invalidColor`))
  const latest = (await resellerAPI.siteConfig()).data.data?.config || {site_name:''}
  const response = await resellerAPI.updateSiteConfig({...latest,theme:{template:template.value,primary_color:color.value}})
  config.value=response.data.data;success.value=t(`${key}.themeSaved`)
 })
}
async function upload(event: Event) {
 const input = event.target as HTMLInputElement
 const file = input.files?.[0];if(!file)return
 await run(async()=>{
  if(!file.name.toLowerCase().endsWith('.zip')||file.size>10*1024*1024)throw Error(t(`${key}.limits`))
  const data=new FormData();data.append('file',file)
  const response=await resellerAPI.uploadFrontend(data);state.value=response.data.data;previewUrl.value='';success.value=t(`${key}.uploaded`)
 })
 input.value=''
}
async function preview() {
 await run(async()=>{
  if(!props.domainUrl)throw Error(t(`${key}.needDomain`))
  const response=await resellerAPI.previewFrontend()
  previewUrl.value=props.domainUrl.replace(/\/$/,'')+'/__frontend_preview/'+response.data.data.preview_token+'/'
 })
}
async function publish() {
 if(!await confirm({title:t(`${key}.publish`),message:t(`${key}.publishConfirm`)}))return
 await run(async()=>{await resellerAPI.publishFrontend(state.value.draft_hash);state.value=(await resellerAPI.frontendStatus()).data.data;success.value=t(`${key}.published`)})
}
async function restore() {
 if(!await confirm({title:t(`${key}.restore`),message:t(`${key}.restoreConfirm`)}))return
 await run(async()=>{await resellerAPI.disableFrontend();state.value=(await resellerAPI.frontendStatus()).data.data;success.value=t(`${key}.restored`)})
}
onMounted(load)
</script>
<template>
 <Card class="p-5 sm:p-6 space-y-5">
  <div><h2 class="text-xl font-bold">{{t(`${key}.title`)}}</h2><p class="mt-1 text-sm text-muted-foreground">{{t(`${key}.description`)}}</p></div>
  <p v-if="error" role="alert" class="rounded-lg border border-destructive/40 p-3 text-sm text-destructive">{{error}}</p>
  <p v-if="success" role="status" class="rounded-lg border border-success/40 p-3 text-sm text-success">{{success}}</p>
  <template v-if="available">
   <section v-if="redemptionUrl" class="rounded-xl border p-4 space-y-2"><h3 class="font-semibold">{{t(`${key}.redemption`)}}</h3><p class="text-sm text-muted-foreground">{{t(`${key}.redemptionHint`)}}</p><a :href="redemptionUrl" target="_blank" rel="noopener noreferrer" class="break-all text-primary underline">{{redemptionUrl}}</a></section>
   <section class="rounded-xl border p-4 space-y-4">
    <h3 class="font-semibold">{{t(`${key}.readyMade`)}}</h3>
    <div class="grid gap-4 sm:grid-cols-2">
     <label class="space-y-2"><span class="block text-sm">{{t(`${key}.template`)}}</span><select v-model="template" class="h-10 w-full rounded-lg border bg-background px-3"><option value="xgai">{{t(`${key}.xgai`)}}</option><option value="classic">{{t(`${key}.classic`)}}</option><option value="vault">{{t(`${key}.vault`)}}</option><option value="inherit">{{t(`${key}.inherit`)}}</option></select></label>
     <label class="space-y-2"><span class="block text-sm">{{t(`${key}.color`)}}</span><div class="flex gap-3"><input v-model="color" type="color" class="h-10 w-16 rounded-lg border bg-background" /><input v-model="color" maxlength="7" class="h-10 w-full rounded-lg border bg-background px-3 font-mono" /></div></label>
    </div>
    <div class="rounded-xl border p-4" :style="{'--sample-color':color}"><div class="flex items-center justify-between gap-3"><span class="font-semibold">{{config?.site_name||t(`${key}.yourBrand`)}}</span><span class="rounded-full px-4 py-2 text-sm text-white" style="background:var(--sample-color)">{{t(`${key}.sampleButton`)}}</span></div></div>
    <p class="text-xs text-muted-foreground">{{t(`${key}.themeHint`)}}</p><Button :disabled="busy" @click="saveTheme">{{t(`${key}.saveTheme`)}}</Button>
   </section>
   <section class="rounded-xl border p-4 space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3"><h3 class="font-semibold">{{t(`${key}.custom`)}}</h3><span class="rounded-full bg-muted px-3 py-1 text-xs">{{t(active?`${key}.customActive`:`${key}.platformActive`)}}</span></div>
    <p class="text-sm text-muted-foreground">{{t(`${key}.limits`)}}</p>
    <label class="block"><span class="mb-2 block text-sm">{{t(`${key}.upload`)}}</span><input type="file" accept=".zip,application/zip" :disabled="busy" class="w-full text-sm file:mr-3 file:rounded-lg file:border file:bg-muted file:px-4 file:py-2" @change="upload" /></label>
    <div v-if="state.draft_hash" class="text-xs text-muted-foreground font-mono break-all">{{t(`${key}.draft`)}}: {{state.draft_hash.slice(0,12)}} · {{Math.ceil(state.draft_size/1024)}} KB</div>
    <div class="flex flex-wrap gap-2"><Button variant="outline" :disabled="busy||!state.draft_hash||!domainUrl" @click="preview">{{t(`${key}.preview`)}}</Button><Button :disabled="busy||!state.draft_hash||!domainUrl" @click="publish">{{t(`${key}.publish`)}}</Button><Button variant="outline" :disabled="busy||!active" @click="restore">{{t(`${key}.restore`)}}</Button></div>
    <p v-if="!domainUrl" class="text-sm text-warning">{{t(`${key}.needDomain`)}}</p><p class="text-xs text-muted-foreground">{{t(`${key}.previewHint`)}}</p>
    <iframe v-if="previewUrl" :src="previewUrl" :title="t(`${key}.preview`)" sandbox="allow-scripts" referrerpolicy="no-referrer" class="h-[520px] w-full rounded-xl border bg-white" />
   </section>
   <div class="flex flex-wrap gap-3 text-sm"><a href="/guides/reseller-frontend.md" download class="underline underline-offset-4">{{t(`${key}.manual`)}}</a><a href="/examples/reseller-frontend-starter.zip" download class="underline underline-offset-4">{{t(`${key}.starter`)}}</a></div>
  </template>
 </Card>
</template>
