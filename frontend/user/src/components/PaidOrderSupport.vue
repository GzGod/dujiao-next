<template>
  <div v-if="isPaid" class="my-4 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-primary/20 bg-primary/5 p-4">
    <span class="text-sm text-foreground">{{ t(deliveryURL ? 'paidSupport.botPrompt' : isClaude ? 'paidSupport.claudePrompt' : 'paidSupport.prompt') }}</span>
    <a :href="deliveryURL || url" target="_blank" rel="noopener noreferrer" class="font-semibold text-primary underline">{{ t(deliveryURL ? 'paidSupport.botClaim' : 'paidSupport.contact') }}</a>
  </div>
  <DialogRoot v-model:open="open"><DialogPortal>
    <DialogOverlay class="fixed inset-0 z-[120] bg-black/60 backdrop-blur-sm" />
    <DialogContent class="fixed left-1/2 top-1/2 z-[121] w-[calc(100%-2rem)] max-w-sm -translate-x-1/2 -translate-y-1/2 rounded-2xl border border-border bg-background p-6 text-foreground shadow-xl">
      <DialogTitle class="text-xl font-bold">{{ t(isClaude ? 'paidSupport.claudeTitle' : 'paidSupport.title') }}</DialogTitle>
      <DialogDescription class="mt-3 text-sm leading-relaxed text-muted-foreground">{{ t(deliveryURL ? 'paidSupport.botPrompt' : isClaude ? 'paidSupport.claudePrompt' : 'paidSupport.prompt') }}</DialogDescription>
      <a :href="deliveryURL || url" target="_blank" rel="noopener noreferrer" class="mt-6 flex w-full items-center justify-center rounded-xl bg-primary px-4 py-3 font-semibold text-primary-foreground" @click="open=false">{{ t(deliveryURL ? 'paidSupport.botClaim' : 'paidSupport.contact') }} · Telegram</a>
      <DialogClose class="mt-3 w-full rounded-xl border border-border px-4 py-2 text-sm">{{ t('paidSupport.dismiss') }}</DialogClose>
    </DialogContent>
  </DialogPortal></DialogRoot>
</template>
<script setup lang="ts">
import { computed,ref,watch } from 'vue'
import { useRoute } from 'vue-router'
import { userApi } from '@/api/client'
import { loadGuestOrderAuth } from '@/utils/guestOrderAuth'
import { hasClaudeItems, type SupportOrder } from '@/utils/claudeOrder'
import { useI18n } from 'vue-i18n'
import { DialogRoot,DialogPortal,DialogOverlay,DialogContent,DialogTitle,DialogDescription,DialogClose } from 'reka-ui'
const props=defineProps<{order:SupportOrder|null|undefined}>()
const {t}=useI18n()
const url='https://t.me/xuegaozhanshen'
const route=useRoute()
const deliveryURL=ref('')
let deliverySeq=0
watch(()=>[props.order?.order_no,props.order?.status] as const,async ([no,status])=>{
 const seq=++deliverySeq
 deliveryURL.value=''
 if(!no||!['paid','fulfilling'].includes(status||'')) return
 const guest=route.path.startsWith('/guest/')||route.query.guest==='1'
 const auth=loadGuestOrderAuth()
 const headers:Record<string,string>={}
 if(guest){
  if(!auth.email||!auth.order_password)return
  const bytes=new TextEncoder().encode(`${auth.email.trim().toLowerCase()}\n${auth.order_password.trim()}`)
  let binary='';for(const b of bytes)binary+=String.fromCharCode(b)
  headers.Authorization='Guest '+btoa(binary).replace(/\+/g,'-').replace(/\//g,'_').replace(/=+$/g,'')
 }
 try{
  const response=await userApi.post(`${guest?'/guest':''}/orders/${encodeURIComponent(no)}/telegram-delivery`,{}, {headers,silentBusinessError:true})
  const candidate=String(response.data.data?.url||'')
  if(seq===deliverySeq&&/^https:\/\/t\.me\/[A-Za-z0-9_]+(?:\?start=[A-Za-z0-9_-]+)?$/.test(candidate))deliveryURL.value=candidate
 }catch{/* Existing support remains available until the independent bot is configured. */}
},{immediate:true})
const isClaude=computed(()=>hasClaudeItems(props.order))
const open=ref(false)
const seen=new Set<string>()
const isPaid=computed(()=>Boolean(props.order?.paid_at)&&['paid','fulfilling','delivered','completed'].includes(props.order?.status||''))
watch(()=>[isPaid.value,props.order?.order_no,isClaude.value] as const,([paid,no])=>{
  if(!paid||!no){open.value=false;return}
  const key=`xgai-paid-support:${isClaude.value ? 'claude:' : ''}${no}`
  if(seen.has(key))return
  seen.add(key)
  try{if(sessionStorage.getItem(key))return;sessionStorage.setItem(key,'1')}catch{/* Storage may be unavailable. */}
  open.value=true
},{immediate:true})
</script>
