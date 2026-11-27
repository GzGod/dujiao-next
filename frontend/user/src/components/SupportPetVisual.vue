<script setup lang="ts">
import { computed,ref,watch } from 'vue'
import { MessageCircle } from 'lucide-vue-next'
import { getImageUrl } from '../utils/image'
export type SupportPetConfig = {mode?:string;url?:string;name?:string;source_url?:string;frame_width?:number;frame_height?:number}
const props=defineProps<{pet?:SupportPetConfig}>()
const failed=ref(false)
const custom=computed(()=>props.pet?.mode==='spritesheet'||props.pet?.mode==='image')
const imageMode=computed(()=>props.pet?.mode==='image')
const url=computed(()=>getImageUrl(custom.value?props.pet?.url:'/images/support/jiyi.webp'))
const ratio=computed(()=>imageMode.value?'1':`${props.pet?.frame_width||192} / ${props.pet?.frame_height||208}`)
watch(url,()=>{failed.value=false})
</script>
<template>
 <span class="pet-visual" :style="{aspectRatio:ratio}" aria-hidden="true">
  <MessageCircle v-if="failed || !url" class="h-12 w-12 text-primary" />
  <img v-else-if="imageMode" :src="url" alt="" class="pet-image" @error="failed=true" />
  <span v-else class="pet-sheet" :style="{backgroundImage:`url(${JSON.stringify(url)})`}"><img :src="url" alt="" class="pet-probe" @error="failed=true" /></span>
 </span>
</template>
<style scoped>
.pet-visual{display:flex;align-items:center;justify-content:center;width:88px;max-height:160px;overflow:hidden;filter:drop-shadow(0 5px 7px #0002)}
.pet-sheet{display:block;width:100%;height:100%;background-repeat:no-repeat;background-size:800% 900%;image-rendering:pixelated;animation:pet-idle 1.1s steps(6) infinite}
.pet-visual:hover .pet-sheet{animation:pet-wave .7s steps(4) infinite}
.pet-image{width:100%;height:100%;object-fit:contain}
.pet-probe{position:absolute;width:1px;height:1px;opacity:0;pointer-events:none}
@keyframes pet-idle{from{background-position:0 0}to{background-position:85.7142857% 0}}
@keyframes pet-wave{from{background-position:0 37.5%}to{background-position:57.1428571% 37.5%}}
@media(max-width:640px){.pet-visual{width:68px;max-height:124px}}
@media(prefers-reduced-motion:reduce){.pet-sheet{animation:none!important}}
</style>
