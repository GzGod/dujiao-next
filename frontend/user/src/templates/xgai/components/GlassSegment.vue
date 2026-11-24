<template>
 <div ref="root" class="glass-segment" role="group" :aria-label="label" @keydown="onKey">
  <GlassSurface :x="position" :width="width" :radius="16" :intensity="1.05" />
  <button v-for="item in options" :key="item.value" type="button" :aria-pressed="modelValue===item.value" :class="{'is-active':modelValue===item.value}" @click="$emit('update:modelValue',item.value)">{{ item.label }}</button>
 </div>
</template>
<script setup lang="ts">
import {ref,watch,nextTick,onMounted,onUnmounted} from 'vue'
import GlassSurface from './GlassSurface.vue'
const props=defineProps<{modelValue:string;options:{value:string;label:string}[];label:string}>()
defineEmits<{'update:modelValue':[value:any]}>()
const root=ref<HTMLElement>();const position=ref(0);const width=ref(100)
function measure(){const el=root.value?.querySelector<HTMLElement>('[aria-pressed="true"]');if(el){position.value=el.offsetLeft;width.value=el.offsetWidth}}
function onKey(event:KeyboardEvent){
 if(!['ArrowLeft','ArrowRight','Home','End'].includes(event.key))return
 event.preventDefault()
 const buttons=Array.from(root.value?.querySelectorAll('button')??[])
 const current=buttons.indexOf(event.target as HTMLButtonElement)
 const index=event.key==='Home'?0:event.key==='End'?buttons.length-1:(current+(event.key==='ArrowRight'?1:buttons.length-1))%buttons.length
 buttons[index]?.click();buttons[index]?.focus()
}
let observer:ResizeObserver|undefined
onMounted(()=>{measure();observer=new ResizeObserver(measure);if(root.value)observer.observe(root.value)})
onUnmounted(()=>observer?.disconnect())
watch(()=>[props.modelValue,props.options],async()=>{await nextTick();measure()},{deep:true})
</script>
<style scoped>
.glass-segment{position:relative;isolation:isolate;display:inline-flex;max-width:100%;padding:4px;border:1px solid var(--x-line-strong);border-radius:20px;background:color-mix(in srgb,var(--x-bg-alt) 65%,transparent);box-shadow:inset 0 2px 6px #00000015,0 1px 0 #ffffff15;backdrop-filter:blur(22px);overflow:visible;scrollbar-width:none}
.glass-segment button{position:relative;z-index:1;flex:none;padding:12px 20px;border:0;background:transparent;color:var(--x-ink-3);font-weight:600;cursor:pointer;transition:color .25s;text-wrap:nowrap}
.glass-segment button.is-active{color:var(--x-ink)}
.glass-segment button:focus-visible{outline:2px solid var(--x-blue);outline-offset:-3px;border-radius:16px}
@media(max-width:600px){.glass-segment button{padding:10px 15px}}

</style>
