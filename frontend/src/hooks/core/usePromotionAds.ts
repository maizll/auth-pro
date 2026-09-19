import { onMounted, ref } from 'vue'
import { fetchAdvertisements, normalizeAdPlaceholder } from '@/api/advertisement'
import type { AdPosition, AdvertisementItem, AdvertisementPlaceholder } from '@/api/advertisement'
import type { PromotionItem, PromotionPage } from '@/api/promotion'

/** 跑马灯滚动所需的最少卡片数，与 ArtPromotionMarquee 的滚动阈值一致；不足时用招租占位补齐 */
const MARQUEE_SCROLL_MIN = 8

/** 占位卡片 id 必须唯一：展示板按 item.id 做 key，同页多个占位会撞 key */
let placeholderSeq = 0

/** 接口返回前与失败时用安全默认：默认文案、无跳转，不再写死外部 QQ 群 */
let currentPlaceholder: AdvertisementPlaceholder = normalizeAdPlaceholder()

const applyPlaceholder = (input?: Partial<AdvertisementPlaceholder> | null) => {
  currentPlaceholder = normalizeAdPlaceholder(input)
}

/** 广告位招租占位：无投放或加载失败时展示；跳转由后台配置，空链接不可点击 */
const promotionAdPlaceholder = (): PromotionItem => ({
  id: `ad-placeholder-${++placeholderSeq}`,
  title: currentPlaceholder.title,
  summary: currentPlaceholder.description,
  tag: '招租',
  linkUrl: currentPlaceholder.linkUrl,
  imageUrl: ''
})

/** 广告记录 → 投放项；tag 接口没有，统一标为「推荐」 */
const mapAdToPromotion = (ad: AdvertisementItem): PromotionItem => ({
  id: ad.id,
  title: ad.title,
  summary: ad.description,
  tag: '推荐',
  linkUrl: ad.destinationUrl,
  imageUrl: ad.imageUrl
})

/** 拉取广告记录并映射；失败或异常一律当无投放处理，广告不是业务功能，不把错误抛到页面 */
const loadPromotionItems = async (position: AdPosition): Promise<PromotionItem[]> => {
  try {
    const result = await fetchAdvertisements(position)
    applyPlaceholder(result?.placeholder)
    return (result?.records ?? []).map(mapAdToPromotion)
  } catch {
    applyPlaceholder()
    return []
  }
}

/**
 * 跑马灯用：广告记录映射成 PromotionItem 列表。
 * 不足 MARQUEE_SCROLL_MIN 条时用招租占位补齐；失败时整排占位。
 */
export function usePromotionAds(position: AdPosition) {
  // 初始即铺满招租占位，避免接口返回前跑马灯空一闪
  const items = ref<PromotionItem[]>(
    Array.from({ length: MARQUEE_SCROLL_MIN }, promotionAdPlaceholder)
  )

  const load = async () => {
    const mapped = await loadPromotionItems(position)
    while (mapped.length < MARQUEE_SCROLL_MIN) {
      mapped.push(promotionAdPlaceholder())
    }
    items.value = mapped
  }

  onMounted(load)

  return { items, load }
}

/**
 * 九宫格展示板用：按 perPage 条一页切分，每页不足用招租占位补齐一整页。
 * 恰好 perPage 条时只有一页，展示板不会轮播；超过后多页自动轮播。
 * 无投放或失败时返回一整页占位。
 */
export function usePromotionAdPages(position: AdPosition, perPage: number) {
  const buildPages = (mapped: PromotionItem[]): PromotionPage[] => {
    const pageCount = Math.max(1, Math.ceil(mapped.length / perPage))
    return Array.from({ length: pageCount }, (_, pageIndex) => {
      const pageItems = mapped.slice(pageIndex * perPage, (pageIndex + 1) * perPage)
      while (pageItems.length < perPage) {
        pageItems.push(promotionAdPlaceholder())
      }
      return { id: `ads-${position}-${pageIndex}`, layout: 'grid' as const, items: pageItems }
    })
  }

  // 初始即给一整页招租占位，避免接口返回前展示板空一闪
  const pages = ref<PromotionPage[]>(buildPages([]))

  const load = async () => {
    pages.value = buildPages(await loadPromotionItems(position))
  }

  onMounted(load)

  return { pages, load }
}
