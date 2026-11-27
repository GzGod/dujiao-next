<template>
  <div class="space-y-6">
    <ResellerSectionHeader
      :title="t('resellerConsole.products.title')"
      :description="t('resellerConsole.products.description')"
    >
      <template #actions>
        <Button v-if="primaryDomainUrl" as-child variant="outline" size="sm">
          <a :href="primaryDomainUrl" target="_blank" rel="noopener noreferrer">
            <ExternalLink class="h-4 w-4" />
            {{ t('resellerConsole.products.previewStore') }}
          </a>
        </Button>
        <Button as-child variant="outline" size="sm">
          <RouterLink to="/reseller/orders">
            <ShoppingBag class="h-4 w-4" />
            {{ t('resellerConsole.products.viewOrders') }}
          </RouterLink>
        </Button>
      </template>
    </ResellerSectionHeader>

    <div class="grid gap-4 md:grid-cols-3">
      <Card class="p-5">
        <div class="flex items-center justify-between gap-3">
          <div>
            <p class="text-xs font-semibold uppercase tracking-[0.16em] text-muted-foreground">{{ t('resellerConsole.products.cards.defaultMarkup') }}</p>
            <div class="mt-2 font-mono text-2xl font-black text-foreground">{{ profile?.default_markup_percent || '0.00' }}%</div>
          </div>
          <span class="flex h-11 w-11 items-center justify-center rounded-xl bg-primary/10 text-primary">
            <Percent class="h-5 w-5" />
          </span>
        </div>
        <p class="mt-3 text-sm text-muted-foreground">{{ t('resellerConsole.products.cards.defaultMarkupDescription') }}</p>
      </Card>
      <Card class="p-5">
        <div class="flex items-center justify-between gap-3">
          <div>
            <p class="text-xs font-semibold uppercase tracking-[0.16em] text-muted-foreground">{{ t('resellerConsole.products.cards.maxMarkup') }}</p>
            <div class="mt-2 font-mono text-2xl font-black text-foreground">{{ profile?.max_markup_percent || '0.00' }}%</div>
          </div>
          <span class="flex h-11 w-11 items-center justify-center rounded-xl bg-info/10 text-info">
            <Gauge class="h-5 w-5" />
          </span>
        </div>
        <p class="mt-3 text-sm text-muted-foreground">{{ t('resellerConsole.products.cards.maxMarkupDescription') }}</p>
      </Card>
      <Card class="p-5">
        <div class="flex items-center justify-between gap-3">
          <div>
            <p class="text-xs font-semibold uppercase tracking-[0.16em] text-muted-foreground">{{ t('resellerConsole.products.cards.storefront') }}</p>
            <div class="mt-2 truncate font-mono text-sm font-bold text-foreground">{{ primaryDomain || t('resellerConsole.products.cards.noPrimaryDomain') }}</div>
          </div>
          <span class="flex h-11 w-11 items-center justify-center rounded-xl bg-success/10 text-success">
            <Globe class="h-5 w-5" />
          </span>
        </div>
        <p class="mt-3 text-sm text-muted-foreground">{{ t('resellerConsole.products.cards.storefrontDescription') }}</p>
      </Card>
    </div>

    <ResellerProductSettingsPanel :embedded="true" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { isStorefrontReady } from '../../utils/resellerWorkspace'
import { useI18n } from 'vue-i18n'
import { ExternalLink, Gauge, Globe, Percent, ShoppingBag } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import ResellerSectionHeader from '../../components/reseller-console/ResellerSectionHeader.vue'
import ResellerProductSettingsPanel from '../personal/ResellerProductSettingsPanel.vue'
import { useResellerProfile } from '../../composables/reseller/useResellerProfile'

const { t } = useI18n()
const { snapshot, load } = useResellerProfile()

const isActiveVerifiedDomain = isStorefrontReady

const profile = computed(() => snapshot.value?.profile || null)
const primaryDomain = computed(() => {
  const domains = snapshot.value?.domains || []
  const activeDomains = domains.filter(isActiveVerifiedDomain)
  return (activeDomains.find((item) => item.is_primary) || activeDomains[0])?.domain || ''
})
const primaryDomainUrl = computed(() => (primaryDomain.value ? `https://${primaryDomain.value}` : ''))


onMounted(load)
</script>
