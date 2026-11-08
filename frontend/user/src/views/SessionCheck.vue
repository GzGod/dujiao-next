<template>
  <div class="xg-check">
    <section class="xg-check__hero">
      <div class="xg-container">
        <h1 class="xg-check__title">{{ t('xg.check.title') }}</h1>
        <p class="xg-check__lede">{{ t('xg.check.lede') }}</p>
        <ul class="xg-check__facts">
          <li v-for="fact in facts" :key="fact">
            <ShieldCheck class="h-4 w-4" aria-hidden="true" />{{ fact }}
          </li>
        </ul>
      </div>
    </section>

    <section class="xg-container xg-check__body">
      <div class="xg-check__panel">
        <ol class="xg-check__steps">
          <li>
            <span class="xg-check__num">1</span>
            <div>
              <p>{{ t('xg.check.step1') }}</p>
              <a :href="SESSION_URL" target="_blank" rel="noopener noreferrer" class="xg-link xg-check__url">
                chatgpt.com/api/auth/session
                <ExternalLink class="h-3.5 w-3.5" aria-hidden="true" />
              </a>
            </div>
          </li>
          <li>
            <span class="xg-check__num">2</span>
            <p>{{ t('xg.check.step2') }}</p>
          </li>
        </ol>

        <label for="xg-session" class="xg-check__label">{{ t('xg.check.inputLabel') }}</label>
        <textarea
          id="xg-session"
          v-model="raw"
          class="xg-check__input"
          rows="8"
          spellcheck="false"
          autocomplete="off"
          :placeholder="t('xg.check.placeholder')"
          @keydown.meta.enter="runCheck"
          @keydown.ctrl.enter="runCheck"
        ></textarea>
        <p v-if="error" class="xg-check__error" role="alert">{{ t(`xg.check.errors.${error}`) }}</p>

        <div class="xg-check__actions">
          <button type="button" class="xg-btn xg-btn--primary" @click="runCheck">{{ t('xg.check.submit') }}</button>
          <button type="button" class="xg-btn xg-btn--ghost" :disabled="!raw && !result" @click="clearAll">{{ t('xg.check.clear') }}</button>
        </div>

        <p class="xg-check__warn">
          <TriangleAlert class="h-4 w-4 shrink-0" aria-hidden="true" />
          <span>{{ t('xg.check.warning') }}</span>
        </p>
      </div>

      <div class="xg-check__result" aria-live="polite">
        <template v-if="result">
          <div class="xg-check__plan-row">
            <span class="xg-check__plan" :class="`xg-check__plan--${result.plan}`">{{ planLabel }}</span>
            <span class="xg-check__status" :class="{ 'xg-check__status--bad': result.delinquent }">
              {{ result.delinquent ? t('xg.check.delinquent') : t('xg.check.normal') }}
            </span>
          </div>
          <p class="xg-check__verdict">{{ verdict }}</p>

          <dl class="xg-check__rows">
            <div>
              <dt>{{ t('xg.check.fields.email') }}</dt>
              <dd>{{ result.email ? maskEmail(result.email) : '—' }}</dd>
            </div>
            <div>
              <dt>{{ t('xg.check.fields.accountType') }}</dt>
              <dd>{{ result.structure === 'workspace' ? t('xg.check.workspace') : t('xg.check.personal') }}</dd>
            </div>
            <div>
              <dt>{{ t('xg.check.fields.until') }}</dt>
              <dd>{{ result.subscriptionUntil ? formatDate(result.subscriptionUntil) : t('xg.check.untilUnknown') }}</dd>
            </div>
            <div>
              <dt>{{ t('xg.check.fields.session') }}</dt>
              <dd>{{ result.sessionExpires ? formatDate(result.sessionExpires) : '—' }}</dd>
            </div>
          </dl>
          <p class="xg-check__fine">{{ t('xg.check.disclaimer') }}</p>
        </template>

        <div v-else class="xg-check__placeholder">
          <ScanSearch class="h-10 w-10" :stroke-width="1.5" aria-hidden="true" />
          <p>{{ t('xg.check.empty') }}</p>
        </div>
      </div>
    </section>

    <section v-if="result && suggestions.length" class="xg-section xg-section--tint">
      <div class="xg-container">
        <div class="xg-section__head">
          <div>
            <h2 class="xg-h2">{{ t('xg.check.suggestTitle') }}</h2>
            <p class="xg-section__lede">{{ t('xg.check.suggestLede') }}</p>
          </div>
          <router-link to="/products" class="xg-link">{{ t('xg.shop.viewAll') }}</router-link>
        </div>
        <div class="xg-grid">
          <XgProductCard
            v-for="product in suggestions"
            :key="product.id"
            :product="product"
            @open="goToProduct"
            @buy="(p: any) => goToProduct(p.slug)"
          />
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { ExternalLink, ScanSearch, ShieldCheck, TriangleAlert } from 'lucide-vue-next'
import { productAPI } from '../api'
import { useAppStore } from '../stores/app'
import { useLocalized } from '../composables/useProduct'
import { usePageSeo } from '../composables/usePageSeo'
import XgProductCard from '../components/xg/XgProductCard.vue'
import {
  maskEmail,
  parseChatgptSession,
  type PlanKind,
  type SessionCheckError,
  type SessionCheckResult,
} from '../utils/chatgptSession'

const SESSION_URL = 'https://chatgpt.com/api/auth/session'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()
const { getLocalizedText } = useLocalized()

usePageSeo({
  title: () => t('xg.check.title'),
  canonicalPath: () => '/check',
})

const raw = ref('')
const result = ref<SessionCheckResult | null>(null)
const error = ref<SessionCheckError | ''>('')
const products = ref<any[]>([])

const facts = computed(() => [t('xg.check.facts.local'), t('xg.check.facts.noUpload'), t('xg.check.facts.free')])

const runCheck = () => {
  const outcome = parseChatgptSession(raw.value)
  if (outcome.ok) {
    result.value = outcome.result
    error.value = ''
  } else {
    result.value = null
    error.value = outcome.error
  }
}

// 检测完立即从页面上清掉 session 原文，降低凭据留在页面里被截图、被误发的风险
const clearAll = () => {
  raw.value = ''
  result.value = null
  error.value = ''
}

const planLabel = computed(() => {
  if (!result.value) return ''
  if (result.value.plan === 'unknown') return result.value.planRaw || t('xg.check.plans.unknown')
  return t(`xg.check.plans.${result.value.plan}`)
})

const verdict = computed(() => {
  const plan = result.value?.plan
  if (!plan) return ''
  if (result.value?.delinquent) return t('xg.check.verdicts.delinquent')
  return t(`xg.check.verdicts.${plan}`)
})

const formatDate = (iso: string) =>
  new Date(iso).toLocaleString(appStore.locale, { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })

// 按检测到的套餐推荐下一档：免费 / Go → Plus、Go；Plus → Pro；Pro → 继续 Pro；团队 / 企业不推荐个人充值
const suggestPatterns: Record<PlanKind, RegExp[]> = {
  free: [/gpt\s*plus/i, /gpt\s*go/i],
  go: [/gpt\s*plus/i],
  plus: [/gpt\s*pro/i],
  pro: [/gpt\s*pro/i],
  team: [],
  enterprise: [],
  edu: [],
  unknown: [/gpt\s*plus/i, /gpt\s*pro/i],
}

const suggestions = computed(() => {
  const plan = result.value?.plan
  if (!plan) return []
  const patterns = suggestPatterns[plan]
  const picked: any[] = []
  for (const pattern of patterns) {
    for (const product of products.value) {
      if (picked.length >= 3) break
      if (picked.includes(product)) continue
      if (pattern.test(getLocalizedText(product?.title) || '')) picked.push(product)
    }
  }
  return picked
})

const goToProduct = (slug: string) => {
  if (slug) router.push(`/products/${slug}`)
}

onMounted(async () => {
  try {
    const response = await productAPI.list({ page: 1, page_size: 50 })
    const list: any[] = response.data.data || []
    products.value = list.slice().sort((a, b) => Number(a.price_amount || 0) - Number(b.price_amount || 0))
  } catch {
    // 推荐区可选，商品加载失败不影响检测
  }
})
</script>

<style scoped>
.xg-check {
  min-height: 100vh;
  background: var(--neutral-0);
}
.xg-check__hero {
  padding: calc(var(--space-20) + var(--space-8)) 0 var(--space-10);
  background: var(--brand-50);
  border-bottom: 1px solid var(--neutral-200);
}
.xg-check__title {
  font-size: 40px;
  line-height: var(--ui-leading-title);
  font-weight: 800;
  letter-spacing: -0.02em;
  color: var(--neutral-900);
}
.xg-check__lede {
  margin-top: var(--space-3);
  max-width: 600px;
  font-size: 16px;
  line-height: var(--ui-leading-body);
  color: var(--ui-text-muted);
}
.xg-check__facts {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-5);
  margin-top: var(--space-5);
  font-size: 14px;
  color: var(--brand-700);
}
.xg-check__facts li {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
}

.xg-check__body {
  display: grid;
  grid-template-columns: minmax(0, 1.15fr) minmax(0, 1fr);
  gap: var(--space-8);
  padding-top: var(--space-10);
  padding-bottom: var(--space-16);
  align-items: start;
}

.xg-check__steps {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  margin-bottom: var(--space-6);
  font-size: 15px;
  line-height: var(--ui-leading-body);
  color: var(--ui-text-primary);
}
.xg-check__steps li {
  display: flex;
  gap: var(--space-3);
}
.xg-check__num {
  display: inline-flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 999px;
  background: var(--brand-600);
  color: var(--neutral-0);
  font-size: 14px;
  font-weight: 700;
}
.xg-check__url {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  margin-top: var(--space-1);
  word-break: break-all;
}

.xg-check__label {
  display: block;
  margin-bottom: var(--space-2);
  font-size: 14px;
  font-weight: 600;
  color: var(--ui-text-primary);
}
.xg-check__input {
  width: 100%;
  min-height: 180px;
  padding: var(--space-3) var(--space-4);
  border-radius: var(--ui-radius-md);
  border: 1px solid var(--neutral-200);
  background: var(--neutral-25);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 13px;
  line-height: 1.6;
  color: var(--ui-text-primary);
  resize: vertical;
  transition: border-color 160ms ease, box-shadow 160ms ease;
}
.xg-check__input:focus {
  outline: none;
  border-color: var(--brand-600);
  box-shadow: 0 0 0 3px var(--ui-focus-ring);
}
.xg-check__error {
  margin-top: var(--space-2);
  font-size: 14px;
  color: var(--ui-danger);
}
.xg-check__actions {
  display: flex;
  gap: var(--space-3);
  margin-top: var(--space-4);
}
.xg-check__warn {
  display: flex;
  gap: var(--space-2);
  margin-top: var(--space-5);
  padding: var(--space-3) var(--space-4);
  border-radius: var(--ui-radius-sm);
  background: var(--ui-warning-soft);
  font-size: 13px;
  line-height: 1.6;
  color: var(--ui-text-primary);
}
.xg-check__warn svg { margin-top: 3px; color: var(--ui-warning); }

.xg-check__result {
  position: sticky;
  top: calc(var(--space-20) + var(--space-4));
  padding: var(--space-6);
  border-radius: var(--ui-radius-lg);
  border: 1px solid var(--neutral-200);
  background: var(--neutral-0);
  box-shadow: var(--ui-shadow-3);
}
.xg-check__placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-12) var(--space-4);
  text-align: center;
  font-size: 14px;
  color: var(--ui-text-muted);
}
.xg-check__placeholder svg { color: var(--brand-200); }

.xg-check__plan-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
}
.xg-check__plan {
  font-size: 32px;
  font-weight: 800;
  letter-spacing: -0.02em;
  color: var(--neutral-900);
}
.xg-check__plan--plus,
.xg-check__plan--pro { color: var(--brand-700); }
.xg-check__status {
  padding: 0 var(--space-3);
  border-radius: 999px;
  font-size: 13px;
  line-height: 26px;
  color: var(--ui-success);
  background: var(--ui-success-soft);
}
.xg-check__status--bad {
  color: var(--ui-danger);
  background: var(--ui-danger-soft);
}
.xg-check__verdict {
  margin-top: var(--space-2);
  font-size: 15px;
  line-height: var(--ui-leading-body);
  color: var(--ui-text-primary);
}
.xg-check__rows {
  margin-top: var(--space-5);
  border-top: 1px solid var(--neutral-200);
}
.xg-check__rows div {
  display: flex;
  justify-content: space-between;
  gap: var(--space-4);
  padding: var(--space-3) 0;
  border-bottom: 1px solid var(--neutral-100);
  font-size: 14px;
}
.xg-check__rows dt { color: var(--ui-text-muted); }
.xg-check__rows dd {
  min-width: 0;
  text-align: right;
  word-break: break-all;
  color: var(--ui-text-primary);
  font-weight: 500;
}
.xg-check__fine {
  margin-top: var(--space-4);
  font-size: 12px;
  line-height: 1.6;
  color: var(--ui-text-muted);
}

@media (max-width: 1023px) {
  .xg-check__body { grid-template-columns: minmax(0, 1fr); }
  .xg-check__result { position: static; }
}
@media (max-width: 640px) {
  .xg-check__hero { padding-top: calc(var(--space-16) + var(--space-4)); padding-bottom: var(--space-8); }
  .xg-check__title { font-size: 28px; }
  .xg-check__body { padding-top: var(--space-6); gap: var(--space-6); }
  .xg-check__actions .xg-btn { flex: 1; }
  .xg-check__result { padding: var(--space-5); }
  .xg-check__plan { font-size: 26px; }
}
</style>
