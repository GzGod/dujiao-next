<template><canvas ref="canvas" class="glass-optics" aria-hidden="true" /></template>
<script setup lang="ts">
import {ref,onMounted,onUnmounted,watch} from 'vue'
import {useTheme} from '@/utils/theme'
import {createGlassThemeRenderer} from '@/components/theme/glass-theme-renderer'
const props=defineProps<{x?:number;width?:number;radius?:number;intensity?:number}>()
const canvas=ref<HTMLCanvasElement>();const {theme}=useTheme()
let engine:ReturnType<typeof createGlassThemeRenderer>=null
let observer:ResizeObserver|undefined
onMounted(()=>{if(!canvas.value)return;const el=canvas.value
 try { engine=createGlassThemeRenderer(el,'light',theme.value==='dark',()=>({x:props.x??0,width:props.width??Math.max(1,el.clientWidth-24),height:Math.max(1,el.clientHeight-24),radius:props.radius??18,intensity:props.intensity??.65})) } catch { engine=null }
 observer=new ResizeObserver(()=>engine?.update('light',theme.value==='dark'));observer.observe(el)
})
watch(()=>[props.x,props.width,theme.value],()=>engine?.update('light',theme.value==='dark'))
onUnmounted(()=>{observer?.disconnect();engine?.destroy()})
</script>
<style scoped>.glass-optics{position:absolute;inset:-12px;width:calc(100% + 24px);height:calc(100% + 24px);pointer-events:none;z-index:0}</style>
