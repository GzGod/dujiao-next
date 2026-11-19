<script setup lang="ts">
import type { HTMLAttributes } from "vue"
import { cn } from "@/lib/utils"

const props = defineProps<{
  class?: HTMLAttributes["class"]
}>()
</script>

<template>
  <div
    :class="
      cn(
        'rounded-2xl border bg-card text-card-foreground shadow-[0_2px_12px_rgb(31_35_40_/_0.03)]',
        props.class,
      )
    "
  >
    <slot />
  </div>
</template>
