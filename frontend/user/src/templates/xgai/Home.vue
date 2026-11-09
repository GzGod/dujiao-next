<template>
  <div class="xhome">
    <!-- Hero -->
    <section class="xhero">
      <div class="xhero__glow" aria-hidden="true" />
      <div class="x-container xhero__layout">
        <div class="xhero__copy">
          <div class="xhero__badge">
            <span>{{ t('xgai.hero.badgeLead') }}</span>
            <i />
            {{ t('xgai.hero.badgeTail') }}
          </div>
          <h1>
            {{ t('xgai.hero.titleLead') }}<br />
            <em>{{ t('xgai.hero.titleAccent') }}</em>
          </h1>
          <p>{{ t('xgai.hero.lede') }}</p>
          <div class="xhero__actions">
            <a href="#products" class="x-btn x-btn--primary x-btn--lg" @click.prevent="scrollTo('products')">
              {{ t('xgai.hero.primary') }} <ArrowRight />
            </a>
            <a href="#process" class="xhero__secondary" @click.prevent="scrollTo('process')">
              <span aria-hidden="true">▶</span> {{ t('xgai.hero.secondary') }}
            </a>
          </div>
          <div class="xhero__proof">
            <span v-for="item in heroProof" :key="item"><Check /> {{ item }}</span>
          </div>
        </div>

        <!-- 套餐推荐：按使用频率映射到真实商品，价格实时 -->
        <div class="xrec x-glass">
          <div class="xrec__top">
            <span>
              <small>{{ t('xgai.rec.eyebrow') }}</small>
              <b>{{ t('xgai.rec.title') }}</b>
            </span>
          </div>
          <p class="xrec__q">{{ t('xgai.rec.question') }}</p>
          <div class="xrec__options" role="radiogroup">
            <button
              v-for="option in usageOptions"
              :key="option.value"
              type="button"
              role="radio"
              :aria-checked="usage === option.value"
              :class="{ 'is-active': usage === option.value }"
              @click="usage = option.value"
            >
              <i><Check v-if="usage === option.value" /></i>
              <span>
                <b>{{ option.title }}</b>
                <small>{{ option.detail }}</small>
              </span>
            </button>
          </div>
          <div class="xrec__result">
            <span>
              <small>{{ t('xgai.rec.for') }}</small>
              <b>{{ recommendedTitle }}</b>
              <p>{{ t(`xgai.rec.reason.${usage}`) }}</p>
            </span>
            <strong v-if="recommendedProduct"><i>¥</i>{{ formatPrice(recommendedProduct.price_amount) }}</strong>
          </div>
          <button type="button" class="x-btn x-btn--primary x-btn--block" :disabled="!recommendedProduct" @click="recommendedProduct && goToProduct(recommendedProduct.slug)">
            {{ t('xgai.rec.cta') }} <ArrowRight />
          </button>
          <small class="xrec__note">{{ t('xgai.rec.note') }}</small>
        </div>
      </div>
    </section>

    <!-- 信任条：只写真实能力，不放编造的数字 -->
    <section class="xtrust">
      <div class="x-container xtrust__row">
        <div v-for="item in trustItems" :key="item.title">
          <strong>{{ item.title }}</strong>
          <span>{{ item.text }}</span>
        </div>
      </div>
    </section>

    <!-- 商品 -->
    <section id="products" class="x-container x-section xprod">
      <div class="xprod__head">
        <span>
          <small class="x-eyebrow">{{ t('xgai.products.eyebrow') }}</small>
          <h2 class="x-h2">{{ t('xgai.products.title') }}</h2>
          <p class="x-lede">{{ t('xgai.products.lede') }}</p>
        </span>
        <div class="x-segment" role="group" :aria-label="t('xgai.products.filter')">
          <button v-for="item in filters" :key="item.value" type="button" :class="{ 'is-active': filter === item.value }" @click="filter = item.value">
            {{ item.label }}
          </button>
        </div>
      </div>

      <div v-if="loading" class="xprod__grid">
        <div v-for="n in 6" :key="n" class="xprod__skeleton" />
      </div>
      <div v-else-if="visibleProducts.length" class="xprod__grid">
        <XgaiProductCard
          v-for="product in visibleProducts"
          :key="product.id"
          :product="product"
          :featured="product.id === featuredId"
          @open="goToProduct"
          @buy="openQuickBuy"
        />
      </div>
      <p v-else class="xprod__empty">{{ t('home.featured.empty') }}</p>

      <div class="xprod__help">
        <span>
          <b>{{ t('xgai.products.helpTitle') }}</b>
          {{ t('xgai.products.helpText') }}
        </span>
        <a href="#checker" class="x-link" @click.prevent="scrollTo('checker')">{{ t('xgai.products.helpCta') }} <ArrowUpRight /></a>
      </div>
    </section>

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
          <p>{{ step.text }}</p>
        </article>
      </div>
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

    <ProductQuickBuy
      v-if="quickBuyProduct"
      :product="quickBuyProduct"
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
  ArrowRight, ArrowUpRight, Check, ClipboardList, CreditCard, ExternalLink, ListChecks, PackageCheck,
  ScanSearch, ShieldCheck, Wallet,
} from 'lucide-vue-next'
import { productAPI } from '../../api'
import { useAppStore } from '../../stores/app'
import { useLocalized } from '../../composables/useProduct'
import { usePageSeo } from '../../composables/usePageSeo'
import ProductQuickBuy from '../../components/ProductQuickBuy.vue'
import XgaiProductCard from './components/XgaiProductCard.vue'
import { detectAiBrand, sortByBrandThenPrice, type AiBrand } from '../../utils/aiBrand'
import { maskEmail, parseChatgptSession, type PlanKind, type SessionCheckError, type SessionCheckResult } from '../../utils/chatgptSession'
import '../xgai/styles/xgai-home.css'

const SESSION_URL = 'https://chatgpt.com/api/auth/session'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()
const { getLocalizedText } = useLocalized()

usePageSeo({ title: () => t('xgai.seo.title'), canonicalPath: () => '/' })

// ---------- 商品 ----------
const products = ref<any[]>([])
const loading = ref(true)
type Filter = 'all' | AiBrand
const filter = ref<Filter>('all')

const titleOf = (p: any) => getLocalizedText(p?.title) || ''
const findByTitle = (pattern: RegExp) =>
  products.value
    .filter((p) => pattern.test(titleOf(p)))
    .sort((a, b) => Number(a.price_amount || 0) - Number(b.price_amount || 0))[0] || null

const brandsPresent = computed(() => new Set(products.value.map((p) => detectAiBrand(p))))
const filters = computed(() => {
  const list: { value: Filter; label: string }[] = [{ value: 'all', label: t('xgai.products.all') }]
  if (brandsPresent.value.has('openai')) list.push({ value: 'openai', label: 'ChatGPT' })
  if (brandsPresent.value.has('grok')) list.push({ value: 'grok', label: 'Grok' })
  if (brandsPresent.value.has('other')) list.push({ value: 'other', label: t('xgai.products.other') })
  return list
})
const visibleProducts = computed(() => {
  const list = filter.value === 'all' ? products.value : products.value.filter((p) => detectAiBrand(p) === filter.value)
  return sortByBrandThenPrice(list)
})
// 「最多人选」只是视觉强调，用 GPT Plus 这款主力商品，不附加任何销量数字
const featuredId = computed(() => findByTitle(/gpt\s*plus/i)?.id ?? null)

const formatPrice = (amount: unknown) => {
  const n = Number(amount)
  if (!Number.isFinite(n)) return '—'
  return Number.isInteger(n) ? String(n) : n.toFixed(2)
}

// ---------- 套餐推荐 ----------
type Usage = 'light' | 'daily' | 'pro'
const usage = ref<Usage>('daily')
const usageOptions = computed(() => ([
  { value: 'light' as const, title: t('xgai.rec.light'), detail: t('xgai.rec.lightDetail') },
  { value: 'daily' as const, title: t('xgai.rec.daily'), detail: t('xgai.rec.dailyDetail') },
  { value: 'pro' as const, title: t('xgai.rec.pro'), detail: t('xgai.rec.proDetail') },
]))
const usagePattern: Record<Usage, RegExp> = { light: /gpt\s*go/i, daily: /gpt\s*plus/i, pro: /gpt\s*pro/i }
const recommendedProduct = computed(() => findByTitle(usagePattern[usage.value]))
const recommendedTitle = computed(() => (recommendedProduct.value ? titleOf(recommendedProduct.value) : '—'))

// ---------- 首页文案块（全部为真实能力，不含编造数据） ----------
const heroProof = computed(() => [t('xgai.hero.proof1'), t('xgai.hero.proof2'), t('xgai.hero.proof3')])
const trustItems = computed(() => [
  { title: t('xgai.trust.payTitle'), text: t('xgai.trust.payText') },
  { title: t('xgai.trust.autoTitle'), text: t('xgai.trust.autoText') },
  { title: t('xgai.trust.ownTitle'), text: t('xgai.trust.ownText') },
  { title: t('xgai.trust.trackTitle'), text: t('xgai.trust.trackText') },
])
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
const openQuickBuy = (product: any) => {
  quickBuyProduct.value = product
  quickBuyVisible.value = true
}
const goToProduct = (slug: string) => {
  if (slug) router.push(`/products/${slug}`)
}
const scrollTo = (id: string) => {
  document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
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
