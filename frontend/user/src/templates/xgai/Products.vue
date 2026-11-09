<template>
  <div class="xprod-page">
    <nav class="x-container xpd__crumb xck__crumb" :aria-label="t('xgai.detail.breadcrumb')">
      <RouterLink to="/">{{ t('nav.home') }}</RouterLink>
      <span>/</span>
      <b>{{ pageTitle }}</b>
    </nav>

    <div class="x-container">
      <h1 class="xprod__title">{{ pageTitle }}</h1>

      <div class="xprod__layout">
        <!-- 侧栏：搜索 + 分类 -->
        <aside class="xprod__side">
          <div class="xprod__search">
            <Search />
            <input
              v-model="searchQuery"
              class="xck__input"
              :placeholder="t('products.searchBoxPlaceholder')"
              :aria-label="t('products.searchLabel')"
            />
            <button v-if="searchQuery" type="button" class="xprod__search-clear" :aria-label="t('blog.searchClear')" @click="clearSearch"><X /></button>
          </div>

          <div class="xck__card xprod__cats">
            <h2>{{ t('products.categories') }}</h2>
            <div class="xprod__cat-list">
              <div class="xprod__cat-row">
                <button type="button" class="xprod__cat" :class="{ 'is-active': selectedCategory === null }" @click="selectCategory(null)">
                  <span>{{ t('products.allCategories') }}</span>
                </button>
              </div>
              <template v-for="grp in categoryGroups" :key="grp.id">
                <div class="xprod__cat-row">
                  <button type="button" class="xprod__cat" :class="{ 'is-active': selectedCategory === grp.id }" @click="selectCategory(grp.id)">
                    <img v-if="grp.icon" :src="getImageUrl(grp.icon)" :alt="catName(grp)" loading="lazy" />
                    <span>{{ catName(grp) }}</span>
                  </button>
                  <button
                    v-if="grp.children.length"
                    type="button"
                    class="xprod__cat-toggle"
                    :class="{ 'is-open': expandedParentIds.includes(grp.id) }"
                    :aria-expanded="expandedParentIds.includes(grp.id)"
                    :aria-label="catName(grp)"
                    @click="toggleParentCategory(grp.id)"
                  >
                    <ChevronDown />
                  </button>
                </div>
                <template v-if="grp.children.length && expandedParentIds.includes(grp.id)">
                  <button
                    v-for="child in grp.children"
                    :key="child.id"
                    type="button"
                    class="xprod__cat xprod__cat--child"
                    :class="{ 'is-active': selectedCategory === child.id }"
                    @click="selectCategory(child.id)"
                  >
                    <span>{{ catName(child) }}</span>
                  </button>
                </template>
              </template>
            </div>
          </div>
        </aside>

        <!-- 商品区 -->
        <section>
          <div v-if="loading" class="xprod__grid">
            <div v-for="i in 9" :key="i" class="xprod__skeleton" />
          </div>

          <template v-else-if="products.length">
            <div class="xprod__grid">
              <XgaiProductCard
                v-for="product in products"
                :key="product.id"
                :product="product"
                @open="goToProduct"
                @buy="openQuickBuy"
              />
            </div>

            <nav v-if="totalPages > 1" class="xprod__pager">
              <button type="button" :disabled="currentPage <= 1" @click="changePage(currentPage - 1)"><ChevronLeft /></button>
              <button
                v-for="p in pageWindow"
                :key="p"
                type="button"
                :class="{ 'is-active': p === currentPage }"
                @click="changePage(p)"
              >{{ p }}</button>
              <button type="button" :disabled="currentPage >= totalPages" @click="changePage(currentPage + 1)"><ChevronRight /></button>
            </nav>
          </template>

          <div v-else class="xck__empty">
            <component :is="searchQuery || selectedCategory ? SearchX : PackageOpen" />
            <p>{{ (searchQuery || selectedCategory) ? t('products.emptyFiltered') : t('products.empty') }}</p>
            <button v-if="searchQuery || selectedCategory" type="button" class="x-btn x-btn--ghost" @click="resetFilters">{{ t('products.clearFilters') }}</button>
          </div>
        </section>
      </div>
    </div>

    <ProductQuickBuy
      v-if="quickBuyProduct"
      :product="quickBuyProduct"
      :visible="quickBuyVisible"
      @update:visible="quickBuyVisible = $event"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ChevronDown, ChevronLeft, ChevronRight, PackageOpen, Search, SearchX, X } from 'lucide-vue-next'
import { useProductList } from '../../composables/useProductList'
import { usePageSeo } from '../../composables/usePageSeo'
import { useLocalized } from '../../composables/useProduct'
import { getImageUrl } from '../../utils/image'
import type { PublicCategory } from '../../utils/category'
import XgaiProductCard from './components/XgaiProductCard.vue'
import ProductQuickBuy from '../../components/ProductQuickBuy.vue'
import './styles/xgai-products.css'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const { getLocalizedText } = useLocalized()

const quickBuyProduct = ref<any>(null)
const quickBuyVisible = ref(false)
const openQuickBuy = (product: any) => {
  quickBuyProduct.value = product
  quickBuyVisible.value = true
}
const goToProduct = (slug: string) => {
  if (slug) router.push(`/products/${slug}`)
}

const {
  loading,
  products,
  selectedCategory,
  searchQuery,
  currentPage,
  totalPages,
  expandedParentIds,
  categoryGroups,
  categoryMap,
  selectCategory,
  toggleParentCategory,
  changePage,
  clearSearch,
  initialize,
  cleanup,
} = useProductList({ pageSize: 12, homeRouteName: 'products' })

const catName = (cat: PublicCategory) => getLocalizedText(cat.name) || cat.slug || ''

const selectedCategoryName = computed(() => {
  if (!selectedCategory.value) return ''
  const cat = categoryMap.value.get(selectedCategory.value)
  return cat ? catName(cat) : ''
})

const pageTitle = computed(() => {
  if (route.name === 'category-products') return selectedCategoryName.value || t('nav.products')
  return t('nav.products')
})

// 分页窗口：当前页前后各 2 页
const pageWindow = computed(() => {
  const total = totalPages.value
  const cur = currentPage.value
  const start = Math.max(1, cur - 2)
  const end = Math.min(total, start + 4)
  const realStart = Math.max(1, end - 4)
  const pages: number[] = []
  for (let p = realStart; p <= end; p++) pages.push(p)
  return pages
})

const resetFilters = () => {
  clearSearch()
  selectCategory(null)
}

usePageSeo({
  canonicalPath: () => route.path,
  title: () => pageTitle.value,
})

onMounted(() => { void initialize() })
onUnmounted(() => cleanup())
</script>
