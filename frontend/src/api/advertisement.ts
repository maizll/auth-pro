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

interface AdvertisementPlaceholder {
  title: string
  description: string
  linkUrl: string
}

export const DEFAULT_AD_PLACEHOLDER: AdvertisementPlaceholder = {
  title: '广告位出租',
  description: '虚位以待，欢迎联系投放',
  linkUrl: ''
}

export const AD_NO_REFUND_NOTICE =
  '虚拟产品，付款后不退款。审核不过请修改后重新提交；未展示天数顺延；违规下线剩余天数作废。'

export function fetchAdvertisements(position: AdPosition) {
  return request.get<{ records: AdvertisementItem[]; placeholder?: AdvertisementPlaceholder }>({
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
