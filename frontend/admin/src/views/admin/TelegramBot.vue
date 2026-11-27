<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Bot, RefreshCw } from 'lucide-vue-next'
const { t } = useI18n()
const loading = ref(false)
const failed = ref(false)
const state = ref<{enabled:boolean;connected:boolean;webhook_ok:boolean;admin_enabled:boolean;username:string;bot_url:string;pending_updates:number;has_webhook_error:boolean} | null>(null)
async function load() {
  loading.value = true; failed.value = false
  try { state.value = (await adminAPI.getDeliveryBotStatus()).data.data }
  catch { failed.value = true; state.value = null }
  finally { loading.value = false }
}
onMounted(load)
</script>
<template>
  <div class="space-y-6">
    <header class="flex flex-wrap items-start justify-between gap-4">
      <div><h1 class="flex items-center gap-2 text-2xl font-bold"><Bot class="text-primary" />{{ t('deliveryBot.title') }}</h1><p class="mt-2 text-muted-foreground">{{ t('deliveryBot.description') }}</p></div>
      <Button variant="outline" :disabled="loading" @click="load"><RefreshCw class="mr-2 h-4 w-4" :class="{'animate-spin':loading}" />{{t('deliveryBot.refresh')}}</Button>
    </header>
    <p v-if="failed" role="alert" class="text-destructive">{{t('deliveryBot.failed')}}</p>
    <Card><CardHeader><CardTitle>{{state?.username ? '@'+state.username : t('deliveryBot.title')}}</CardTitle><CardDescription>{{t('deliveryBot.security')}}</CardDescription></CardHeader>
      <CardContent class="space-y-5">
        <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <div v-for="item in ([['configured',state?.enabled],['connection',state?.connected],['webhook',state?.webhook_ok],['admin',state?.admin_enabled]] as const)" :key="item[0]" class="space-y-3 rounded-xl border border-border bg-muted/20 p-4"><p class="text-sm text-muted-foreground">{{t('deliveryBot.'+item[0])}}</p><Badge :variant="item[1] ? 'default':'secondary'">{{t(item[1]?'deliveryBot.yes':'deliveryBot.no')}}</Badge></div>
        </div>
        <p v-if="state?.has_webhook_error" class="text-sm text-amber-600">{{t('deliveryBot.error')}}</p>
        <p v-if="state" class="text-sm text-muted-foreground">{{t('deliveryBot.pending')}}：{{state.pending_updates ?? 0}}</p>
        <div class="flex flex-wrap gap-3"><Button v-if="state?.username" as-child><a :href="'https://t.me/'+state.username" target="_blank" rel="noopener noreferrer">{{t('deliveryBot.open')}}</a></Button><Button as-child variant="outline"><router-link to="/orders">{{t('deliveryBot.orders')}}</router-link></Button></div>
      </CardContent>
    </Card>
    <Card><CardHeader><CardTitle>{{t('deliveryBot.help')}}</CardTitle></CardHeader><CardContent><ol class="list-decimal space-y-3 pl-5 text-sm leading-7 text-muted-foreground"><li v-for="key in ['step1','step2','step3']" :key="key">{{t('deliveryBot.'+key)}}</li></ol></CardContent></Card>
  </div>
</template>
