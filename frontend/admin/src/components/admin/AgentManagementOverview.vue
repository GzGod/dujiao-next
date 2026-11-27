<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { useAdminAuthStore } from '@/stores/auth'
import { ArrowUpRight, KeyRound, Store, Globe2 } from 'lucide-vue-next'
const props = defineProps<{ refreshToken?: number }>()
const { t } = useI18n()
const auth = useAdminAuthStore()
const counts = ref<(number | null)[]>([null,null,null])
const entries = [
 {name:'apiPending',to:'/api-credentials?status=pending_review',permission:'GET:/admin/api-credentials',icon:KeyRound,read:()=>adminAPI.getApiCredentials({status:'pending_review',page_size:1})},
 {name:'sitePending',to:'/resellers/profiles?status=pending_review',permission:'GET:/admin/resellers/profiles',icon:Store,read:()=>adminAPI.getResellerProfiles({status:'pending_review',page_size:1})},
 {name:'siteActive',to:'/resellers/profiles?status=active',permission:'GET:/admin/resellers/profiles',icon:Globe2,read:()=>adminAPI.getResellerProfiles({status:'active',page_size:1})},
]
watch(() => props.refreshToken,async()=>{
 await Promise.allSettled(entries.map(async(entry,i)=>{
  if (!auth.hasPermission(entry.permission)) return
  const response = await entry.read(); counts.value[i] = response.data.pagination?.total ?? 0
 }))
},{immediate:true})
</script>
<template>
  <section class="space-y-3">
    <div class="grid gap-3 sm:grid-cols-3">
      <template v-for="(entry,i) in entries" :key="entry.name"><router-link v-if="auth.hasPermission(entry.permission)" :to="entry.to" class="group rounded-xl border border-border bg-card p-4 transition-colors hover:border-primary/40 hover:bg-primary/5"><div class="flex items-center justify-between gap-3"><span class="flex items-center gap-2 text-sm text-muted-foreground"><component :is="entry.icon" class="h-4 w-4" />{{t('agentManagement.'+entry.name)}}</span><ArrowUpRight class="h-4 w-4 text-muted-foreground group-hover:text-primary" /></div><div class="mt-3 text-3xl font-semibold tabular-nums">{{counts[i] ?? '—'}}</div></router-link></template>
    </div>
    <p class="text-sm leading-6 text-muted-foreground">{{t('agentManagement.distinction')}}</p>
  </section>
</template>
