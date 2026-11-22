import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '../stores/app'
import { usePageSeo } from './usePageSeo'
import { siteBuyingContent } from '../utils/productGuide'

/**
 * 关于页共享逻辑（classic + vault 双模板共用）。
 * 完整保留原 views/About.vue 的行为，仅抽离为 composable。
 */
export function useAbout() {
  const { t, locale } = useI18n()
  const appStore = useAppStore()

  usePageSeo({
    title: () => t('nav.about'),
    canonicalPath: () => '/about',
  })

  const aboutConfig = computed(() => appStore.config?.about || null)
  const useMainFallback = computed(() => !appStore.isResellerTenant && appStore.config?.brand?.site_url?.replace(/\/$/, '') === 'https://xuegaogx.cc')
  const fallback = computed(() => useMainFallback.value ? siteBuyingContent(String(locale.value)) : null)
  const contactConfig = computed(() => {
    const contact = appStore.config?.contact || {}
    return useMainFallback.value ? { ...contact, telegram: contact.telegram || 'https://t.me/xuegaozhanshen' } : contact
  })

  const resolveLocalizedText = (raw: unknown): string => {
    if (!raw || typeof raw !== 'object') {
      return ''
    }

    const record = raw as Record<string, unknown>
    const lang = String(locale.value || appStore.locale || 'zh-CN')
    const candidates = [record[lang], record['zh-CN'], record['zh-TW'], record['en-US']]

    for (const candidate of candidates) {
      if (typeof candidate === 'string' && candidate.trim() !== '') {
        return candidate.trim()
      }
    }

    return ''
  }

  const heroTitle = computed(() => resolveLocalizedText(aboutConfig.value?.hero?.title) || (useMainFallback.value ? String(appStore.config?.brand?.site_name || '') : ''))
  const heroSubtitle = computed(() => resolveLocalizedText(aboutConfig.value?.hero?.subtitle) || fallback.value?.subtitle || '')
  const introductionText = computed(() => resolveLocalizedText(aboutConfig.value?.introduction) || fallback.value?.intro || '')
  const servicesTitle = computed(() => resolveLocalizedText(aboutConfig.value?.services?.title) || fallback.value?.servicesTitle || '')
  const contactTitle = computed(() => resolveLocalizedText(aboutConfig.value?.contact?.title) || fallback.value?.contactTitle || '')
  const contactText = computed(() => resolveLocalizedText(aboutConfig.value?.contact?.text) || fallback.value?.contactText || '')

  const serviceItems = computed(() => {
    const raw = aboutConfig.value?.services?.items
    if (!Array.isArray(raw) || raw.length === 0) {
      return fallback.value?.services || []
    }

    return raw
      .map((item) => resolveLocalizedText(item))
      .filter((item) => item !== '')
  })

  const hasIntroduction = computed(() => introductionText.value !== '')
  const hasServices = computed(() => servicesTitle.value !== '' || serviceItems.value.length > 0)
  const hasContactLinks = computed(() => !!(contactConfig.value?.telegram || contactConfig.value?.whatsapp || contactConfig.value?.email || contactConfig.value?.support_url))
  const hasContact = computed(() => contactTitle.value !== '' || contactText.value !== '' || hasContactLinks.value)

  onMounted(async () => {
    if (!appStore.config) {
      await appStore.loadConfig()
    }
  })

  return {
    contactConfig,
    heroTitle,
    heroSubtitle,
    introductionText,
    servicesTitle,
    contactTitle,
    contactText,
    serviceItems,
    hasIntroduction,
    hasServices,
    hasContactLinks,
    hasContact,
  }
}
