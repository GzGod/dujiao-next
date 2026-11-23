<template>
  <section class="xdecision" :aria-label="labels.heading">
    <header><small>{{ labels.heading }}</small><h2>{{ guide.summary }}</h2></header>
    <div class="xdecision__grid">
      <article><h3>{{ labels.suitable }}</h3><p>{{ guide.suitable }}</p></article>
      <article><h3>{{ labels.before }}</h3><ul><li v-for="item in guide.before" :key="item">{{ item }}</li></ul></article>
    </div>
    <nav class="xdecision__actions" :aria-label="labels.delivery">
      <template v-if="guide.kind === 'gpt'">
        <a href="/guides/redeem">{{ labels.guide }} ↗</a>
        <a :href="appStore.redemptionUrl()" target="_blank" rel="noopener noreferrer">{{ labels.redeem }} ↗</a>
      </template>
      <a href="https://t.me/xuegaozhanshen" target="_blank" rel="noopener noreferrer">{{ labels.support }} ↗</a>
    </nav>
  </section>
</template>
<script setup lang="ts">
import { useAppStore } from '../../../stores/app'
const appStore = useAppStore()
import type { ProductGuide } from '../../../utils/productGuide'
defineProps<{ guide: ProductGuide; labels: Record<string, string> }>()
</script>
<style scoped>
.xdecision { color: var(--x-ink); }
.xdecision header { max-width: 850px; margin-bottom: 28px; }
.xdecision header small { color: var(--x-blue); font-weight: 600; font-size: 13px; }
.xdecision h2 { margin: 12px 0 0; font-size: clamp(19px, 2vw, 25px); line-height: 1.6; font-weight: 600; }
.xdecision__grid { display: grid; grid-template-columns: 1fr 1.6fr; gap: 20px; }
.xdecision article { padding: 24px; border: 1px solid var(--x-line-strong); border-radius: 18px; background: var(--x-surface); }
.xdecision h3 { margin: 0 0 14px; font-size: 16px; font-weight: 650; }
.xdecision p, .xdecision li { color: var(--x-ink-3); font-size: 14px; line-height: 1.85; }
.xdecision ul { list-style: disc; padding-left: 18px; margin: 0; }
.xdecision li + li { margin-top: 9px; }
.xdecision__actions { display: flex; flex-wrap: wrap; gap: 12px; margin: 24px 0; }
.xdecision__actions a { padding: 11px 16px; border: 1px solid var(--x-line-strong); border-radius: 10px; background: var(--x-blue-soft); color: var(--x-blue); font-size: 13px; font-weight: 600; }
.xdecision a:focus-visible { outline: 2px solid var(--x-blue); outline-offset: 3px; }
@media (max-width: 680px) { .xdecision__grid { grid-template-columns: 1fr; gap: 12px; } .xdecision article { padding: 20px; } .xdecision__actions a { flex: 1 1 100%; text-align: center; } }
</style>
