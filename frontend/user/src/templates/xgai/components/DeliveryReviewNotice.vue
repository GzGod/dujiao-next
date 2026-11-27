<template>
  <section class="delivery-review" role="status">
    <p>{{ t('payment.deliveryUnderReview') }}</p>
    <a class="x-btn x-btn--primary" :href="supportUrl" target="_blank" rel="noopener noreferrer">
      {{ t('payment.reviewContact') }}
    </a>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ orderNo: string }>()
const { t } = useI18n()
const supportUrl = computed(() => 'https://t.me/xuegaozhanshen?text=' +
  encodeURIComponent(t('payment.reviewContactDraft', { order: props.orderNo })))
</script>

<style scoped>
.delivery-review { display: grid; justify-items: start; gap: 12px; padding-block: 12px; color: var(--x-ink); }
.delivery-review p { margin: 0; }
.inline-pay__result .delivery-review { justify-items: center; }
</style>
