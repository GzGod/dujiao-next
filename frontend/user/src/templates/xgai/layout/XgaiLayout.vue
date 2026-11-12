<template>
  <div class="xgai-scope">
    <header class="xh">
      <div class="x-container xh__inner">
        <RouterLink to="/" class="xh__logo" :aria-label="brandName">
          <img v-if="brandLogo" :src="brandLogo" :alt="brandName" class="xh__logo-img" />
          <template v-else>
            <span class="xh__mark" aria-hidden="true"><i /><i /><i /></span>
            <span>{{ brandName }}</span>
          </template>
        </RouterLink>

        <nav class="xh__nav" :class="{ 'is-open': menuOpen }" :aria-label="t('xgai.nav.label')">
          <RouterLink v-for="item in navItems" :key="item.key" :to="item.to" :class="{ 'is-current': item.match(route.path) }" @click="menuOpen = false">{{ item.label }}</RouterLink>
          <template v-for="item in customNav" :key="item.key">
            <RouterLink v-if="item.type === 'route'" :to="item.path" @click="menuOpen = false">{{ item.label }}</RouterLink>
            <a v-else :href="item.path" :target="item.target" rel="noopener noreferrer" @click="menuOpen = false">{{ item.label }}</a>
          </template>
          <div class="xh__nav-mobile">
            <RouterLink :to="accountPath" @click="menuOpen = false">{{ accountLabel }}</RouterLink>
            <button v-if="userAuthStore.isAuthenticated" type="button" @click="logout">{{ t('navbar.logout') }}</button>
            <div class="xh__langs">
              <button v-for="lang in languages" :key="lang.code" type="button" :class="{ 'is-active': appStore.locale === lang.code }" @click="changeLanguage(lang.code)">{{ lang.name }}</button>
            </div>
          </div>
        </nav>

        <div class="xh__actions">
          <div ref="langEl" class="xh__lang">
            <button type="button" class="xh__icon" :aria-label="t('navbar.selectLanguage')" @click="langOpen = !langOpen">
              <Languages />
            </button>
            <div v-if="langOpen" class="xh__menu">
              <button v-for="lang in languages" :key="lang.code" type="button" :class="{ 'is-active': appStore.locale === lang.code }" @click="changeLanguage(lang.code)">{{ lang.name }}</button>
            </div>
          </div>
          <RouterLink to="/cart" class="xh__icon xh__cart" :aria-label="t('navbar.cart')">
            <ShoppingBag />
            <span v-if="cartCount > 0">{{ cartCount }}</span>
          </RouterLink>
          <RouterLink :to="ordersPath" class="xh__text">{{ t('xgai.nav.orders') }}</RouterLink>
          <RouterLink :to="accountPath" class="xh__text">{{ accountLabel }}</RouterLink>
          <RouterLink to="/products" class="x-btn x-btn--primary xh__buy">
            {{ t('xgai.nav.buy') }} <ArrowRight />
          </RouterLink>
          <button type="button" class="xh__burger" :class="{ 'is-open': menuOpen }" :aria-expanded="menuOpen" :aria-label="t('navbar.more')" @click="menuOpen = !menuOpen">
            <i /><i />
          </button>
        </div>
      </div>
    </header>

    <main class="xgai-main">
      <slot />
    </main>

    <footer class="xf">
      <div class="x-container xf__main">
        <div class="xf__brand">
          <RouterLink to="/" class="xh__logo">
            <img v-if="brandLogo" :src="brandLogo" :alt="brandName" class="xh__logo-img" />
            <template v-else>
              <span class="xh__mark" aria-hidden="true"><i /><i /><i /></span>
              <span>{{ brandName }}</span>
            </template>
          </RouterLink>
          <p>{{ brandDescription || t('xgai.footer.tagline') }}</p>
        </div>
        <div class="xf__col">
          <b>{{ t('xgai.footer.shop') }}</b>
          <RouterLink to="/products">{{ t('xgai.footer.allProducts') }}</RouterLink>
          <RouterLink :to="{ path: '/', hash: '#checker' }">{{ t('xgai.footer.checker') }}</RouterLink>
          <RouterLink :to="ordersPath">{{ t('xgai.nav.orders') }}</RouterLink>
        </div>
        <div class="xf__col">
          <b>{{ t('xgai.footer.help') }}</b>
          <RouterLink :to="{ path: '/', hash: '#process' }">{{ t('xgai.footer.process') }}</RouterLink>
          <RouterLink v-if="noticeEnabled" to="/notice">{{ t('nav.notice') }}</RouterLink>
          <RouterLink v-if="aboutEnabled" to="/about">{{ t('nav.about') }}</RouterLink>
          <a v-if="contact?.telegram" :href="contact.telegram" target="_blank" rel="noopener noreferrer">Telegram</a>
          <a v-if="contact?.email" :href="`mailto:${contact.email}`">{{ t('footer.email') }}</a>
          <a v-if="contact?.support_url" :href="contact.support_url" target="_blank" rel="noopener noreferrer">{{ t('footer.supportUrl') }}</a>
        </div>
        <div class="xf__col">
          <b>{{ t('xgai.footer.partner') }}</b>
          <RouterLink to="/me/api">{{ t('xgai.footer.dealer') }}</RouterLink>
          <RouterLink to="/api-docs">{{ t('xgai.footer.apiDocs') }}</RouterLink>
        </div>
      </div>
      <div class="x-container xf__bottom">
        <span>© {{ year }} {{ brandName }}</span>
        <span class="xf__legal">
          <RouterLink to="/terms">{{ t('footer.terms') }}</RouterLink>
          <RouterLink to="/privacy">{{ t('footer.privacy') }}</RouterLink>
          <a v-for="link in footerLinks" :key="link.name" :href="link.url || undefined" target="_blank" rel="noopener noreferrer">{{ link.name }}</a>
        </span>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { ArrowRight, Languages, ShoppingBag } from 'lucide-vue-next'
import { useAppStore } from '../../../stores/app'
import { useCartStore } from '../../../stores/cart'
import { useUserAuthStore } from '../../../stores/userAuth'
import { useNavConfig } from '../../../composables/useNavConfig'
import { getImageUrl } from '../../../utils/image'
import { getLocalizedText } from '../../../utils/resellerSiteConfig'
import '../styles/xgai.css'
import '../styles/xgai-layout.css'
// 卡片 / 输入框 / 提示条（.xck__*）各页共用，随外壳一起加载，避免直达非结算页时缺样式
import '../styles/xgai-checkout.css'

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()
const cartStore = useCartStore()
const userAuthStore = useUserAuthStore()
const { noticeEnabled, aboutEnabled, customNavItems } = useNavConfig()

const menuOpen = ref(false)
const langOpen = ref(false)
const langEl = ref<HTMLElement | null>(null)
const year = new Date().getFullYear()

const brandName = computed(() => String(appStore.config?.brand?.site_name || '').trim() || '雪糕 AI')
// 主站默认使用内置的横版锁定图（public/brand）；分销站点位没有自带 Logo 时保持文字兜底，
// 避免把主站品牌图带到分销商子站。站点配置里的 site_logo 始终优先。
const DEFAULT_BRAND_LOGO = '/brand/xuegaoai-horizontal.svg'
const brandLogo = computed(() => {
  const raw = String(appStore.config?.brand?.site_logo || '').trim()
  if (raw) return getImageUrl(raw)
  return appStore.isResellerTenant ? '' : DEFAULT_BRAND_LOGO
})
const brandDescription = computed(() => getLocalizedText(appStore.config?.brand?.site_description as Record<string, string>, appStore.locale))
const contact = computed(() => appStore.config?.contact as { telegram?: string; email?: string; support_url?: string } | undefined)
const footerLinks = computed(() => {
  const links = appStore.config?.footer_links
  if (!Array.isArray(links)) return []
  return links
    .map((item: { name?: unknown; url?: unknown }) => ({
      name: typeof item?.name === 'string' ? item.name.trim() : getLocalizedText(item?.name as Record<string, string>, appStore.locale),
      url: String(item?.url || '').trim(),
    }))
    .filter((item) => item.name)
})

const cartCount = computed(() => cartStore.totalItems)
const ordersPath = computed(() => (userAuthStore.isAuthenticated ? '/me/orders' : '/guest/orders'))
const accountPath = computed(() => (userAuthStore.isAuthenticated ? '/me' : '/auth/login'))
const accountLabel = computed(() => (userAuthStore.isAuthenticated ? t('navbar.personalCenter') : t('navbar.login')))

// 首页锚点（检测 / 流程）不算「当前页」，只给真实页面路由做高亮，避免 router-link 默认按 / 前缀全部点亮
const navItems = computed(() => [
  { key: 'products', to: '/products', label: t('xgai.nav.products'), match: (p: string) => p.startsWith('/products') || p.startsWith('/categories') },
  { key: 'checker', to: { path: '/', hash: '#checker' }, label: t('xgai.nav.checker'), match: () => false },
  { key: 'process', to: { path: '/', hash: '#process' }, label: t('xgai.nav.process'), match: () => false },
  { key: 'partner', to: '/me/api', label: t('xgai.nav.partner'), match: (p: string) => p.startsWith('/me/api') },
])
const customNav = computed(() => customNavItems.value)

const languages = [
  { code: 'zh-CN', name: '简体中文' },
  { code: 'zh-TW', name: '繁體中文' },
  { code: 'en-US', name: 'English' },
]
const changeLanguage = (code: string) => {
  appStore.setLocale(code)
  langOpen.value = false
}
const logout = () => {
  userAuthStore.logout()
  menuOpen.value = false
}

watch(() => route.fullPath, () => { menuOpen.value = false })

const onDocClick = (e: MouseEvent) => {
  if (langOpen.value && langEl.value && !langEl.value.contains(e.target as Node)) langOpen.value = false
}
onMounted(() => {
  document.addEventListener('click', onDocClick)
  // Toast / 弹窗等 Teleport 到 body 的浮层靠这个 class 拿到 xgai 令牌
  document.body.classList.add('xgai-tokens')
})
onUnmounted(() => {
  document.removeEventListener('click', onDocClick)
  document.body.classList.remove('xgai-tokens')
})
</script>
