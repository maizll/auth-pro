import request from '@/utils/http'

export interface StoreAccount {
  bound: boolean
  account: string
  role: string
  licenseNo: string
  domain: string
  requestDomain: string
  domainMismatch: boolean
  edition: string
  editionExpireAt?: number | null
  permanent: boolean
  features: string[]
  verifiedAt: number
  graceUntil: number
  offlineGrace: boolean
  graceWarning: boolean
  explicitRevoked: boolean
  reason: string
  sourceBase: string
  siteUrl: string
  trustProxy: boolean
  installId: string
}

export interface StorePlan {
  id: number
  name: string
  period: string
  priceCents: number
}

export interface StoreCatalogItem {
  kind: string
  id: string
  name: string
  version: string
  priceCents: number
  ownership: 'free' | 'included' | 'purchased' | 'none' | string
}

export function fetchStoreAccount() {
  return request.get<StoreAccount>({ url: '/api/store/account', showErrorMessage: false })
}

export function saveStoreConnection(data: { sourceBase: string; siteUrl: string; trustProxy: boolean }) {
  return request.put({ url: '/api/store/settings', data, showSuccessMessage: true })
}

export function fetchStorePlans() {
  return request.get<{ list: StorePlan[] }>({ url: '/api/store/plans', showErrorMessage: false })
}

export function fetchStoreCatalog() {
  return request.get<{ list: StoreCatalogItem[]; edition: string }>({
    url: '/api/store/catalog',
    showErrorMessage: false
  })
}

export function bindStoreAccount(data: Record<string, unknown>) {
  return request.post<StoreAccount>({ url: '/api/store/bind', data })
}

export function createStoreEditionOrder(planId: number) {
  return request.post<{ orderNo: string; payUrl: string; amountCents: number; title: string }>({
    url: '/api/store/orders',
    data: { planId }
  })
}

export function fetchStoreEditionOrder(orderNo: string) {
  return request.get<{ orderNo: string; status: string }>({
    url: `/api/store/orders/${orderNo}`,
    showErrorMessage: false
  })
}

export function refreshStoreSnapshot() {
  return request.post<StoreAccount>({ url: '/api/store/refresh' })
}

export function installStoreItem(kind: string, id: string) {
  return request.post({ url: '/api/store/install', data: { kind, id } })
}
