import request from '@/utils/http'

/** 广告位：设计 11 槽 + 公开页横幅；popup 已停用 */
export type AdPosition =
  | 'home-banner'
  | 'sidebar'
  | 'console-home'
  | 'console-sidebar'
  | 'console-login'
  | 'console-topbar'
  | 'console-rail'
  | 'store-native'
  | 'update-done'
  | 'list-footer'
  | 'profile-side'
  | 'docs-side'
  | 'lock-screen'

export interface AdvertisementItem {
  id: string
  title: string
  imageUrl: string
  destinationUrl: string
  position: string
  positions?: string[]
  weight: number
  startAt: string
  endAt: string
  description: string
  isPlaceholder?: boolean
}

export interface AdvertisementPlaceholder {
  title: string
  description: string
  linkUrl: string
}

export const DEFAULT_AD_PLACEHOLDER: AdvertisementPlaceholder = {
  title: '广告位招租',
  description: '在这里推广你的网站',
  linkUrl: ''
}

export const AD_NO_REFUND_NOTICE =
  '虚拟产品，付款后不退款。审核不过请修改后重新提交；未展示天数顺延；违规下线剩余天数作废。'

/** 卡片类位置：空位可显示招租（设计 ①②③⑤⑨⑩）；文字类等空位直接塌掉 */
export const AD_RECRUIT_POSITIONS: ReadonlySet<AdPosition> = new Set([
  'console-home',
  'console-sidebar',
  'sidebar',
  'console-login',
  'console-rail',
  'profile-side',
  'docs-side'
])

/** 与后端 adSlotCatalog 日价一致，招租卡「¥xx/天起」用；登录页等未鉴权场景不调 slots 接口 */
export const AD_SLOT_PRICE_YUAN: Partial<Record<AdPosition, number>> = {
  'console-home': 30,
  'console-sidebar': 20,
  sidebar: 20,
  'console-login': 15,
  'console-rail': 18,
  'profile-side': 5,
  'docs-side': 8
}

export function isAdRecruitPosition(position: AdPosition | string): boolean {
  return AD_RECRUIT_POSITIONS.has(position as AdPosition)
}

export function fetchAdvertisements(position: AdPosition) {
  return request.get<{
    records: AdvertisementItem[]
    placeholder?: AdvertisementPlaceholder
    enabled?: boolean
  }>({
    url: '/api/advertisements',
    params: { position },
    showErrorMessage: false
  })
}

export function fetchAdSlotCatalog() {
  return request.get<{
    list: Array<{
      id: string
      name: string
      description: string
      priceCents: number
      capacity: number
      enabled: boolean
      leftHint: number
    }>
    noRefundNotice: string
  }>({ url: '/api/ads/slots' })
}

export function fetchAdCalendar(slotId: string, month: string) {
  return request.get<{
    days: Array<{ date: string; left: number; full: boolean; priceCents: number }>
    capacity: number
  }>({ url: '/api/ads/calendar', params: { slotId, month } })
}

export function fetchAdPayOptions() {
  return request.get<{ list: Array<{ code: string; label: string }>; noRefundNotice: string }>({
    url: '/api/ads/pay-options'
  })
}

export function createAdOrder(data: Record<string, unknown>) {
  return request.post({ url: '/api/ads/orders', data })
}

export function payAdOrder(data: Record<string, unknown>) {
  return request.post({ url: '/api/ads/orders/pay', data })
}

export function fetchMyAdOrders() {
  return request.get<{ list: Array<Record<string, unknown>>; noRefundNotice: string }>({
    url: '/api/ads/orders'
  })
}

export function resubmitAdOrder(data: Record<string, unknown>) {
  return request.post({ url: '/api/ads/orders/resubmit', data })
}

export function fetchAdsHide() {
  return request.get<{
    hidden: boolean
    switchOn: boolean
    commercial: boolean
    effective: boolean
  }>({ url: '/api/ads/hide' })
}

export function setAdsHide(hidden: boolean) {
  return request.put({ url: '/api/ads/hide', data: { hidden } })
}
