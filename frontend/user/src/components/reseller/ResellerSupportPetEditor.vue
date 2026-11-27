<script setup lang="ts">
import {computed,ref} from 'vue'
import {useI18n} from 'vue-i18n'
import {supportPetFrameSize} from '../../utils/supportPet'
import {resellerAPI} from '../../api/reseller'
import {Button} from '@/components/ui/button'
import {Input} from '@/components/ui/input'
import SupportPetVisual from '../SupportPetVisual.vue'
import type {SupportPetConfig} from '../SupportPetVisual.vue'
const model=defineModel<SupportPetConfig>({required:true})
const {t,locale}=useI18n()
const busy=ref(false),error=ref(''),fileInput=ref<HTMLInputElement|null>(null)
const tutorial=computed(()=>`/guides/reseller-support-pet${locale.value==='zh-TW'?'.zh-TW':locale.value==='en-US'?'.en':''}.md`)
async function upload(event:Event){
 const input=event.target as HTMLInputElement,file=input.files?.[0];input.value='';if(!file)return
 busy.value=true;error.value=''
 let objectUrl=''
 try{
  if(!/^image\/(png|webp|gif|jpeg)$/.test(file.type)||file.size>10*1024*1024)throw Error(t('supportPet.invalidFile'))
  objectUrl=URL.createObjectURL(file)
  const dimensions=await new Promise<{width:number;height:number}>((resolve,reject)=>{const image=new Image();image.onload=()=>resolve({width:image.naturalWidth,height:image.naturalHeight});image.onerror=()=>reject(Error(t('supportPet.invalidFile')));image.src=objectUrl})
  const sheet=model.value.mode==='spritesheet'
  if(sheet&&!supportPetFrameSize(dimensions.width,dimensions.height))throw Error(t('supportPet.invalidSheet'))
  const response=await resellerAPI.uploadImage(file)
  const url=response.data.data?.url
  if(!url)throw Error(t('supportPet.failed'))
  model.value={...model.value,mode:sheet?'spritesheet':'image',url,frame_width:sheet?dimensions.width/8:0,frame_height:sheet?dimensions.height/9:0}
 }catch(e:any){error.value=e?.message||t('supportPet.failed')}
 finally{if(objectUrl)URL.revokeObjectURL(objectUrl);busy.value=false}
}
</script>
<template>
 <section class="mt-5 rounded-2xl border bg-card p-4 sm:p-5 space-y-4">
  <div><h3 class="font-semibold">{{t('supportPet.title')}}</h3><p class="mt-1 text-sm text-muted-foreground">{{t('supportPet.hint')}}</p></div>
  <div class="flex flex-wrap gap-3 text-sm"><a href="https://petdex.dev/" target="_blank" rel="noopener noreferrer" class="text-primary underline">{{t('supportPet.browse')}} ↗</a><a :href="tutorial" target="_blank" rel="noopener noreferrer" class="text-primary underline">{{t('supportPet.guide')}} ↗</a><a :href="tutorial" download class="text-primary underline">{{t('supportPet.downloadGuide')}}</a></div>
  <label class="block space-y-2"><span class="text-sm font-medium">{{t('supportPet.display')}}</span><select v-model="model.mode" :disabled="busy" class="h-10 w-full rounded-lg border bg-background px-3"><option value="default">{{t('supportPet.default')}}</option><option value="spritesheet">{{t('supportPet.sheet')}}</option><option value="image">{{t('supportPet.image')}}</option><option value="hidden">{{t('supportPet.hidden')}}</option></select></label>
  <template v-if="model.mode==='spritesheet'||model.mode==='image'">
   <p class="text-xs text-muted-foreground">{{t(model.mode==='spritesheet'?'supportPet.sheetHint':'supportPet.imageHint')}}</p>
   <div class="grid gap-4 sm:grid-cols-2"><label class="space-y-2"><span class="text-sm">{{t('supportPet.name')}}</span><Input v-model="model.name" maxlength="60" :disabled="busy" /></label><label class="space-y-2"><span class="text-sm">{{t('supportPet.source')}}</span><Input v-model.trim="model.source_url" placeholder="https://petdex.dev/pets/jiyi" :disabled="busy" /></label></div>
   <input ref="fileInput" type="file" accept="image/png,image/webp,image/gif,image/jpeg" class="hidden" @change="upload" />
   <Button type="button" variant="outline" :disabled="busy" @click="fileInput?.click()">{{t(busy?'supportPet.uploading':'supportPet.upload')}}</Button>
   <p v-if="!model.url" class="text-xs text-muted-foreground">{{t('supportPet.needImage')}}</p>
  </template>
  <p v-if="error" role="alert" class="text-sm text-destructive">{{error}}</p>
  <div v-if="model.mode!=='hidden'" class="flex min-h-36 flex-col items-center justify-center gap-2 rounded-xl border bg-muted/20 p-4"><SupportPetVisual :pet="model" /><span class="rounded-full border bg-card px-3 py-1 text-xs">{{t('floatingSupport.title')}} ↗</span></div>
  <p class="text-xs text-muted-foreground">{{t('supportPet.saveHint')}}</p>
 </section>
</template>
