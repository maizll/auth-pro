import request from '@/utils/http'

/** 广告位标识，一个 slot 对应一处投放位置，内容互不共享 */
export type PromotionSlot = 'dashboard' | 'plugin-store'

/** 单条投放内容；无 imageUrl 时用 accent 渲染渐变底 + 首字占位 */
export interface PromotionItem {
  id: string
  title: string
  summary: string
  tag: string
  linkUrl: string
  imageUrl?: string
  accent?: string
}

/** 轮播的一页；layout 由数据方决定，渲染层只做分发 */
export interface PromotionPage {
  id: string
  layout: 'grid' | 'banner'
  items: PromotionItem[]
}

export function fetchPromotions(slot: PromotionSlot) {
  return request.get<PromotionPage[]>({
    url: '/api/promotions',
    params: { slot }
  })
}
