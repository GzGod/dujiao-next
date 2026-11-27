<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Gauge, Network, ShieldCheck, UserRound } from 'lucide-vue-next'
import { adminAPI } from '@/api/admin'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { notifyError, notifySuccess } from '@/utils/notify'

const { t } = useI18n()
const loading = ref(false)
const submitting = ref(false)

const defaultRateLimit = (guest: boolean) => ({
  enabled: guest,
  window_seconds: 60,
  max_requests: guest ? 3 : 10,
  block_seconds: 120,
})

const defaultSettlementLimits: Record<string, number> = {
  first_amount: 200,
  slow_seconds: 150,
  short_minutes: 30,
  long_hours: 24,
  repeat_amount: 150,
  short_total: 300,
  long_total: 400,
  paid_count: 3,
  short_unpaid: 3,
  long_unpaid: 4,
  reorder_minutes: 120,
  high_amount: 150,
  loyal_days: 7,
  loyal_orders: 2,
}
const defaultSettlementWeights: Record<string, number> = {
  first_seen_high_amount: 25,
  slow_qr_payment: 35,
  same_device_contact_switch: 40,
  unpaid_probe_moderate: 35,
  unredeemed_high_reorder: 35,
  subnet_burst_high_amount: 30,
  same_ip_different_contact: 15,
  disposable_or_random_email: 25,
}

const riskListKinds = ['ip', 'cidr', 'user_id', 'device_token', 'contact_norm'] as const
type RiskListEntry = { kind: typeof riskListKinds[number]; value: string; allow: boolean; expires_at?: string }
const canAllow = (kind: RiskListEntry['kind']) => kind !== 'contact_norm'
const form = reactive({
  settlement_mode: 'shadow',
  alipay_risk_subject: '',
  risky_email_domains_text: '',
  settlement_threshold: 60,
  settlement_weights: { ...defaultSettlementWeights },
  settlement_limits: { ...defaultSettlementLimits },
  enabled: false,
  common: {
    ip_blacklist_text: '',
    settlement_list: [] as RiskListEntry[],
  },
  guest: {
    enabled: true,
    max_pending_orders_per_ip: 2,
    max_quantity_per_product_per_order: 1,
    max_pending_quantity_per_ip_product: 2,
    payment_expire_minutes: 5,
    rate_limit: defaultRateLimit(true),
  },
  member: {
    enabled: true,
    max_pending_orders_per_user: 5,
    max_pending_orders_per_ip: 0,
    max_quantity_per_product_per_order: 0,
    rate_limit: defaultRateLimit(false),
  },
})

const numberValue = (value: unknown, fallback: number) => {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : fallback
}

const applyRateLimit = (target: typeof form.guest.rate_limit, raw: unknown, guest: boolean) => {
  const data = raw as Record<string, unknown> | undefined
  const defaults = defaultRateLimit(guest)
  target.enabled = data?.enabled != null ? !!data.enabled : defaults.enabled
  target.window_seconds = numberValue(data?.window_seconds, defaults.window_seconds)
  target.max_requests = numberValue(data?.max_requests, defaults.max_requests)
  target.block_seconds = numberValue(data?.block_seconds, defaults.block_seconds)
}

const applyLegacyRateLimit = (target: typeof form.guest.rate_limit, raw: unknown) => {
  const data = raw as Record<string, unknown> | undefined
  target.enabled = data?.enabled != null ? !!data.enabled : false
  target.window_seconds = numberValue(data?.window_seconds, 60)
  target.max_requests = numberValue(data?.max_requests, 5)
  target.block_seconds = numberValue(data?.block_seconds, 120)
}

const applyRecommendedGuestPolicy = () => {
  form.guest.enabled = true
  form.guest.max_pending_orders_per_ip = 2
  form.guest.max_quantity_per_product_per_order = 1
  form.guest.max_pending_quantity_per_ip_product = 2
  form.guest.payment_expire_minutes = 5
  Object.assign(form.guest.rate_limit, defaultRateLimit(true))
}

const loadNestedConfig = (data: Record<string, unknown>) => {
  form.alipay_risk_subject = typeof data.alipay_risk_subject === 'string' ? data.alipay_risk_subject : ''
  form.risky_email_domains_text = Array.isArray(data.risky_email_domains) ? data.risky_email_domains.join('\n') : ''
  form.settlement_mode = data.settlement_mode === 'hold_notice' || data.settlement_mode === 'silent_trap' ? data.settlement_mode : 'shadow'
  form.settlement_threshold = numberValue(data.settlement_threshold, 60)
  const limits = data.settlement_limits as Record<string, unknown> | undefined
  for (const [key, fallback] of Object.entries(defaultSettlementLimits)) form.settlement_limits[key] = numberValue(limits?.[key], fallback)
  const weights = data.settlement_weights as Record<string, unknown> | undefined
  for (const [key, fallback] of Object.entries(defaultSettlementWeights)) {
    form.settlement_weights[key] = numberValue(weights?.[key], fallback)
  }
  const common = data.common as Record<string, unknown> | undefined
  const guest = data.guest as Record<string, unknown> | undefined
  const member = data.member as Record<string, unknown> | undefined
  form.enabled = !!data.enabled
  form.common.settlement_list = Array.isArray(common?.settlement_list) ? common.settlement_list.map((entry: RiskListEntry) => ({ ...entry, allow: Boolean(entry.allow) })) : []
  form.common.ip_blacklist_text = ((common?.ip_blacklist as string[] | undefined) || []).join('\n')

  form.guest.enabled = guest?.enabled != null ? !!guest.enabled : true
  form.guest.max_pending_orders_per_ip = numberValue(guest?.max_pending_orders_per_ip, 2)
  form.guest.max_quantity_per_product_per_order = numberValue(guest?.max_quantity_per_product_per_order, 1)
  form.guest.max_pending_quantity_per_ip_product = numberValue(guest?.max_pending_quantity_per_ip_product, 2)
  form.guest.payment_expire_minutes = numberValue(guest?.payment_expire_minutes, 5)
  applyRateLimit(form.guest.rate_limit, guest?.rate_limit, true)

  form.member.enabled = member?.enabled != null ? !!member.enabled : true
  form.member.max_pending_orders_per_user = numberValue(member?.max_pending_orders_per_user, 5)
  form.member.max_pending_orders_per_ip = numberValue(member?.max_pending_orders_per_ip, 0)
  form.member.max_quantity_per_product_per_order = numberValue(member?.max_quantity_per_product_per_order, 0)
  applyRateLimit(form.member.rate_limit, member?.rate_limit, false)
}

const loadFlatConfig = (data: Record<string, unknown>) => {
  form.enabled = !!data.enabled
  form.common.ip_blacklist_text = ((data.ip_blacklist as string[] | undefined) || []).join('\n')
  form.guest.enabled = true
  form.guest.max_pending_orders_per_ip = numberValue(data.max_pending_orders_per_ip, 5)
  form.guest.max_quantity_per_product_per_order = 0
  form.guest.max_pending_quantity_per_ip_product = 0
  form.guest.payment_expire_minutes = 0
  applyLegacyRateLimit(form.guest.rate_limit, data.order_rate_limit)
  form.member.enabled = true
  form.member.max_pending_orders_per_user = numberValue(data.max_pending_orders_per_user, 3)
  form.member.max_pending_orders_per_ip = numberValue(data.max_pending_orders_per_ip, 5)
  form.member.max_quantity_per_product_per_order = 0
  applyLegacyRateLimit(form.member.rate_limit, data.order_rate_limit)
}

const loadConfig = async () => {
  loading.value = true
  try {
    const res = await adminAPI.getSettings({ key: 'order_risk_control_config' })
    const data = res.data?.data as Record<string, unknown> | undefined
    if (data && Object.keys(data).length > 0) {
      if (data.guest && data.member) loadNestedConfig(data)
      else loadFlatConfig(data)
    }
  } catch {
    // 读取失败时保留安全的推荐表单值，保存仍由用户主动触发。
  } finally {
    loading.value = false
  }
}

const normalizeLines = (value: string) => Array.from(new Set(value
  .split('\n')
  .map((line: string) => line.trim())
  .filter((line: string) => line.length > 0)))

const save = async () => {
  const entries: RiskListEntry[] = []
  for (const entry of form.common.settlement_list) {
    const value = entry.value.trim()
    const expiry = entry.expires_at?.trim()
    if (!value || (expiry && !Number.isFinite(Date.parse(expiry)))) {
      notifyError(t('admin.settings.orderRiskControl.listInvalid'))
      return
    }
    entries.push({
      kind: entry.kind, value, allow: canAllow(entry.kind) && entry.allow,
      expires_at: expiry ? new Date(expiry).toISOString() : undefined,
    })
  }
  submitting.value = true
  try {
    await adminAPI.updateSettings({
      key: 'order_risk_control_config',
      value: {
        version: 2,
        settlement_mode: form.settlement_mode,
        alipay_risk_subject: form.alipay_risk_subject.trim(),
        risky_email_domains: normalizeLines(form.risky_email_domains_text),
        settlement_threshold: Number(form.settlement_threshold),
        settlement_weights: { ...form.settlement_weights },
        settlement_limits: { ...form.settlement_limits },
        enabled: form.enabled,
        common: {
          ip_blacklist: normalizeLines(form.common.ip_blacklist_text),
          settlement_list: entries,
        },
        guest: {
          enabled: form.guest.enabled,
          max_pending_orders_per_ip: Number(form.guest.max_pending_orders_per_ip),
          max_quantity_per_product_per_order: Number(form.guest.max_quantity_per_product_per_order),
          max_pending_quantity_per_ip_product: Number(form.guest.max_pending_quantity_per_ip_product),
          payment_expire_minutes: Number(form.guest.payment_expire_minutes),
          rate_limit: {
            enabled: form.guest.rate_limit.enabled,
            window_seconds: Number(form.guest.rate_limit.window_seconds),
            max_requests: Number(form.guest.rate_limit.max_requests),
            block_seconds: Number(form.guest.rate_limit.block_seconds),
          },
        },
        member: {
          enabled: form.member.enabled,
          max_pending_orders_per_user: Number(form.member.max_pending_orders_per_user),
          max_pending_orders_per_ip: Number(form.member.max_pending_orders_per_ip),
          max_quantity_per_product_per_order: Number(form.member.max_quantity_per_product_per_order),
          rate_limit: {
            enabled: form.member.rate_limit.enabled,
            window_seconds: Number(form.member.rate_limit.window_seconds),
            max_requests: Number(form.member.rate_limit.max_requests),
            block_seconds: Number(form.member.rate_limit.block_seconds),
          },
        },
      },
    })
    notifySuccess(t('admin.settings.alerts.saveSuccess'))
  } catch (err) {
    const known = err as Error & { __notified?: boolean }
    if (!known?.__notified) notifyError(known?.message || t('admin.settings.alerts.saveFailed'))
  } finally {
    submitting.value = false
  }
}

const formatLimit = (value: number) => value > 0 ? String(value) : t('admin.settings.orderRiskControl.noLimit')
const guestSummary = computed(() => t('admin.settings.orderRiskControl.guest.summary', {
  orders: formatLimit(Number(form.guest.max_pending_orders_per_ip)),
  quantity: formatLimit(Number(form.guest.max_quantity_per_product_per_order)),
  pending: formatLimit(Number(form.guest.max_pending_quantity_per_ip_product)),
}))

defineExpose({ save })
onMounted(loadConfig)
</script>

<template>
  <div class="space-y-6" :class="{ 'pointer-events-none opacity-60': loading }">
    <section class="overflow-hidden rounded-xl border bg-card">
      <div class="flex flex-col gap-5 border-b bg-muted/30 p-5 sm:flex-row sm:items-center sm:justify-between">
        <div class="flex items-start gap-3">
          <div class="rounded-lg border bg-background p-2 text-foreground shadow-sm">
            <ShieldCheck class="h-5 w-5" />
          </div>
          <div>
            <h2 class="text-base font-semibold">{{ t('admin.settings.orderRiskControl.master.title') }}</h2>
            <p class="mt-1 max-w-2xl text-xs leading-5 text-muted-foreground">{{ t('admin.settings.orderRiskControl.master.subtitle') }}</p>
          </div>
        </div>
        <div class="flex items-center gap-3 rounded-full border bg-background px-4 py-2">
          <span class="text-xs font-medium" :class="form.enabled ? 'text-emerald-600' : 'text-muted-foreground'">
            {{ form.enabled ? t('admin.settings.orderRiskControl.statusEnabled') : t('admin.settings.orderRiskControl.statusDisabled') }}
          </span>
          <Switch v-model="form.enabled" />
        </div>
      </div>
    </section>

    <section class="space-y-4 rounded-xl border bg-card p-5">
      <h2 class="font-semibold">{{ t('admin.settings.orderRiskControl.settlementTitle') }}</h2>
      <div class="grid gap-4 sm:grid-cols-2">
        <label class="grid gap-2 text-sm">
          {{ t('admin.settings.orderRiskControl.settlementMode') }}
          <select v-model="form.settlement_mode" class="h-9 rounded-md border bg-background px-3">
            <option value="shadow">{{ t('admin.settings.orderRiskControl.modeShadow') }}</option>
            <option value="hold_notice">{{ t('admin.settings.orderRiskControl.modeNotice') }}</option>
            <option value="silent_trap">{{ t('admin.settings.orderRiskControl.modeSilent') }}</option>
          </select>
        </label>
        <label class="grid gap-2 text-sm">
          {{ t('admin.settings.orderRiskControl.settlementThreshold') }}
          <Input v-model.number="form.settlement_threshold" type="number" min="1" max="800" />
        </label>
      </div>
      <p class="text-sm text-muted-foreground">{{ t('admin.settings.orderRiskControl.settlementHint') }}</p>
      <label class="grid gap-2 text-sm">
        {{ t('admin.settings.orderRiskControl.riskyDomains') }}
        <Textarea v-model="form.risky_email_domains_text" rows="3" />
      </label>
      <label class="grid gap-2 text-sm">
        {{ t('admin.settings.orderRiskControl.alipayRiskSubject') }}
        <Input v-model="form.alipay_risk_subject" maxlength="80" />
        <span class="text-xs text-muted-foreground">
          {{ t('admin.settings.orderRiskControl.alipayRiskSubjectHint', { order: '{order_no}', amount: '{amount}' }) }}
        </span>
      </label>
      <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <label v-for="(_, key) in defaultSettlementLimits" :key="key" class="grid gap-2 text-sm">
          {{ t('admin.settings.orderRiskControl.limits.' + key) }}
          <Input v-model.number="form.settlement_limits[key]" type="number" min="1" />
        </label>
        <label v-for="(_, key) in defaultSettlementWeights" :key="key" class="grid gap-2 text-sm">
          {{ t('admin.settings.orderRiskControl.rules.' + key) }}
          <Input v-model.number="form.settlement_weights[key]" type="number" min="0" max="100" />
        </label>
      </div>
    </section>


    <section v-show="form.enabled" class="overflow-hidden rounded-xl border border-amber-200/70 bg-card shadow-sm dark:border-amber-900/60">
      <div class="border-b border-amber-200/70 bg-amber-50/70 p-5 dark:border-amber-900/60 dark:bg-amber-950/20">
        <div class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div class="flex items-start gap-3">
            <div class="rounded-lg bg-amber-500 p-2 text-white shadow-sm"><Network class="h-5 w-5" /></div>
            <div>
              <div class="flex items-center gap-2">
                <h3 class="text-base font-semibold">{{ t('admin.settings.orderRiskControl.guest.title') }}</h3>
                <span class="rounded-full bg-amber-100 px-2 py-0.5 text-[11px] font-semibold text-amber-800 dark:bg-amber-900/50 dark:text-amber-200">
                  {{ t('admin.settings.orderRiskControl.guest.priority') }}
                </span>
              </div>
              <p class="mt-1 max-w-2xl text-xs leading-5 text-muted-foreground">{{ t('admin.settings.orderRiskControl.guest.subtitle') }}</p>
            </div>
          </div>
          <div class="flex items-center gap-3">
            <Button type="button" variant="outline" size="sm" @click="applyRecommendedGuestPolicy">
              {{ t('admin.settings.orderRiskControl.guest.applyRecommended') }}
            </Button>
            <Switch v-model="form.guest.enabled" />
          </div>
        </div>
      </div>

      <div v-show="form.guest.enabled" class="space-y-6 p-5">
        <div class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
          <div class="space-y-2">
            <Label>{{ t('admin.settings.orderRiskControl.guest.maxPendingPerIP') }}</Label>
            <Input v-model.number="form.guest.max_pending_orders_per_ip" type="number" min="0" max="100" />
            <p class="text-xs leading-5 text-muted-foreground">{{ t('admin.settings.orderRiskControl.guest.maxPendingPerIPHint') }}</p>
          </div>
          <div class="space-y-2">
            <Label>{{ t('admin.settings.orderRiskControl.guest.maxQuantityPerProduct') }}</Label>
            <Input v-model.number="form.guest.max_quantity_per_product_per_order" type="number" min="0" max="100000" />
            <p class="text-xs leading-5 text-muted-foreground">{{ t('admin.settings.orderRiskControl.guest.maxQuantityPerProductHint') }}</p>
          </div>
          <div class="space-y-2">
            <Label>{{ t('admin.settings.orderRiskControl.guest.maxPendingQuantityPerIPProduct') }}</Label>
            <Input v-model.number="form.guest.max_pending_quantity_per_ip_product" type="number" min="0" max="100000" />
            <p class="text-xs leading-5 text-muted-foreground">{{ t('admin.settings.orderRiskControl.guest.maxPendingQuantityPerIPProductHint') }}</p>
          </div>
          <div class="space-y-2">
            <Label>{{ t('admin.settings.orderRiskControl.guest.paymentExpireMinutes') }}</Label>
            <Input v-model.number="form.guest.payment_expire_minutes" type="number" min="0" max="10080" />
            <p class="text-xs leading-5 text-muted-foreground">{{ t('admin.settings.orderRiskControl.guest.paymentExpireMinutesHint') }}</p>
          </div>
        </div>

        <div class="rounded-lg border border-dashed bg-muted/25 px-4 py-3 text-xs leading-5 text-muted-foreground">
          {{ guestSummary }}
        </div>

        <div class="rounded-lg border p-4">
          <div class="mb-4 flex items-center justify-between gap-4">
            <div class="flex items-center gap-2">
              <Gauge class="h-4 w-4 text-amber-600" />
              <div>
                <Label>{{ t('admin.settings.orderRiskControl.guest.rateLimitTitle') }}</Label>
                <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.settings.orderRiskControl.guest.rateLimitHint') }}</p>
              </div>
            </div>
            <Switch v-model="form.guest.rate_limit.enabled" />
          </div>
          <div v-show="form.guest.rate_limit.enabled" class="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <div class="space-y-2"><Label>{{ t('admin.settings.orderRiskControl.rateLimit.windowSeconds') }}</Label><Input v-model.number="form.guest.rate_limit.window_seconds" type="number" min="10" max="3600" /></div>
            <div class="space-y-2"><Label>{{ t('admin.settings.orderRiskControl.rateLimit.maxRequests') }}</Label><Input v-model.number="form.guest.rate_limit.max_requests" type="number" min="1" max="100" /></div>
            <div class="space-y-2"><Label>{{ t('admin.settings.orderRiskControl.rateLimit.blockSeconds') }}</Label><Input v-model.number="form.guest.rate_limit.block_seconds" type="number" min="0" max="86400" /></div>
          </div>
        </div>
      </div>
    </section>

    <section v-show="form.enabled" class="overflow-hidden rounded-xl border bg-card">
      <div class="border-b bg-sky-50/50 p-5 dark:bg-sky-950/10">
        <div class="flex items-start justify-between gap-4">
          <div class="flex items-start gap-3">
            <div class="rounded-lg bg-sky-600 p-2 text-white shadow-sm"><UserRound class="h-5 w-5" /></div>
            <div>
              <h3 class="text-base font-semibold">{{ t('admin.settings.orderRiskControl.member.title') }}</h3>
              <p class="mt-1 max-w-2xl text-xs leading-5 text-muted-foreground">{{ t('admin.settings.orderRiskControl.member.subtitle') }}</p>
            </div>
          </div>
          <Switch v-model="form.member.enabled" />
        </div>
      </div>
      <div v-show="form.member.enabled" class="space-y-6 p-5">
        <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
          <div class="space-y-2"><Label>{{ t('admin.settings.orderRiskControl.member.maxPendingPerUser') }}</Label><Input v-model.number="form.member.max_pending_orders_per_user" type="number" min="0" max="100" /><p class="text-xs leading-5 text-muted-foreground">{{ t('admin.settings.orderRiskControl.member.maxPendingPerUserHint') }}</p></div>
          <div class="space-y-2"><Label>{{ t('admin.settings.orderRiskControl.member.maxPendingPerIP') }}</Label><Input v-model.number="form.member.max_pending_orders_per_ip" type="number" min="0" max="100" /><p class="text-xs leading-5 text-muted-foreground">{{ t('admin.settings.orderRiskControl.member.maxPendingPerIPHint') }}</p></div>
          <div class="space-y-2"><Label>{{ t('admin.settings.orderRiskControl.member.maxQuantityPerProduct') }}</Label><Input v-model.number="form.member.max_quantity_per_product_per_order" type="number" min="0" max="100000" /><p class="text-xs leading-5 text-muted-foreground">{{ t('admin.settings.orderRiskControl.member.maxQuantityPerProductHint') }}</p></div>
        </div>
        <div class="rounded-lg border p-4">
          <div class="mb-4 flex items-center justify-between gap-4">
            <div><Label>{{ t('admin.settings.orderRiskControl.member.rateLimitTitle') }}</Label><p class="mt-1 text-xs text-muted-foreground">{{ t('admin.settings.orderRiskControl.member.rateLimitHint') }}</p></div>
            <Switch v-model="form.member.rate_limit.enabled" />
          </div>
          <div v-show="form.member.rate_limit.enabled" class="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <div class="space-y-2"><Label>{{ t('admin.settings.orderRiskControl.rateLimit.windowSeconds') }}</Label><Input v-model.number="form.member.rate_limit.window_seconds" type="number" min="10" max="3600" /></div>
            <div class="space-y-2"><Label>{{ t('admin.settings.orderRiskControl.rateLimit.maxRequests') }}</Label><Input v-model.number="form.member.rate_limit.max_requests" type="number" min="1" max="100" /></div>
            <div class="space-y-2"><Label>{{ t('admin.settings.orderRiskControl.rateLimit.blockSeconds') }}</Label><Input v-model.number="form.member.rate_limit.block_seconds" type="number" min="0" max="86400" /></div>
          </div>
        </div>
      </div>
    </section>

    <section v-show="form.enabled" class="rounded-xl border bg-card p-5">
      <div class="mb-4">
        <h3 class="text-sm font-semibold">{{ t('admin.settings.orderRiskControl.ipBlacklist.title') }}</h3>
        <p class="mt-1 text-xs leading-5 text-muted-foreground">{{ t('admin.settings.orderRiskControl.ipBlacklist.subtitle') }}</p>
      </div>
      <Textarea v-model="form.common.ip_blacklist_text" :placeholder="t('admin.settings.orderRiskControl.ipBlacklist.placeholder')" rows="6" class="font-mono text-sm" />
      <div class="mt-4 space-y-3 border-t pt-4">
        <div class="flex items-center justify-between gap-2">
          <h3 class="font-semibold">{{ t('admin.settings.orderRiskControl.listTitle') }}</h3>
          <Button type="button" size="sm" variant="outline" @click="form.common.settlement_list.push({ kind: 'ip', value: '', allow: false })">
            {{ t('admin.settings.orderRiskControl.listAdd') }}
          </Button>
        </div>
        <div v-for="(entry, index) in form.common.settlement_list" :key="index"
          class="grid gap-2 rounded-lg border p-3 sm:grid-cols-2 lg:grid-cols-[1fr_2fr_1fr_2fr_auto]">
          <select v-model="entry.kind" class="h-9 rounded-md border bg-background px-2"
            :aria-label="t('admin.settings.orderRiskControl.listKind')" @change="() => { if (!canAllow(entry.kind)) entry.allow = false }">
            <option v-for="kind in riskListKinds" :key="kind" :value="kind">
              {{ t('admin.settings.orderRiskControl.listKinds.' + kind) }}
            </option>
          </select>
          <Input v-model="entry.value" :placeholder="t('admin.settings.orderRiskControl.listValue')" />
          <select v-model="entry.allow" class="h-9 rounded-md border bg-background px-2"
            :aria-label="t('admin.settings.orderRiskControl.listAction')">
            <option :value="false">{{ t('admin.settings.orderRiskControl.listDeny') }}</option>
            <option v-if="canAllow(entry.kind)" :value="true">{{ t('admin.settings.orderRiskControl.listAllow') }}</option>
          </select>
          <Input v-model="entry.expires_at" :aria-label="t('admin.settings.orderRiskControl.listExpiry')"
            :placeholder="t('admin.settings.orderRiskControl.listExpiry')" />
          <Button type="button" size="sm" variant="ghost" @click="form.common.settlement_list.splice(index, 1)">
            {{ t('admin.settings.orderRiskControl.listRemove') }}
          </Button>
        </div>
      </div>
    </section>
  </div>
</template>
