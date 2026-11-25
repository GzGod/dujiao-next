<template>
  <div class="xhome">
    <section class="xshop-intro x-container">
      <div><small class="x-eyebrow">{{ t('xgai.products.eyebrow') }}</small><h1>{{ t('xgai.shop.title') }}</h1><p>{{ t('xgai.shop.description') }}</p></div>
      <div class="xshop-intro__links"><a :href="'https://redeem.xuegaogx.cc/'" class="x-btn x-btn--primary">{{ t('xgai.nav.redeem') }} <ArrowUpRight /></a><RouterLink :to="userAuthStore.isAuthenticated ? '/me/orders' : '/guest/orders'" class="x-btn">{{ t('xgai.shop.orders') }} <ArrowRight /></RouterLink></div>
    </section>

    <!-- 商品 -->
    <section id="products" class="x-container x-section xprod">
      <div class="xprod__head">
        <GlassSegment v-model="filter" :options="filters" :label="t('xgai.products.filter')" />
      </div>

      <div v-if="loading" class="xprod__grid">
        <div v-for="n in 6" :key="n" class="xprod__skeleton" />
      </div>
      <TransitionGroup v-else-if="visibleProducts.length" tag="div" name="glass-cards" appear class="xprod__grid" :key="filter">
        <XgaiProductCard
          v-for="product in visibleProducts"
          :key="product.id"
          :product="product"
          :featured="false"
          @open="goToProduct"
          @buy="openQuickBuy"
        />
      </TransitionGroup>
      <p v-else class="xprod__empty">{{ t('home.featured.empty') }}</p>

      <div class="xprod__help">
        <span>
          <b>{{ t('xgai.products.helpTitle') }}</b>
          {{ t('xgai.products.helpText') }}
        </span>
        <a href="#checker" class="x-link" @click.prevent="scrollTo('checker')">{{ t('xgai.products.helpCta') }} <ArrowUpRight /></a>
      </div>
    </section>

    <details class="xshop-help x-container"><summary>{{ t('xgai.shop.help') }}</summary>
    <!-- 账号检测 -->
    <section id="checker" class="xcheck">
      <div class="x-container xcheck__layout">
        <div class="xcheck__copy">
          <small class="x-eyebrow">{{ t('xgai.checker.eyebrow') }}</small>
          <h2 class="x-h2">{{ t('xgai.checker.titleLead') }}<br />{{ t('xgai.checker.titleTail') }}</h2>
          <p class="x-lede">{{ t('xgai.checker.lede') }}</p>
          <div class="xcheck__privacy">
            <ShieldCheck />
            <span>
              <b>{{ t('xgai.checker.privacyTitle') }}</b>
              <small>{{ t('xgai.checker.privacyText') }}</small>
            </span>
          </div>
          <ol class="xcheck__steps">
            <li>
              <span>1</span>
              <p>{{ t('xg.check.step1') }}
                <a :href="SESSION_URL" target="_blank" rel="noopener noreferrer">chatgpt.com/api/auth/session <ExternalLink /></a>
              </p>
            </li>
            <li><span>2</span><p>{{ t('xg.check.step2') }}</p></li>
          </ol>
        </div>

        <form class="xcheck__card x-glass" @submit.prevent="runCheck">
          <header>
            <span><i><ShieldCheck /></i><b>{{ t('xg.check.title') }}</b></span>
            <em><i /> {{ t('xgai.checker.local') }}</em>
          </header>

          <template v-if="result">
            <div class="xcheck__plan">
              <strong :class="{ 'is-paid': result.plan === 'plus' || result.plan === 'pro' }">{{ planLabel }}</strong>
              <span :class="{ 'is-bad': result.delinquent }">{{ result.delinquent ? t('xg.check.delinquent') : t('xg.check.normal') }}</span>
            </div>
            <p class="xcheck__verdict">{{ verdict }}</p>
            <dl class="xcheck__rows">
              <div><dt>{{ t('xg.check.fields.email') }}</dt><dd>{{ result.email ? maskEmail(result.email) : '—' }}</dd></div>
              <div><dt>{{ t('xg.check.fields.accountType') }}</dt><dd>{{ result.structure === 'workspace' ? t('xg.check.workspace') : t('xg.check.personal') }}</dd></div>
              <div><dt>{{ t('xg.check.fields.until') }}</dt><dd>{{ result.subscriptionUntil ? formatDate(result.subscriptionUntil) : t('xg.check.untilUnknown') }}</dd></div>
            </dl>
            <div class="xcheck__actions">
              <button v-if="suggestion" type="button" class="x-btn x-btn--primary" @click="goToProduct(suggestion.slug)">
                {{ t('xg.check.buy', { name: getLocalizedText(suggestion.title) }) }} <ArrowRight />
              </button>
              <button type="button" class="x-btn x-btn--ghost" @click="clearCheck">{{ t('xg.check.again') }}</button>
            </div>
          </template>

          <template v-else>
            <label for="xgai-session">{{ t('xg.check.inputLabel') }}</label>
            <textarea
              id="xgai-session"
              v-model="raw"
              spellcheck="false"
              autocomplete="off"
              :placeholder="t('xg.check.placeholder')"
              @input="error = ''"
              @keydown.meta.enter="runCheck"
              @keydown.ctrl.enter="runCheck"
            />
            <p v-if="error" class="xcheck__error" role="alert">{{ t(`xg.check.errors.${error}`) }}</p>
            <button type="submit" class="x-btn x-btn--primary x-btn--block">{{ t('xg.check.submit') }} <ArrowRight /></button>
          </template>

          <p class="xcheck__foot"><ShieldCheck /> {{ t('xgai.checker.foot') }}</p>
        </form>
      </div>
    </section>

    <!-- 流程 -->
    <section id="process" class="x-container x-section xflow">
      <div class="xflow__head">
        <small class="x-eyebrow">{{ t('xgai.process.eyebrow') }}</small>
        <h2 class="x-h2">{{ t('xgai.process.title') }}</h2>
        <p class="x-lede">{{ t('xgai.process.lede') }}</p>
      </div>
      <div class="xflow__grid">
        <article v-for="(step, index) in processSteps" :key="step.title">
          <span>{{ String(index + 1).padStart(2, '0') }}</span>
          <i><component :is="step.icon" /></i>
          <h3>{{ step.title }}</h3>
          <p v-if="index !== 2">{{ step.text }}</p>
          <div v-else class="xflow__delivery">
            <div><b>GPT</b><p>{{ t('xgai.process.gptDelivery') }}</p></div>
            <div><b>Claude</b><p>{{ t('xgai.process.claudeDelivery') }}</p></div>
          </div>
          <div v-if="index === 2" class="xflow__step-links">
            <a :href="appStore.redemptionUrl()" target="_blank" rel="noopener noreferrer">{{ t('xgai.process.startRedeem') }} ↗</a>
            <a href="https://t.me/xuegaozhanshen" target="_blank" rel="noopener noreferrer">{{ t('xgai.process.support') }} ↗</a>
          </div>
          <div v-if="index === 3" class="xflow__step-links">
            <a :href="appStore.redemptionUrl('/progress')" target="_blank" rel="noopener noreferrer">{{ t('xgai.process.progress') }} ↗</a>
          </div>
        </article>
      </div>
      <aside class="xflow__help">
        <div>
          <h3>{{ t('xgai.process.helpTitle') }}</h3>
          <p>{{ t('xgai.process.helpText') }}</p>
        </div>
        <div class="xflow__actions">
          <a href="/guides/redeem">{{ t('xgai.process.guide') }}</a>
          <RouterLink :to="userAuthStore.isAuthenticated ? '/me/orders' : '/guest/orders'">{{ t('xgai.process.orders') }}</RouterLink>
          <a href="https://t.me/xuegaozhanshen" target="_blank" rel="noopener noreferrer">{{ t('xgai.process.support') }}</a>
        </div>
      </aside>
    </section>

    <!-- 保障 -->
    <section class="xguard">
      <div class="x-container xguard__layout">
        <div class="xguard__copy">
          <small>{{ t('xgai.guard.eyebrow') }}</small>
          <h2>{{ t('xgai.guard.title') }}</h2>
          <p>{{ t('xgai.guard.lede') }}</p>
        </div>
        <div class="xguard__grid">
          <article v-for="item in guardItems" :key="item.title">
            <span><component :is="item.icon" /></span>
            <b>{{ item.title }}</b>
            <p>{{ item.text }}</p>
          </article>
        </div>
      </div>
    </section>

    </details>

    <ProductQuickBuy
      inline-checkout
      v-if="quickBuyProduct"
      :product="quickBuyProduct"
      :source-element="quickBuySource"
      :visible="quickBuyVisible"
      @update:visible="quickBuyVisible = $event"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import {
  ArrowRight, ArrowUpRight, ClipboardList, CreditCard, ExternalLink, ListChecks, PackageCheck,
  ScanSearch, ShieldCheck, Wallet,
} from 'lucide-vue-next'
import { productAPI } from '../../api'
import { useAppStore } from '../../stores/app'
import { useUserAuthStore } from '../../stores/userAuth'
import { useLocalized } from '../../composables/useProduct'
import { usePageSeo } from '../../composables/usePageSeo'
import ProductQuickBuy from '../../components/ProductQuickBuy.vue'
import XgaiProductCard from './components/XgaiProductCard.vue'
import GlassSegment from './components/GlassSegment.vue'
import './styles/glass-commerce.css'
import { detectAiBrand, sortByBrandThenPrice, isKycServiceProduct } from '../../utils/aiBrand'
import { maskEmail, parseChatgptSession, type PlanKind, type SessionCheckError, type SessionCheckResult } from '../../utils/chatgptSession'
import '../xgai/styles/xgai-home.css'

const SESSION_URL = 'https://chatgpt.com/api/auth/session'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()
const userAuthStore = useUserAuthStore()
const { getLocalizedText } = useLocalized()

usePageSeo({ title: () => t('xgai.seo.title'), description: () => t('xgai.seo.description'), canonicalPath: () => '/' })

// ---------- 商品 ----------
const products = ref<any[]>([])
const loading = ref(true)
type Filter = 'all' | 'openai' | 'claude' | 'kyc' | 'ip'
const filter = ref<Filter>('all')
const findByTitle = (pattern: RegExp) => products.value.filter(p => pattern.test(getLocalizedText(p.title))).sort((a,b) => Number(a.price_amount)-Number(b.price_amount))[0] || null
const isKycProduct = isKycServiceProduct
const filters = computed(() => [
  { value: 'all' as Filter, label: t('xgai.products.allProducts') },
  { value: 'openai' as Filter, label: 'ChatGPT' },
  { value: 'claude' as Filter, label: 'Claude' },
  { value: 'kyc' as Filter, label: 'KYC' },
  { value: 'ip' as Filter, label: 'IP' },
])
const visibleProducts = computed(() => {
  const list = products.value.filter(p => filter.value === 'all' || (filter.value === 'ip' ? p.category?.slug === 'ip' || p.slug === 'us-residential-ip-monthly' : filter.value === 'kyc' ? isKycProduct(p) : filter.value === 'claude' ? !isKycProduct(p) && /claude/i.test(getLocalizedText(p.title)) : detectAiBrand(p) === 'openai' && !isKycProduct(p)))
  return sortByBrandThenPrice(list)
})
const processSteps = computed(() => [
  { icon: ListChecks, title: t('xgai.process.s1t'), text: t('xgai.process.s1') },
  { icon: ClipboardList, title: t('xgai.process.s2t'), text: t('xgai.process.s2') },
  { icon: CreditCard, title: t('xgai.process.s3t'), text: t('xgai.process.s3') },
  { icon: PackageCheck, title: t('xgai.process.s4t'), text: t('xgai.process.s4') },
])
const guardItems = computed(() => [
  { icon: Wallet, title: t('xgai.guard.payTitle'), text: t('xgai.guard.payText') },
  { icon: ClipboardList, title: t('xgai.guard.trackTitle'), text: t('xgai.guard.trackText') },
  { icon: ScanSearch, title: t('xgai.guard.localTitle'), text: t('xgai.guard.localText') },
])

// ---------- 账号检测（本地解析，复用 utils/chatgptSession） ----------
const raw = ref('')
const result = ref<SessionCheckResult | null>(null)
const error = ref<SessionCheckError | ''>('')
const runCheck = () => {
  const outcome = parseChatgptSession(raw.value)
  if (outcome.ok) {
    result.value = outcome.result
    error.value = ''
    raw.value = ''
  } else {
    result.value = null
    error.value = outcome.error
  }
}
const clearCheck = () => {
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
  if (!result.value) return ''
  return result.value.delinquent ? t('xg.check.verdicts.delinquent') : t(`xg.check.verdicts.${result.value.plan}`)
})
const suggestPattern: Record<PlanKind, RegExp | null> = {
  free: /gpt\s*plus/i, go: /gpt\s*plus/i, plus: /gpt\s*pro/i, pro: /gpt\s*pro/i,
  team: null, enterprise: null, edu: null, unknown: null,
}
const suggestion = computed(() => {
  if (!result.value || result.value.delinquent) return null
  const pattern = suggestPattern[result.value.plan]
  return pattern ? findByTitle(pattern) : null
})
const formatDate = (iso: string) =>
  new Date(iso).toLocaleString(appStore.locale, { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })

// ---------- 交互 ----------
const quickBuyProduct = ref<any>(null)
const quickBuyVisible = ref(false)
const quickBuySource = ref<HTMLElement | undefined>()
const openQuickBuy = (product: any, source?: HTMLElement) => {
  quickBuySource.value = source
  quickBuyProduct.value = product
  quickBuyVisible.value = true
}
const goToProduct = (slug: string) => {
  if (slug) router.push(`/products/${slug}`)
}
const scrollTo = (id: string) => {
  const target = document.getElementById(id)
  const disclosure = target?.closest('details')
  if (disclosure) disclosure.open = true
  target?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

onMounted(async () => {
  try {
    const response = await productAPI.list({ page: 1, page_size: 50 })
    products.value = response.data.data || []
  } catch (err) {
    console.error('Failed to load products:', err)
  } finally {
    loading.value = false
  }
})
</script>

<style scoped>
.xshop-intro{display:flex;align-items:center;justify-content:space-between;gap:24px;margin-top:24px;padding-block:28px;border-bottom:1px solid var(--x-line)}
.xshop-intro h1{font-size:28px;line-height:1.3;margin:8px 0;color:var(--x-ink);letter-spacing:-.025em}
.xshop-intro p{margin:0;color:var(--x-ink-3);font-size:14px;line-height:1.6}
.xshop-intro__links{display:flex;gap:12px;flex-shrink:0}.xshop-intro__links .x-btn{border:1px solid var(--x-line)}
.xhome .xprod{padding-block:24px 32px}.xhome .xprod__head{margin-bottom:20px;display:flex;justify-content:flex-start}.xhome .xprod__grid{grid-template-columns:repeat(4,minmax(0,1fr));gap:16px}
.xhome :deep(.xpc){height:340px;min-height:340px;padding:20px;background:var(--x-surface);box-shadow:none;transition:border-color .3s,box-shadow .3s}.xhome :deep(.xpc:hover){transform:none;box-shadow:none;border-color:var(--x-blue)}
.xhome :deep(.xpc--expanded){box-shadow:0 12px 32px rgba(0,70,150,.12);border-color:var(--x-blue)}.xhome :deep(.xpc__name b){font-size:16px;line-height:1.5;white-space:normal;overflow:visible;text-overflow:clip}.xhome :deep(.xpc__brand){align-items:flex-start;min-height:96px}.xhome :deep(.xpc__price strong){font-size:30px}
.xshop-help{padding-block:20px 40px}.xshop-help>summary{cursor:pointer;color:var(--x-ink-2);font-size:14px;padding:16px 0;border-top:1px solid var(--x-line)}
@media(max-width:1100px){.xhome .xprod__grid{grid-template-columns:repeat(3,minmax(0,1fr))}}
@media(max-width:760px){.xshop-intro{align-items:flex-start;flex-direction:column;margin-top:8px;padding-block:20px}.xshop-intro h1{font-size:24px}.xhome .xprod__grid{grid-template-columns:repeat(2,minmax(0,1fr))}.xhome :deep(.xpc){padding:16px}.xshop-intro__links{flex-wrap:wrap}}
@media(max-width:440px){.xhome .xprod__grid{grid-template-columns:1fr}.xshop-intro h1{font-size:22px}}
</style>
