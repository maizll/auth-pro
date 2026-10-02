// 买家站商业版接口：账号、绑定、套餐、下单、查单和安装。
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
  /** 商业版来源，如购买、后台开通。不在签名快照里，旧客户端验签不受影响。 */
  editionSource?: string
  permanent: boolean
  features: string[]
  verifiedAt: number
  graceUntil: number
  offlineGrace: boolean
  graceWarning: boolean
  explicitRevoked: boolean
  /** 源站核对后确认本地旧绑定已不能用。 */
  bindingInvalid?: boolean
  bindingInvalidReason?: string
  /** 本次读取已向源站确认绑定仍然有效。 */
  sourceVerified?: boolean
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

export interface StorePayOption {
  code: string
  label: string
  icon?: string
  color?: string
  payType?: string
}

export function fetchStoreAccount(verify = false) {
  return request.get<StoreAccount>({
    url: '/api/store/account',
    params: verify ? { verify: 1 } : undefined,
    showErrorMessage: false
  })
}

/** 套餐列表。product 是本站商业版所属的应用；暂时不能购买时 notice 说明原因。 */
export function fetchStorePlans() {
  return request.get<{
    list: StorePlan[]
    payOptions?: StorePayOption[]
    product?: { name: string }
    notice?: string
  }>({
    url: '/api/store/plans',
    showErrorMessage: false
  })
}

export function bindStoreAccount(data: Record<string, unknown>) {
  return request.post<StoreAccount>({ url: '/api/store/bind', data })
}

export function fetchStoreRegisterCaptcha() {
  return request.get<{ enabled: boolean; captchaId: string }>({
    url: '/api/store/register/captcha',
    showErrorMessage: false
  })
}

export function sendStoreRegisterEmailCode(data: Record<string, unknown>) {
  return request.post<{ expiresIn?: number }>({
    url: '/api/store/register/email-code',
    data,
    showErrorMessage: false,
    showSuccessMessage: false
  })
}

export function registerStoreAccount(data: Record<string, unknown>) {
  return request.post<StoreAccount>({
    url: '/api/store/register',
    data,
    showErrorMessage: false,
    showSuccessMessage: false
  })
}

export function createStoreEditionOrder(planId: number, payMethod = '') {
  return request.post<{ orderNo: string; payUrl: string; amountCents: number; title: string }>({
    url: '/api/store/orders',
    data: { planId, payMethod: payMethod || undefined }
  })
}

export function createStoreItemOrder(
  itemKind: 'plugin' | 'template',
  itemId: string,
  payMethod = ''
) {
  return request.post<{ orderNo: string; payUrl: string; amountCents: number; title: string }>({
    url: '/api/store/orders',
    data: { itemKind, itemId, payMethod: payMethod || undefined }
  })
}

export function fetchStoreEditionOrder(orderNo: string) {
  return request.get<{ orderNo: string; status: string }>({
    url: `/api/store/orders/${orderNo}`,
    showErrorMessage: false
  })
}

export function fetchStoreManageLink() {
  return request.post<{ url: string }>({
    url: '/api/store/manage-link',
    showErrorMessage: false,
    showSuccessMessage: false
  })
}

export function logoutStoreAccount() {
  return request.post({
    url: '/api/store/logout',
    showErrorMessage: false,
    showSuccessMessage: false
  })
}

/** 向源站刷新本站快照。quiet 用于顶栏的后台刷新：失败时调用方会退回读本地状态，不弹错误提示。 */
export function refreshStoreSnapshot(force = false, quiet = false) {
  return request.post<StoreAccount>({
    url: force ? '/api/store/refresh?force=1' : '/api/store/refresh',
    showErrorMessage: !quiet
  })
}
