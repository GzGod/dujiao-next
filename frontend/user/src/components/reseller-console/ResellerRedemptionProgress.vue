<script setup lang="ts">
import { useI18n } from 'vue-i18n'
defineProps<{entries?:{order_no:string;status:string;recorded_at:string}[]}>()
const {t}=useI18n()
const keyFor=(status:string)=>({unused:'unused',inactive:'unused',submitted:'submittedCode',processing:'redeeming',completed:'completed',success:'completed',succeeded:'completed',destroyed:'destroyed',failed:'error',needs_action:'error'}[status] || 'unknown')
</script>
<template>
 <div v-if="entries?.length" class="space-y-1"><div v-for="entry in entries" :key="entry.order_no" class="text-xs"><span class="rounded-md border bg-muted/40 px-2 py-1" :title="entry.order_no">{{t('resellerUX.'+keyFor(entry.status))}}</span><time class="mt-1 block text-[10px] text-muted-foreground">{{new Date(entry.recorded_at).toLocaleString()}}</time></div></div>
 <span v-else class="text-xs text-muted-foreground">{{t('resellerUX.unknown')}}</span>
</template>
