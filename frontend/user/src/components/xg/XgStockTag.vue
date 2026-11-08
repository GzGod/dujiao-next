<template>
  <span class="xg-stock" :class="`xg-stock--${tone}`">
    <span class="xg-stock__dot" aria-hidden="true"></span>
    {{ label }}
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ status?: string }>()
const { t } = useI18n()

// 有货（含不限量）/ 库存紧张 / 暂时缺货
const tone = computed(() => {
  if (props.status === 'out_of_stock') return 'out'
  if (props.status === 'low_stock') return 'low'
  return 'in'
})
const label = computed(() => t(`xg.stock.${tone.value}`))
</script>

<style scoped>
.xg-stock {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  padding: 2px var(--space-2);
  border-radius: 999px;
  font-size: 12px;
  font-weight: 500;
  line-height: 20px;
  white-space: nowrap;
}
.xg-stock__dot {
  width: 6px;
  height: 6px;
  border-radius: 999px;
  background: currentColor;
}
.xg-stock--in { color: var(--ui-success); background: var(--ui-success-soft); }
.xg-stock--low { color: var(--ui-warning); background: var(--ui-warning-soft); }
.xg-stock--out { color: var(--neutral-500); background: var(--neutral-100); }
</style>
