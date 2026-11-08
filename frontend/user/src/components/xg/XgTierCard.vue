<template>
  <div class="xg-tier" :class="{ 'xg-tier--featured': featured }">
    <div class="xg-tier__top">
      <span class="xg-tier__badge" :class="`xg-tier__badge--${level}`">{{ name }}</span>
      <span v-if="featured" class="xg-tier__flag">{{ flag }}</span>
    </div>
    <div class="xg-tier__rate">
      <span class="xg-tier__rate-value">{{ rate }}</span>
      <span class="xg-tier__rate-unit">{{ rateUnit }}</span>
    </div>
    <p class="xg-tier__rule">{{ rule }}</p>
    <ul class="xg-tier__perks">
      <li v-for="perk in perks" :key="perk">{{ perk }}</li>
    </ul>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  level: 'free' | 'basic' | 'pro' | 'elite'
  name: string
  rate: string
  rateUnit: string
  rule: string
  perks: string[]
  featured?: boolean
  flag?: string
}>()
</script>
