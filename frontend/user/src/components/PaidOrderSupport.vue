<template>
  <div v-if="isPaid" class="my-4 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-primary/20 bg-primary/5 p-4">
    <span class="text-sm text-foreground">{{ t('paidSupport.prompt') }}</span>
    <a :href="url" target="_blank" rel="noopener noreferrer" class="font-semibold text-primary underline">{{ t('paidSupport.contact') }}</a>
  </div>
  <DialogRoot v-model:open="open"><DialogPortal>
    <DialogOverlay class="fixed inset-0 z-[120] bg-black/60 backdrop-blur-sm" />
    <DialogContent class="fixed left-1/2 top-1/2 z-[121] w-[calc(100%-2rem)] max-w-sm -translate-x-1/2 -translate-y-1/2 rounded-2xl border border-border bg-background p-6 text-foreground shadow-xl">
      <DialogTitle class="text-xl font-bold">{{ t('paidSupport.title') }}</DialogTitle>
      <DialogDescription class="mt-3 text-sm leading-relaxed text-muted-foreground">{{ t('paidSupport.prompt') }}</DialogDescription>
      <a :href="url" target="_blank" rel="noopener noreferrer" class="mt-6 flex w-full items-center justify-center rounded-xl bg-primary px-4 py-3 font-semibold text-primary-foreground" @click="open=false">{{ t('paidSupport.contact') }} · Telegram</a>
      <DialogClose class="mt-3 w-full rounded-xl border border-border px-4 py-2 text-sm">{{ t('paidSupport.dismiss') }}</DialogClose>
    </DialogContent>
  </DialogPortal></DialogRoot>
</template>
<script setup lang="ts">
import { computed,ref,watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { DialogRoot,DialogPortal,DialogOverlay,DialogContent,DialogTitle,DialogDescription,DialogClose } from 'reka-ui'
const props=defineProps<{order:{order_no?:string;status?:string;paid_at?:string|null}|null|undefined}>()
const {t}=useI18n()
const url='https://t.me/xuegaozhanshen'
const open=ref(false)
const seen=new Set<string>()
const isPaid=computed(()=>Boolean(props.order?.paid_at)&&['paid','fulfilling','delivered','completed'].includes(props.order?.status||''))
watch(()=>[isPaid.value,props.order?.order_no] as const,([paid,no])=>{
  if(!paid||!no){open.value=false;return}
  const key=`xgai-paid-support:${no}`
  if(seen.has(key))return
  seen.add(key)
  try{if(sessionStorage.getItem(key))return;sessionStorage.setItem(key,'1')}catch{/* Storage may be unavailable. */}
  open.value=true
},{immediate:true})
</script>
