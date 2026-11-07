<template>
  <div v-if="proof?.transaction" class="min-w-0" :class="boxClass">
    <div class="text-xs text-muted-foreground">{{ t('orderDetail.paymentProof.title') }}</div>
    <div class="mt-1 flex min-w-0 items-center gap-2">
      <a
        v-if="explorerUrl"
        :href="explorerUrl"
        target="_blank"
        rel="noopener noreferrer"
        class="min-w-0 truncate font-mono text-primary underline-offset-4 hover:underline"
        :title="proof.transaction"
      >{{ shortHash }}</a>
      <span v-else class="min-w-0 truncate font-mono" :title="proof.transaction">{{ shortHash }}</span>
      <button
        type="button"
        class="shrink-0 text-muted-foreground transition-colors hover:text-foreground"
        :aria-label="t('orderDetail.paymentProof.copy')"
        @click="handleCopy"
      >
        <Check v-if="copied" class="h-3.5 w-3.5 text-success" aria-hidden="true" />
        <Copy v-else class="h-3.5 w-3.5" aria-hidden="true" />
      </button>
    </div>
    <div class="mt-1 text-xs text-muted-foreground">{{ networkName }}</div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, Copy } from 'lucide-vue-next'
import { copyText } from '../../utils/clipboard'
import { explorerTxUrl, networkLabel } from '../../utils/x402Wallet'

const props = defineProps<{
  proof?: { network?: string; transaction?: string; payer?: string } | null
  boxClass?: string
}>()

const { t } = useI18n()
const copied = ref(false)

const shortHash = computed(() => {
  const tx = String(props.proof?.transaction || '')
  return tx.length > 20 ? `${tx.slice(0, 10)}…${tx.slice(-8)}` : tx
})
const explorerUrl = computed(() => explorerTxUrl(String(props.proof?.network || ''), String(props.proof?.transaction || '')))
const networkName = computed(() => networkLabel(String(props.proof?.network || '')))

const handleCopy = async () => {
  if (!props.proof?.transaction) return
  try {
    await copyText(props.proof.transaction)
    copied.value = true
    window.setTimeout(() => {
      copied.value = false
    }, 1500)
  } catch {
    // 复制失败时保持原状，用户仍可点击链接查看
  }
}
</script>
