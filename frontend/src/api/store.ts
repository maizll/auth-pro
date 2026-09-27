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
  connectionIssues?: { field: string; message: string }[]
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
  billing?: string
  purchaseOnly?: boolean
  ownership: 'free' | 'included' | 'purchased' | 'none' | string
}

export function fetchStoreAccount() {
  return request.get<StoreAccount>({ url: '/api/store/account', showErrorMessage: false })
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

export function createStoreItemOrder(itemKind: 'plugin' | 'template', itemId: string) {
  return request.post<{ orderNo: string; payUrl: string; amountCents: number; title: string }>({
    url: '/api/store/orders',
    data: { itemKind, itemId }
  })
}

export function fetchStoreEditionOrder(orderNo: string) {
  return request.get<{ orderNo: string; status: string }>({
    url: `/api/store/orders/${orderNo}`,
    showErrorMessage: false
  })
}

export function logoutStoreAccount() {
  return request.post({ url: '/api/store/logout', showErrorMessage: true, showSuccessMessage: false })
}

export function refreshStoreSnapshot() {
  return request.post<StoreAccount>({ url: '/api/store/refresh' })
}
