<template>
  <div class="xg-checker">
    <div class="xg-checker__head">
      <h2 class="xg-checker__title">{{ t('xg.check.title') }}</h2>
      <span class="xg-checker__badge">
        <ShieldCheck class="h-3.5 w-3.5" aria-hidden="true" />{{ t('xg.check.badge') }}
      </span>
    </div>

    <!-- 检测结果 -->
    <div v-if="result" aria-live="polite">
      <div class="xg-checker__plan-row">
        <span class="xg-checker__plan" :class="`xg-checker__plan--${result.plan}`">{{ planLabel }}</span>
        <span class="xg-checker__status" :class="{ 'xg-checker__status--bad': result.delinquent }">
          {{ result.delinquent ? t('xg.check.delinquent') : t('xg.check.normal') }}
        </span>
      </div>
      <p class="xg-checker__verdict">{{ verdict }}</p>

      <dl class="xg-checker__rows">
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
      </dl>

      <div class="xg-checker__actions">
        <button v-if="suggestion" type="button" class="xg-btn xg-btn--primary" @click="emit('open', suggestion.slug)">
          {{ t('xg.check.buy', { name: suggestionTitle }) }}
        </button>
        <button type="button" class="xg-btn xg-btn--ghost" @click="clearAll">{{ t('xg.check.again') }}</button>
      </div>
      <p class="xg-checker__fine">{{ t('xg.check.disclaimer') }}</p>
    </div>

    <!-- 输入 -->
    <div v-else>
      <ol class="xg-checker__steps">
        <li>
          <span class="xg-checker__num">1</span>
          <span>
            {{ t('xg.check.step1') }}
            <a :href="SESSION_URL" target="_blank" rel="noopener noreferrer" class="xg-checker__url">
              chatgpt.com/api/auth/session<ExternalLink class="h-3 w-3" aria-hidden="true" />
            </a>
          </span>
        </li>
        <li>
          <span class="xg-checker__num">2</span>
          <span>{{ t('xg.check.step2') }}</span>
        </li>
      </ol>

      <label :for="inputId" class="sr-only">{{ t('xg.check.inputLabel') }}</label>
      <textarea
        :id="inputId"
        v-model="raw"
        class="xg-checker__input"
        rows="5"
        spellcheck="false"
        autocomplete="off"
        :placeholder="t('xg.check.placeholder')"
        @keydown.meta.enter="runCheck"
        @keydown.ctrl.enter="runCheck"
      ></textarea>
      <p v-if="error" class="xg-checker__error" role="alert">{{ t(`xg.check.errors.${error}`) }}</p>

      <div class="xg-checker__actions">
        <button type="button" class="xg-btn xg-btn--primary" @click="runCheck">{{ t('xg.check.submit') }}</button>
        <button type="button" class="xg-btn xg-btn--ghost" :disabled="!raw" @click="clearAll">{{ t('xg.check.clear') }}</button>
      </div>
      <p class="xg-checker__warn">
        <TriangleAlert class="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
        <span>{{ t('xg.check.warning') }}</span>
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ExternalLink, ShieldCheck, TriangleAlert } from 'lucide-vue-next'
import { useAppStore } from '../../stores/app'
import { useLocalized } from '../../composables/useProduct'
import {
  maskEmail,
  parseChatgptSession,
  type PlanKind,
  type SessionCheckError,
  type SessionCheckResult,
} from '../../utils/chatgptSession'

const props = defineProps<{ products: any[] }>()
const emit = defineEmits<{ open: [slug: string] }>()

const SESSION_URL = 'https://chatgpt.com/api/auth/session'
const inputId = 'xg-session-input'

const { t } = useI18n()
const appStore = useAppStore()
const { getLocalizedText } = useLocalized()

const raw = ref('')
const result = ref<SessionCheckResult | null>(null)
const error = ref<SessionCheckError | ''>('')

// Session 只在本组件内存里解析；解析成功后立即清掉原文，避免凭据留在页面上
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
  if (!result.value) return ''
  if (result.value.delinquent) return t('xg.check.verdicts.delinquent')
  return t(`xg.check.verdicts.${result.value.plan}`)
})

const formatDate = (iso: string) =>
  new Date(iso).toLocaleString(appStore.locale, { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })

// 按检测到的套餐推荐下一档：免费 → Plus；Go → Plus；Plus / Pro → Pro；团队 / 企业 / 教育 / 欠费不推荐
const suggestPattern: Record<PlanKind, RegExp | null> = {
  free: /gpt\s*plus/i,
  go: /gpt\s*plus/i,
  plus: /gpt\s*pro/i,
  pro: /gpt\s*pro/i,
  team: null,
  enterprise: null,
  edu: null,
  unknown: null,
}

const suggestion = computed(() => {
  if (!result.value || result.value.delinquent) return null
  const pattern = suggestPattern[result.value.plan]
  if (!pattern) return null
  const matches = props.products.filter((p) => pattern.test(getLocalizedText(p?.title) || ''))
  if (!matches.length) return null
  return matches.reduce((a, b) => (Number(a?.price_amount || 0) <= Number(b?.price_amount || 0) ? a : b))
})
const suggestionTitle = computed(() => getLocalizedText(suggestion.value?.title) || '')
</script>

<style scoped>
.xg-checker {
  padding: var(--space-6);
  border-radius: var(--ui-radius-lg);
  border: 1px solid var(--neutral-200);
  background: var(--neutral-0);
  box-shadow: var(--ui-shadow-3);
}
.xg-checker__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  margin-bottom: var(--space-4);
}
.xg-checker__title {
  font-size: 18px;
  font-weight: 700;
  color: var(--neutral-900);
}
.xg-checker__badge {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  padding: 0 var(--space-2);
  border-radius: 999px;
  font-size: 12px;
  line-height: 22px;
  white-space: nowrap;
  color: var(--brand-700);
  background: var(--brand-50);
}

.xg-checker__steps {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin-bottom: var(--space-3);
  font-size: 13px;
  line-height: 1.6;
  color: var(--ui-text-secondary);
}
.xg-checker__steps li { display: flex; gap: var(--space-2); }
.xg-checker__num {
  display: inline-flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  margin-top: 1px;
  border-radius: 999px;
  background: var(--brand-600);
  color: var(--neutral-0);
  font-size: 12px;
  font-weight: 700;
}
.xg-checker__url {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-weight: 600;
  color: var(--brand-600);
  word-break: break-all;
}
.xg-checker__url:hover { color: var(--brand-700); }

.xg-checker__input {
  width: 100%;
  min-height: 120px;
  padding: var(--space-3);
  border-radius: var(--ui-radius-sm);
  border: 1px solid var(--neutral-200);
  background: var(--neutral-25);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  line-height: 1.6;
  color: var(--ui-text-primary);
  resize: vertical;
  transition: border-color 160ms ease, box-shadow 160ms ease;
}
.xg-checker__input:focus {
  outline: none;
  border-color: var(--brand-600);
  box-shadow: 0 0 0 3px var(--ui-focus-ring);
}
.xg-checker__error {
  margin-top: var(--space-2);
  font-size: 13px;
  color: var(--ui-danger);
}
.xg-checker__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  margin-top: var(--space-4);
}
.xg-checker__actions .xg-btn { flex: 1 1 auto; }
.xg-checker__warn {
  display: flex;
  gap: var(--space-2);
  margin-top: var(--space-4);
  font-size: 12px;
  line-height: 1.6;
  color: var(--ui-text-muted);
}
.xg-checker__warn svg { margin-top: 3px; color: var(--ui-warning); }

.xg-checker__plan-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
}
.xg-checker__plan {
  font-size: 32px;
  font-weight: 800;
  letter-spacing: -0.02em;
  color: var(--neutral-900);
}
.xg-checker__plan--plus,
.xg-checker__plan--pro { color: var(--brand-700); }
.xg-checker__status {
  padding: 0 var(--space-3);
  border-radius: 999px;
  font-size: 13px;
  line-height: 26px;
  white-space: nowrap;
  color: var(--ui-success);
  background: var(--ui-success-soft);
}
.xg-checker__status--bad { color: var(--ui-danger); background: var(--ui-danger-soft); }
.xg-checker__verdict {
  margin-top: var(--space-2);
  font-size: 14px;
  line-height: var(--ui-leading-body);
  color: var(--ui-text-primary);
}
.xg-checker__rows {
  margin-top: var(--space-4);
  border-top: 1px solid var(--neutral-200);
}
.xg-checker__rows div {
  display: flex;
  justify-content: space-between;
  gap: var(--space-4);
  padding: var(--space-2) 0;
  border-bottom: 1px solid var(--neutral-100);
  font-size: 13px;
}
.xg-checker__rows dt { color: var(--ui-text-muted); }
.xg-checker__rows dd {
  min-width: 0;
  text-align: right;
  word-break: break-all;
  font-weight: 500;
  color: var(--ui-text-primary);
}
.xg-checker__fine {
  margin-top: var(--space-3);
  font-size: 12px;
  line-height: 1.6;
  color: var(--ui-text-muted);
}

@media (max-width: 640px) {
  .xg-checker { padding: var(--space-5); }
  .xg-checker__plan { font-size: 26px; }
}
</style>
