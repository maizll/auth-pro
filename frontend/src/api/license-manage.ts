/**
 * 授权管理相关 API
 *
 * 覆盖授权列表、应用管理、套餐管理、卡密管理、验证日志五个页面的数据接口。
 * 注意：授权/应用相关接口从 query 读取参数（使用 params），
 * 套餐与卡密接口从 body 读取参数（使用 data），差异是后端已有约定，请勿混用。
 */
import request from '@/utils/http'

// ==================== 类型定义 ====================

/** 通用分页响应（后端使用 list/total 字段） */
interface ListResponse<T> {
  list: T[]
  total: number
}

/** 应用下拉选项 */
export interface LicenseAppOption {
  id: number | string
  name: string
}

/** 授权列表项 */
export interface LicenseItem {
  id: number
  licenseNo: string
  domain: string
  ownerType: string
  ownerId: number
  ownerName: string
  appId: number
  appName: string
  type: string
  typeLabel: string
  status: string
  statusLabel: string
  source?: string
  sourceLabel?: string
  commercialActive?: boolean
  expireAt: string
  verifyCount: number
  boundSites?: number
  maxSites?: number
  remark: string
  createdAt: string
}

/** 授权列表搜索参数 */
export interface LicenseSearchParams {
  page: number
  pageSize: number
  keyword?: string
  type?: string
  status?: string
  source?: string
  appId?: string | number
}

/** 授权归属账号选项 */
export interface LicenseOwnerOption {
  id: number
  name: string
  account: string
  type: 'user' | 'agent'
}

/** 授权套餐选项（新增授权弹窗用） */
export interface LicensePlanOption {
  id: number
  name: string
  durationDays: number
  durationText: string
  price: number
}

/** 应用列表项 */
export interface AppSaleGap {
  code: string
  label: string
  path?: string
}

export interface LicenseAppItem {
  id: number
  name: string
  appKey: string
  appSecret: string
  purchaseLicenseTypes: string[]
  licenseCount: number
  recentVersion: string
  versionCount: number
  enabled: boolean
  archived?: boolean
  licenseRequired: boolean
  commercialProduct?: boolean
  saleGaps?: AppSaleGap[]
  graceDays?: number
  revokeOnPasswordChange?: boolean
  commercialFeatures?: string[]
  remark?: string
  createdAt: string
}

/** 套餐列表项 */
export interface PlanItem {
  id: number
  appId: number
  appName: string
  name: string
  licenseType: string
  durationDays: number
  durationText: string
  price: number
  maxSites: number
  sort: number
  enabled: boolean
  remark: string
  createdAt: string
}

/** 套餐搜索参数 */
export interface PlanSearchParams {
  page?: number
  pageSize?: number
  appId?: number
  keyword?: string
  status?: string
}

/** 卡密批次列表项 */
export interface CardBatchItem {
  id: number
  batchNo: string
  appId: number
  appName: string
  planId: number
  planName: string
  durationDays: number
  price: number
  typeLabel: string
  unusedCount: number
  redeemedCount: number
  disabledCount: number
  status: string
  remark: string
  createdAt: string
}

/** 卡密批次搜索参数 */
export interface CardBatchSearchParams {
  page: number
  pageSize: number
  keyword?: string
  appId?: number
  status?: string
}

/** 卡密明细项 */
export interface CardItem {
  id: number
  cardCode: string
  status: string
  redeemedByType?: string
  redeemedByAccount?: string
  licenseId?: number
  redeemedAt?: string
}

/** 验证日志列表项 */
export interface VerifyLogItem {
  id: number
  requestDomain: string
  appName: string
  result: string
  reason: string
  clientIp: string
  serverIp: string
  responseTime: number
  createdAt: string
}

/** 验证日志搜索参数 */
export interface VerifyLogSearchParams {
  page: number
  pageSize: number
  keyword?: string
  appId?: string | number
  result?: string
  startDate?: string
  endDate?: string
}

/** 密钥绑定站点 */
export interface LicenseSiteItem {
  id: number
  targetType: string
  target: string
  serverIp: string
  firstSeenAt: string
  lastSeenAt: string
}

// ==================== 授权 ====================

/** 授权列表 */
export function fetchLicenseList(params: LicenseSearchParams) {
  return request.get<ListResponse<LicenseItem>>({ url: '/api/license/list', params })
}

/** 应用下拉选项（授权视角，仅 id/name） */
export function fetchLicenseAppOptions() {
  return request.get<LicenseAppOption[]>({ url: '/api/license/apps' })
}

/** 归属账号远程搜索 */
export function fetchLicenseOwners(params: {
  ownerType: 'user' | 'agent'
  keyword: string
  limit: number
}) {
  return request.get<LicenseOwnerOption[]>({ url: '/api/license/owners', params })
}

/** 新增授权 */
export function fetchCreateLicense(params: {
  appId: number
  planId: number
  ownerType: string
  ownerId: number | null
  type: string
  domain: string
  remark: string
}) {
  return request.post({ url: '/api/license/create', params })
}

/** 编辑授权 */
export function fetchUpdateLicense(
  id: number,
  params: { appId: number; type: string; domain: string; expireAt: string; remark: string }
) {
  return request.put({ url: `/api/license/${id}`, params })
}

/** 启用/禁用授权 */
export function fetchToggleLicense(id: number, status: string) {
  return request.put({ url: `/api/license/${id}/toggle`, params: { status } })
}

/** 删除授权 */
export function fetchDeleteLicense(id: number) {
  return request.del({ url: `/api/license/${id}` })
}

/** 密钥绑定站点列表 */
export function fetchLicenseSites(licenseId: number) {
  return request.get<{ list: LicenseSiteItem[]; boundSites: number; maxSites: number }>({
    url: `/api/license/${licenseId}/sites`
  })
}

/** 解绑站点 */
export function fetchUnbindLicenseSite(licenseId: number, siteId: number) {
  return request.del({ url: `/api/license/${licenseId}/sites/${siteId}` })
}

// ==================== 应用 ====================

/** 应用列表（完整字段） */
export function fetchLicenseAppList() {
  return request.get<LicenseAppItem[]>({ url: '/api/app/list' })
}

/** 新增应用 */
export interface CommercialAppPayload {
  name: string
  enabled: boolean
  remark: string
  purchaseLicenseTypes: string[]
  commercialProduct?: boolean
  graceDays?: number
  revokeOnPasswordChange?: boolean
  commercialFeatures?: string[]
}

export function fetchCreateLicenseApp(params: CommercialAppPayload) {
  return request.post<{ id: number; switched?: boolean }>({ url: '/api/app/create', params })
}

/** 编辑应用 */
export function fetchUpdateLicenseApp(id: number, params: CommercialAppPayload) {
  return request.put<{ switched?: boolean }>({ url: `/api/app/${id}`, params })
}

/** 补生成商店签名密钥 */
export function fetchEnsureStoreSnapshotKey() {
  return request.post({ url: '/api/app/store-snapshot-key', showSuccessMessage: true })
}

export function fetchGrantCommercialEdition(id: number) {
  return request.post({
    url: `/api/v1/source/admin/store/licenses/${id}/grant`,
    data: { period: 'permanent' },
    showSuccessMessage: true
  })
}

export function fetchRevokeCommercialEdition(id: number, reason: string) {
  return request.post({
    url: `/api/v1/source/admin/store/licenses/${id}/revoke`,
    data: { reason },
    showSuccessMessage: true
  })
}

/** 恢复已归档的应用 */
export function fetchRestoreLicenseApp(id: number) {
  return request.post({ url: `/api/app/${id}/restore` })
}

/** 归档应用。有目录条目时要带上迁移目标，或 archive 表示原地归档。 */
export function fetchDeleteLicenseApp(
  id: number,
  options?: { migrateAppId?: number; archive?: boolean }
) {
  const params: Record<string, number> = {}
  if (options?.migrateAppId) params.migrateAppId = options.migrateAppId
  if (options?.archive) params.archive = 1
  return request.del({
    url: `/api/app/${id}`,
    params: Object.keys(params).length ? params : undefined
  })
}

/** 重置 AppSecret */
export function fetchResetAppSecret(id: number) {
  return request.put<{ appSecret: string }>({ url: `/api/app/${id}/reset-secret` })
}

/** 开关授权校验 */
export function fetchUpdateAppLicenseRequired(id: number, licenseRequired: boolean) {
  return request.put<{ licenseRequired: boolean }>({
    url: `/api/app/${id}/license-required`,
    params: { licenseRequired }
  })
}

/** 按应用、按语言下载客户端接入 ZIP（密钥由后端按 appId 读取，不走前端） */
export function fetchDownloadSDKPack(payload: {
  appId: number
  language: 'php' | 'node' | 'python' | 'go' | 'browser'
  modules: string[]
  baseUrl?: string
}) {
  return request.post<Blob>({
    url: '/api/sdk/pack',
    data: payload,
    responseType: 'blob',
    timeout: 60_000
  })
}

// ==================== 套餐 ====================

/** 套餐列表（不分页，按应用/关键词/状态过滤） */
export function fetchPlanList(params: PlanSearchParams) {
  return request.get<PlanItem[]>({ url: '/api/plan/list', params })
}

/** 套餐提交参数（body 提交） */
export interface PlanPayload {
  appId?: number
  name: string
  licenseType: string
  durationDays: number
  price: number
  maxSites: number
  sort: number
  enabled: boolean
  remark: string
}

/** 新增套餐 */
export function fetchCreatePlan(data: PlanPayload) {
  return request.post({ url: '/api/plan/create', data })
}

/** 编辑套餐 */
export function fetchUpdatePlan(id: number, data: PlanPayload) {
  return request.put({ url: `/api/plan/${id}`, data })
}

/** 启用/禁用套餐 */
export function fetchTogglePlan(id: number) {
  return request.put({ url: `/api/plan/${id}/toggle` })
}

/** 删除套餐 */
export function fetchDeletePlan(id: number) {
  return request.del({ url: `/api/plan/${id}` })
}

// ==================== 卡密 ====================

/** 卡密批次列表 */
export function fetchCardBatches(params: CardBatchSearchParams) {
  return request.get<ListResponse<CardBatchItem>>({ url: '/api/license/cards/batches', params })
}

/** 生成卡密（body 提交） */
export function fetchCreateCardBatch(data: {
  appId?: number
  planId?: number
  type: string
  quantity: number
  remark: string
}) {
  return request.post<{ cards: string[] }>({ url: '/api/license/cards/batches', data })
}

/** 删除卡密批次 */
export function fetchDeleteCardBatch(id: number) {
  return request.del({ url: `/api/license/cards/batches/${id}` })
}

/** 批次卡密明细 */
export function fetchBatchCards(
  batchId: number,
  params: { status?: string; page: number; pageSize: number }
) {
  return request.get<ListResponse<CardItem>>({
    url: `/api/license/cards/batches/${batchId}/cards`,
    params
  })
}

/** 禁用/恢复卡密（body 提交） */
export function fetchUpdateCardStatus(id: number, status: string) {
  return request.put({ url: `/api/license/cards/${id}/status`, data: { status } })
}

// ==================== 验证日志 ====================

/** 验证日志列表 */
export function fetchVerifyLogList(params: VerifyLogSearchParams) {
  return request.get<ListResponse<VerifyLogItem>>({ url: '/api/verify-log/list', params })
}

/** 清空验证日志 */
export function fetchClearVerifyLogs() {
  return request.del({ url: '/api/verify-log/clear' })
}
