<template>
  <div class="home-page min-h-screen bg-background text-foreground">

    <!-- ==================== LIST MODE ==================== -->
    <template v-if="templateMode === 'list'">
      <!-- Hero Banner (shared with card mode) -->
      <section v-if="showHeroSection" class="relative z-10 border-b pt-24 pb-10">
        <div class="container mx-auto px-4">
          <div class="relative overflow-hidden rounded-2xl border bg-card"
            @touchstart="onBannerTouchStart"
            @touchend="onBannerTouchEnd">
            <Transition name="banner-fade" mode="out-in">
              <img v-if="!bannerLoading && heroImage" :src="heroImage" :key="heroImage" class="absolute inset-0 h-full w-full object-cover" />
            </Transition>
            <div class="absolute inset-0 bg-black/50"></div>
            <div v-if="bannerLoading" class="relative flex min-h-[200px] flex-col justify-between p-5 sm:min-h-[240px] sm:p-6 md:min-h-[320px] md:p-10">
              <div class="space-y-3">
                <div class="h-5 w-24 theme-skeleton rounded-full" style="background: rgba(255,255,255,0.35)"></div>
                <div class="h-8 max-w-3xl theme-skeleton rounded-xl md:h-10" style="background: rgba(255,255,255,0.35)"></div>
                <div class="h-4 max-w-2xl theme-skeleton rounded-lg" style="background: rgba(255,255,255,0.3)"></div>
              </div>
            </div>
            <div v-else class="relative flex min-h-[200px] flex-col justify-between p-5 sm:min-h-[240px] sm:p-6 md:min-h-[320px] md:p-10">
              <div v-if="bannerCount > 1" class="mb-3 flex items-center justify-end gap-2">
                <button type="button"
                  class="inline-flex h-8 w-8 items-center justify-center rounded-full border border-white/30 bg-black/20 text-white transition hover:bg-black/35 md:h-9 md:w-9"
                  @click="handlePrevHeroBanner" :aria-label="t('common.previousBanner')">
                  <ChevronLeft class="h-3.5 w-3.5" />
                </button>
                <button type="button"
                  class="inline-flex h-8 w-8 items-center justify-center rounded-full border border-white/30 bg-black/20 text-white transition hover:bg-black/35 md:h-9 md:w-9"
                  @click="handleNextHeroBanner" :aria-label="t('common.nextBanner')">
                  <ChevronRight class="h-3.5 w-3.5" />
                </button>
              </div>
              <div class="space-y-2 sm:space-y-3">
                <span :class="heroBadgeClass">
                  <span class="h-2 w-2 rounded-full bg-emerald-300"></span>
                  {{ heroBadge }}
                </span>
                <h1 class="max-w-4xl text-xl font-semibold tracking-[-0.02em] text-white sm:text-2xl md:text-3xl">
                  {{ heroTitle }}
                </h1>
                <p class="max-w-3xl text-xs leading-relaxed text-gray-100 sm:text-sm">
                  {{ heroSubtitle }}
                </p>
              </div>
              <div v-if="bannerCount > 1" class="mt-4 flex items-center gap-2">
                <button v-for="(_, bIdx) in banners" :key="`list-dot-${bIdx}`" type="button"
                  class="h-2 rounded-full transition-all"
                  :class="bIdx === currentBannerIndex ? 'w-6 bg-white' : 'w-2 bg-white/45 hover:bg-white/70'"
                  @click="selectHeroBanner(bIdx)"></button>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- Main: Left Categories + Right Product List -->
      <section class="relative z-10 pb-6" :class="showHeroSection ? 'pt-6' : 'pt-24'">
        <div class="container mx-auto px-4">
          <div class="flex flex-col lg:flex-row gap-6">

            <CategorySidebar
              :categories="listCategoryGroups"
              :selected-category="listSelectedCategory"
              :expanded-parent-ids="listExpandedParentIds"
              :show-drawer="listShowFilterDrawer"
              compact
              @select-category="listSelectCategory"
              @toggle-parent="listToggleParentCategory"
              @update:show-drawer="listShowFilterDrawer = $event"
            />

            <!-- Right: Product List -->
            <main class="flex-1 min-w-0">
              <!-- Search Bar -->
              <div class="relative mb-4">
                <div class="absolute inset-y-0 left-3.5 flex items-center pointer-events-none">
                  <Search class="w-4 h-4 text-muted-foreground" />
                </div>
                <input
                  v-model="listSearchQuery"
                  type="text"
                  class="w-full h-10 pl-10 pr-10 rounded-xl border bg-card text-sm focus:outline-none focus:ring-2 focus:ring-primary/30 text-foreground placeholder:text-muted-foreground transition-shadow"
                  :placeholder="t('products.searchBoxPlaceholder')"
                  @keydown.enter="listOnSearch"
                />
                <button
                  v-if="listSearchQuery"
                  type="button"
                  class="absolute inset-y-0 right-3 flex items-center text-muted-foreground hover:text-foreground transition-colors"
                  @click="listClearSearch"
                >
                  <X class="w-4 h-4" />
                </button>
              </div>

              <!-- Loading Skeleton -->
              <div v-if="listLoading" class="space-y-6">
                <div v-for="i in 3" :key="i">
                  <div class="flex items-center gap-2 mb-3 px-0.5">
                    <div class="h-5 w-5 rounded theme-skeleton"></div>
                    <div class="h-4 w-28 rounded theme-skeleton"></div>
                  </div>
                  <div class="space-y-2">
                    <div v-for="j in 3" :key="j"
                      class="bg-card rounded-xl border flex items-center h-[72px]">
                      <div class="w-14 h-14 m-2 rounded-lg theme-skeleton flex-shrink-0"></div>
                      <div class="flex-1 px-3 py-2 space-y-2">
                        <div class="h-3.5 w-1/3 rounded theme-skeleton"></div>
                        <div class="h-3 w-1/4 rounded theme-skeleton"></div>
                      </div>
                      <div class="px-4 py-2">
                        <div class="h-4 w-14 rounded theme-skeleton"></div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>

              <!-- Grouped Product List -->
              <div v-else-if="listProductGroups.length > 0" class="space-y-6">
                <div v-for="group in listProductGroups" :key="group.categoryId ?? 'uncategorized'">
                  <!-- Category Header -->
                  <div class="flex items-center gap-2 mb-3 px-0.5">
                    <span class="w-1 h-5 rounded-full bg-primary flex-shrink-0"></span>
                    <img v-if="group.categoryIcon" :src="getImageUrl(group.categoryIcon)"
                      :alt="group.categoryName" loading="lazy" class="h-5 w-5 rounded object-cover flex-shrink-0" />
                    <span class="text-sm font-semibold text-foreground truncate">{{ group.categoryName }}</span>
                    <span class="text-xs text-muted-foreground">({{ group.products.length }})</span>
                  </div>
                  <!-- Products in this category -->
                  <div class="space-y-2">
                    <ProductListItem
                      v-for="(product, idx) in group.products"
                      :key="product.id"
                      :product="product"
                      :index="idx"
                      :animation-step="20"
                      @click="goToProduct"
                      @quick-buy="openQuickBuy"
                    />
                  </div>
                </div>

                <PaginationNav
                  :current-page="listCurrentPage"
                  :total-pages="listTotalPages"
                  :loading="listLoading"
                  compact
                  @change-page="listChangePage"
                />
              </div>

              <!-- Empty State -->
              <EmptyState v-else variant="soft" icon="package" :title="t('products.empty')" />
            </main>
          </div>
        </div>
      </section>
    </template>

    <!-- ==================== CARD MODE (default)：雪糕AI 首页 ==================== -->
    <template v-else>
      <!-- Hero -->
      <section class="xg-hero">
        <div class="xg-container xg-hero__grid">
          <div class="xg-hero__copy">
            <h1 class="xg-hero__title">{{ t('xg.hero.title') }}</h1>
            <p class="xg-hero__lede">{{ t('xg.hero.lede') }}</p>
            <div class="xg-hero__actions">
              <a href="#shop" class="xg-btn xg-btn--primary xg-btn--lg">{{ t('xg.hero.primary') }}</a>
              <a href="#how" class="xg-btn xg-btn--ghost xg-btn--lg">{{ t('xg.hero.secondary') }}</a>
            </div>
            <ul class="xg-hero__facts">
              <li><strong>{{ products.length || '—' }}</strong><span>{{ t('xg.hero.factProducts') }}</span></li>
              <li><strong>{{ t('xg.hero.factSpeedValue') }}</strong><span>{{ t('xg.hero.factSpeed') }}</span></li>
              <li><strong>{{ t('xg.hero.factOwnValue') }}</strong><span>{{ t('xg.hero.factOwn') }}</span></li>
            </ul>
          </div>

          <!-- 右侧：一张真实的「订单凭证」，展示下单后用户实际拿到的东西 -->
          <div class="xg-receipt" aria-hidden="true">
            <div class="xg-receipt__row xg-receipt__row--head">
              <span>{{ t('xg.receipt.title') }}</span>
              <span class="xg-receipt__status">{{ t('xg.receipt.delivered') }}</span>
            </div>
            <div class="xg-receipt__product">{{ heroProductTitle }}</div>
            <div class="xg-receipt__row"><span>{{ t('xg.receipt.account') }}</span><span>you@example.com</span></div>
            <div class="xg-receipt__row"><span>{{ t('xg.receipt.paid') }}</span><span class="xg-receipt__price">{{ heroProductPrice }}</span></div>
            <div class="xg-receipt__code">
              <span>{{ t('xg.receipt.code') }}</span>
              <code>XG-7K2Q-9M4D-PLUS</code>
            </div>
            <p class="xg-receipt__note">{{ t('xg.receipt.note') }}</p>
          </div>
        </div>
      </section>

      <!-- 信任条 -->
      <section class="xg-trust" aria-label="trust">
        <div class="xg-container xg-trust__row">
          <span v-for="item in trustItems" :key="item">{{ item }}</span>
        </div>
      </section>

      <!-- 商品中心 -->
      <section id="shop" class="xg-section">
        <div class="xg-container">
          <div class="xg-section__head">
            <div>
              <h2 class="xg-h2">{{ t('xg.shop.title') }}</h2>
              <p class="xg-section__lede">{{ t('xg.shop.lede') }}</p>
            </div>
            <router-link to="/products" class="xg-link">{{ t('xg.shop.all') }}</router-link>
          </div>

          <div class="xg-chips" role="tablist">
            <button type="button" role="tab" class="xg-chip" :class="{ 'xg-chip--active': activeCategory === 0 }"
              :aria-selected="activeCategory === 0" @click="activeCategory = 0">{{ t('xg.shop.allTab') }}</button>
            <button v-for="cat in shopCategories" :key="cat.id" type="button" role="tab" class="xg-chip"
              :class="{ 'xg-chip--active': activeCategory === cat.id }" :aria-selected="activeCategory === cat.id"
              @click="activeCategory = cat.id">{{ cat.label }}</button>
          </div>

          <div v-if="productsLoading" class="xg-grid">
            <div v-for="n in 6" :key="n" class="xg-card xg-card--skeleton"></div>
          </div>
          <div v-else-if="filteredProducts.length" class="xg-grid">
            <XgProductCard v-for="product in filteredProducts" :key="product.id" :product="product"
              @open="goToProduct" @buy="openQuickBuy" />
          </div>
          <div v-else class="xg-empty">
            <PackageOpen class="h-10 w-10" :stroke-width="1.5" />
            {{ t('home.featured.empty') }}
          </div>
        </div>
      </section>

      <!-- 怎么购买 -->
      <section id="how" class="xg-section xg-section--tint">
        <div class="xg-container">
          <h2 class="xg-h2">{{ t('xg.how.title') }}</h2>
          <p class="xg-section__lede">{{ t('xg.how.lede') }}</p>
          <ol class="xg-steps">
            <li v-for="(step, i) in howSteps" :key="step.title" class="xg-step">
              <span class="xg-step__num">{{ i + 1 }}</span>
              <h3 class="xg-step__title">{{ step.title }}</h3>
              <p class="xg-step__text">{{ step.text }}</p>
            </li>
          </ol>
        </div>
      </section>

      <!-- 代理等级 -->
      <section id="partner" class="xg-section">
        <div class="xg-container">
          <div class="xg-section__head">
            <div>
              <h2 class="xg-h2">{{ t('xg.partner.title') }}</h2>
              <p class="xg-section__lede">{{ t('xg.partner.lede') }}</p>
            </div>
            <router-link to="/api-docs" class="xg-link">{{ t('xg.partner.docs') }}</router-link>
          </div>
          <div class="xg-tiers">
            <XgTierCard level="free" :name="t('xg.partner.free.name')" :rate="t('xg.partner.free.rate')" :rate-unit="t('xg.partner.free.unit')"
              :rule="t('xg.partner.free.rule')" :perks="[t('xg.partner.free.p1'), t('xg.partner.free.p2')]" />
            <XgTierCard level="basic" :name="t('xg.partner.basic.name')" rate="97" :rate-unit="t('xg.partner.percentOff')"
              :rule="t('xg.partner.basic.rule')" :perks="[t('xg.partner.perkApi'), t('xg.partner.perkBalance')]" />
            <XgTierCard level="pro" :name="t('xg.partner.pro.name')" rate="95" :rate-unit="t('xg.partner.percentOff')"
              :rule="t('xg.partner.pro.rule')" :perks="[t('xg.partner.perkApi'), t('xg.partner.perkAuto')]"
              featured :flag="t('xg.partner.popular')" />
            <XgTierCard level="elite" :name="t('xg.partner.elite.name')" rate="91.9" :rate-unit="t('xg.partner.percentOff')"
              :rule="t('xg.partner.elite.rule')" :perks="[t('xg.partner.perkApi'), t('xg.partner.perkBest')]" />
          </div>
          <p class="xg-note">{{ t('xg.partner.note') }}</p>
        </div>
      </section>
    </template>

    <ProductQuickBuy
      v-if="quickBuyProduct"
      :product="quickBuyProduct"
      :visible="quickBuyVisible"
      @update:visible="quickBuyVisible = $event"
    />

    <AnnouncementModal
      v-if="activeAnnouncement"
      :announcement="activeAnnouncement"
      :visible="announcementVisible"
      @update:visible="announcementVisible = $event"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ChevronLeft, ChevronRight, PackageOpen, Search, X } from 'lucide-vue-next'
import { productAPI } from '../api'
import { getImageUrl } from '../utils/image'
import { useLocalized } from '../composables/useProduct'
import { useBannerCarousel } from '../composables/useBannerCarousel'
import { useProductList } from '../composables/useProductList'
import { useProductListGroups } from '../composables/useProductListGroups'
import { usePageSeo } from '../composables/usePageSeo'
import { useAppStore } from '../stores/app'
import ProductListItem from '../components/ProductListItem.vue'
import ProductQuickBuy from '../components/ProductQuickBuy.vue'
import CategorySidebar from '../components/CategorySidebar.vue'
import PaginationNav from '../components/PaginationNav.vue'
import EmptyState from '../components/EmptyState.vue'
import AnnouncementModal from '../components/AnnouncementModal.vue'
import XgProductCard from '../components/xg/XgProductCard.vue'
import XgTierCard from '../components/xg/XgTierCard.vue'
import { useAnnouncement, type HomeAnnouncement } from '../composables/useAnnouncement'

const router = useRouter()
const { t } = useI18n()
const { getLocalizedText } = useLocalized()
const appStore = useAppStore()

// 英雄横幅上的玻璃态徽章(白字深底,跟随图片而非主题)
const heroBadgeClass = 'inline-flex items-center gap-2 rounded-md border border-white/25 bg-black/55 px-2.5 py-0.5 text-xs font-semibold uppercase tracking-wider text-white backdrop-blur-sm'

const templateMode = computed(() => appStore.config?.template_mode || 'card')

// ==================== Shared State ====================
const products = ref<any[]>([])
const quickBuyProduct = ref<any>(null)
const quickBuyVisible = ref(false)

const { shouldShow } = useAnnouncement()
const activeAnnouncement = ref<HomeAnnouncement | null>(null)
const announcementVisible = ref(false)

const openQuickBuy = (product: any) => {
  quickBuyProduct.value = product
  quickBuyVisible.value = true
}

// ==================== Banner Carousel ====================
const {
  banners,
  bannerLoading,
  currentBannerIndex,
  bannerCount,
  showHeroSection,
  heroImage,
  heroBadge,
  heroTitle,
  heroSubtitle,
  loadBanners,
  handleNextHeroBanner,
  handlePrevHeroBanner,
  selectHeroBanner,
  onBannerTouchStart,
  onBannerTouchEnd,
  stopHeroAutoPlay,
} = useBannerCarousel()

// ==================== List Mode ====================
const {
  loading: listLoading,
  products: listProducts,
  selectedCategory: listSelectedCategory,
  searchQuery: listSearchQuery,
  currentPage: listCurrentPage,
  totalPages: listTotalPages,
  showFilterDrawer: listShowFilterDrawer,
  expandedParentIds: listExpandedParentIds,
  categoryGroups: listCategoryGroups,
  categoryMap: listCategoryMap,
  selectCategory: listSelectCategory,
  toggleParentCategory: listToggleParentCategory,
  changePage: listChangePage,
  clearSearch: listClearSearch,
  onSearch: listOnSearch,
  initialize: listInitialize,
  cleanup: listCleanup,
} = useProductList({ pageSize: 20, homeRouteName: 'home' })

const listProductGroups = useProductListGroups(listProducts, listCategoryMap)

// ==================== SEO ====================
const route = useRoute()
const seoCategoryName = computed(() => {
  if (!listSelectedCategory.value) return ''
  const cat = listCategoryMap.value.get(listSelectedCategory.value)
  return cat ? getLocalizedText(cat.name) : ''
})
usePageSeo({
  canonicalPath: () => route.path,
  title: () => {
    if (route.name === 'category-products') {
      return seoCategoryName.value || t('nav.products')
    }
    if (route.name === 'products') return t('nav.products')
    return undefined
  },
})

// ==================== Card Mode：雪糕AI 首页 ====================
const goToProduct = (slug: string) => {
  router.push(`/products/${slug}`)
}

const productsLoading = ref(true)
const activeCategory = ref(0)

const loadFeaturedProducts = async () => {
  productsLoading.value = true
  try {
    const response = await productAPI.list({ page: 1, page_size: 24 })
    products.value = response.data.data || []
  } catch (error) {
    console.error('Failed to load products:', error)
  } finally {
    productsLoading.value = false
  }
}

// 分类标签只展示实际有商品的分类，不造假分类
const shopCategories = computed(() => {
  const seen = new Map<number, string>()
  for (const p of products.value) {
    const id = Number(p?.category_id || p?.category?.id || 0)
    if (!id || seen.has(id)) continue
    seen.set(id, getLocalizedText(p?.category?.name) || `#${id}`)
  }
  return [...seen.entries()].map(([id, label]) => ({ id, label }))
})

// 按价格从低到高排，入门款在前
const filteredProducts = computed(() => {
  const list = activeCategory.value
    ? products.value.filter((p) => Number(p?.category_id || p?.category?.id) === activeCategory.value)
    : products.value
  return [...list].sort((a, b) => Number(a?.price_amount || 0) - Number(b?.price_amount || 0))
})

// Hero 右侧凭证取一件真实商品（优先 GPT Plus），价格跟后台同步
const heroProduct = computed(() => {
  const list = products.value
  const title = (p: any) => getLocalizedText(p?.title)
  return list.find((p) => /gpt\s*plus/i.test(title(p)))
    || list.find((p) => /plus/i.test(title(p)))
    || list[0]
    || null
})
const heroProductTitle = computed(() => getLocalizedText(heroProduct.value?.title) || 'ChatGPT Plus')
const heroProductPrice = computed(() => {
  const amount = heroProduct.value?.price_amount
  return amount ? `¥${Number(amount).toFixed(2)}` : '—'
})

const trustItems = computed(() => [
  t('xg.trust.auto'),
  t('xg.trust.track'),
  t('xg.trust.support'),
  t('xg.trust.own'),
])

const howSteps = computed(() => [
  { title: t('xg.how.s1t'), text: t('xg.how.s1') },
  { title: t('xg.how.s2t'), text: t('xg.how.s2') },
  { title: t('xg.how.s3t'), text: t('xg.how.s3') },
])

// ==================== Lifecycle ====================
const showAnnouncementIfNeeded = () => {
  const announcement = appStore.config?.announcement as HomeAnnouncement | undefined
  if (announcement && shouldShow(announcement)) {
    activeAnnouncement.value = announcement
    announcementVisible.value = true
  }
}

onMounted(async () => {
  await appStore.loadConfig()
  if (templateMode.value === 'list') {
    await Promise.all([loadBanners(), listInitialize()])
  } else {
    await loadFeaturedProducts()
  }
  showAnnouncementIfNeeded()
})

onUnmounted(() => {
  stopHeroAutoPlay()
  listCleanup()
})
</script>

<style scoped>
.banner-fade-enter-active,
.banner-fade-leave-active {
  transition: opacity 300ms ease;
}
.banner-fade-enter-from,
.banner-fade-leave-to {
  opacity: 0;
}
</style>
