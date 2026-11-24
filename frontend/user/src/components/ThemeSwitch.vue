<template>
  <div class="theme-switch" role="group" :aria-label="labels.group" :data-mode="mode" @keydown="onKey">
    <span class="theme-switch__thumb" aria-hidden="true" />
    <button v-for="item in options" :key="item.mode" type="button" :aria-label="labels[item.mode]" :title="labels[item.mode]" :aria-pressed="mode === item.mode" @click="setTheme(item.mode)">
      <svg viewBox="0 0 24 24" aria-hidden="true"><template v-for="(row,y) in item.pixels" :key="y"><rect v-for="(pixel,x) in row" v-show="pixel === '1'" :key="x" :x="2+Number(x)*2.8" :y="2+y*2.8" width="2" height="2" rx=".35" /></template></svg>
    </button>
  </div>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useTheme, type ThemeMode } from '@/utils/theme'
const { locale } = useI18n()
const { mode, setTheme } = useTheme()
const translations = {
 'zh-CN': { group:'切换显示主题', dark:'夜间', light:'日间', system:'跟随系统' },
 'zh-TW': { group:'切換顯示主題', dark:'夜間', light:'日間', system:'跟隨系統' },
 'en-US': { group:'Display theme', dark:'Dark', light:'Light', system:'System' },
}
const labels = computed(() => translations[locale.value as keyof typeof translations] ?? translations['en-US'])
const options: {mode:ThemeMode;pixels:string[]}[] = [
 {mode:'dark',pixels:['0011100','0111000','1110000','1110000','1110011','0111110','0011100']},
 {mode:'light',pixels:['0001000','0100010','0011100','1011101','0011100','0100010','0001000']},
 {mode:'system',pixels:['1111111','1000001','1000001','1000001','1111111','0001000','0011100']},
]
function onKey(event:KeyboardEvent) {
 if (!['ArrowLeft','ArrowRight','Home','End'].includes(event.key)) return
 event.preventDefault()
 const current=options.findIndex(item=>item.mode===mode.value)
 const index=event.key==='Home'?0:event.key==='End'?2:(current+(event.key==='ArrowRight'?1:2))%3
 setTheme(options[index]!.mode)
 const group=event.currentTarget as HTMLElement
 ;(group.querySelectorAll('button')[index] as HTMLButtonElement).focus()
}
</script>
<style scoped>
.theme-switch{position:relative;display:flex;flex:none;width:102px;height:38px;padding:2px;border:1px solid var(--ui-border-strong);border-radius:14px;background:var(--ui-bg-soft);box-shadow:inset 0 2px 4px #00000018;isolation:isolate}
.theme-switch button{position:relative;z-index:1;display:grid;place-items:center;width:32px;height:32px;padding:0;border:0;border-radius:11px;background:transparent;color:var(--ui-text-muted);cursor:pointer}
.theme-switch button[aria-pressed=true]{color:var(--ui-text-primary)}
.theme-switch button:focus-visible{outline:2px solid var(--ui-accent);outline-offset:2px}
.theme-switch svg{width:18px;height:18px;fill:currentColor}
.theme-switch__thumb{position:absolute;top:2px;left:2px;width:32px;height:32px;border:1px solid var(--ui-border-strong);border-bottom:2px solid var(--ui-accent);border-radius:11px;background:var(--ui-bg-elevated);box-shadow:0 2px 6px #00000025;transition:transform .4s cubic-bezier(.22,1.35,.36,1);pointer-events:none}
.theme-switch[data-mode=light] .theme-switch__thumb{transform:translateX(32px);border-bottom-color:#d79623}
.theme-switch[data-mode=system] .theme-switch__thumb{transform:translateX(64px)}
@media(prefers-reduced-motion:reduce){.theme-switch__thumb{transition:none}}
@media(forced-colors:active){.theme-switch button[aria-pressed=true]{outline:2px solid Highlight}}
</style>
