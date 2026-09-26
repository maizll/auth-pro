import request from '@/utils/http'

const BASE = '/api/v1/source/admin'
const PACKAGE_TIMEOUT = 120000

export interface SourceAuthor {
  name?: string
  url?: string
  email?: string
}

export interface SourceListResponse<T> {
  list: T[]
  total: number
}

export interface SourceApplication {
  id: number
  agentId?: number
  username: string
  email: string
  displayName: string
  reason: string
  status: string
  reviewNote: string
  reviewedBy: string
  reviewedAt: string
  createdAt: string
}

export interface SourceDeveloper {
  id: number
  agentId?: number
  username: string
  email: string
  displayName: string
  enabled: boolean
  createdAt: string
}

export interface SourceCatalogApp {
  id: number
  appKey: string
  name: string
  enabled: boolean
  indexUrl?: string
}

export interface SourcePlugin {
  id: string
  developerId: number
  appId: number
  category: string
  name: string
  description: string
  icon: string
  version: string
  author: SourceAuthor
  sha256: string
  downloadUrl: string
  priceCents?: number
  billing?: string
  delivery?: string
  originUrl?: string
  originHealth?: string
  originHint?: string
  changelog: string
  latestVersion: string
  minVersion: string
  forceUpdate: boolean
  status: string
  reviewNote: string
  reviewedBy: string
  updatedAt: string
  createdAt: string
}

export interface SourceCatalogCategory {
  key: string
  label: string
  kind: 'plugin' | 'template'
  builtin: boolean
}

export interface SourceTemplate {
  id: string
  developerId: number
  appId: number
  category?: string
  templateKey: string
  name: string
  description: string
  version: string
  schemaVersion: number
  sha256: string
  templateUrl: string
  priceCents?: number
  billing?: string
  delivery?: string
  originUrl?: string
  originHealth?: string
  originHint?: string
  changelog: string
  latestVersion: string
  minVersion: string
  forceUpdate: boolean
  status: string
  author: SourceAuthor
  reviewNote: string
  reviewedBy: string
  updatedAt: string
  createdAt: string
}

export interface SourceCatalogItem {
  kind: 'plugin' | 'template'
  id: string
  appId: number
  category: string
  categoryLabel?: string
  name: string
  description: string
  version: string
  latestVersion: string
  status: string
  sha256: string
  downloadUrl?: string
  templateUrl?: string
  priceCents?: number
  billing?: string
  delivery?: string
  originUrl?: string
  originHealth?: string
  originHint?: string
  location?: string
  schemaVersion?: number
  templateKey?: string
  author: SourceAuthor
  icon?: string
  changelog?: string
  updatedAt: string
  createdAt: string
}

export interface SourceVersion {
  kind: string
  itemId: string
  version: string
  changelog: string
  sha256: string
  status: string
  downloadUrl?: string
  templateUrl?: string
  originUrl?: string
  reviewNote: string
  reviewedBy: string
  createdAt: string
  updatedAt: string
}

export interface SourceAuditEntry {
  id: number
  actorType: string
  actorName: string
  action: string
  targetType: string
  targetId: string
  detail: string
  createdAt: string
}

export interface SourceIndexSnapshot {
  payload?: string
  generatedAt?: string
  generatedBy?: string
}

export interface SourceIndexData {
  live: Record<string, unknown>
  name: string
  appKey?: string
  appId?: number
  indexUrl?: string
  pluginCount: number
  templateCount: number
  snapshot?: SourceIndexSnapshot
}

export interface SourceReleaseSettings {
  provider: string
  owner: string
  repo: string
  tagStrategy: string
  branch: string
  hasToken: boolean
  tokenMasked: string
  configured: boolean
}

export interface SourcePackageManifest {
  kind: string
  id: string
  name: string
  version: string
  description: string
  author: SourceAuthor
  sha256: string
  filename: string
  size: number
  stored: boolean
  category?: string
  icon?: string
  schemaVersion?: number
  templateKey?: string
  manifestPath?: string
}

export interface SourcePackagePublishResult {
  kind: string
  id: string
  version: string
  sha256: string
  downloadUrl?: string
  templateUrl?: string
  pushed: boolean
  storedPackage: boolean
  manifest: SourcePackageManifest
  item: SourcePlugin | SourceTemplate
}

export interface SourceAdvertisement {
  id: string
  title: string
  imageUrl: string
  destinationUrl: string
  position: string
  /** 多选广告位；缺省时回退到单个 position */
  positions?: string[]
  weight: number
  startAt: string
  endAt: string
  description: string
}

export interface SourceAdPlaceholder {
  title: string
  description: string
  linkUrl: string
}

export interface SourcePluginDraft {
  id: string
  appId: number
  category?: string
  name: string
  description?: string
  icon?: string
  version: string
  sha256: string
  downloadUrl: string
  priceCents?: number
  changelog?: string
  minVersion?: string
  forceUpdate?: boolean
  author?: SourceAuthor
  shelf?: boolean
}

export interface SourceTemplateDraft {
  id?: string
  appId: number
  category?: string
  templateKey: string
  name: string
  description?: string
  version: string
  schemaVersion?: number
  sha256: string
  templateUrl: string
  priceCents?: number
  changelog?: string
  minVersion?: string
  forceUpdate?: boolean
  author?: SourceAuthor
  shelf?: boolean
}

export interface SourceVersionDraft {
  version: string
  changelog?: string
  sha256: string
  downloadUrl?: string
  templateUrl?: string
}

export type SourceItemStatus =
  | 'draft'
  | 'review'
  | 'approved'
  | 'published'
  | 'hidden'
  | 'rejected'
  | 'deprecated'

export type SourceVersionStatus = 'draft' | 'pending' | 'published' | 'deprecated'

export const SOURCE_ITEM_STATUS: Record<
  string,
  { label: string; type: 'primary' | 'success' | 'warning' | 'info' | 'danger' }
> = {
  draft: { label: '草稿', type: 'info' },
  review: { label: '待审核', type: 'warning' },
  approved: { label: '已通过', type: 'primary' },
  published: { label: '已上架', type: 'success' },
  hidden: { label: '已下架', type: 'info' },
  rejected: { label: '已驳回', type: 'danger' },
  deprecated: { label: '已弃用', type: 'warning' },
  pending: { label: '待处理', type: 'warning' },
  frozen: { label: '已冻结', type: 'danger' }
}

export const SOURCE_VERSION_STATUS: Record<
  string,
  { label: string; type: 'primary' | 'success' | 'warning' | 'info' | 'danger' }
> = {
  draft: { label: '草稿', type: 'info' },
  pending: { label: '待审核', type: 'warning' },
  published: { label: '已发布', type: 'success' },
  deprecated: { label: '已弃用', type: 'warning' }
}

export const AD_POSITIONS = [
  { value: 'home-banner', label: '首页横幅' },
  { value: 'sidebar', label: '侧栏' },
  { value: 'popup', label: '弹窗' }
] as const

export function advertisementPositionList(item: { position?: string; positions?: string[] }) {
  if (item.positions?.length) {
    return [...new Set(item.positions.filter(Boolean))]
  }
  return item.position ? [item.position] : []
}

function noteBody(note?: string) {
  return note ? { note } : {}
}

export function fetchSourceApplications(status: string = 'pending') {
  return request.get<SourceListResponse<SourceApplication>>({
    url: `${BASE}/applications`,
    params: { status }
  })
}

export function approveSourceApplication(id: number) {
  return request.post({ url: `${BASE}/applications/${id}/approve`, data: {} })
}

export function rejectSourceApplication(id: number, note?: string) {
  return request.post({ url: `${BASE}/applications/${id}/reject`, data: noteBody(note) })
}

export function freezeSourceApplication(id: number, note?: string) {
  return request.post({ url: `${BASE}/applications/${id}/freeze`, data: noteBody(note) })
}

export function fetchSourceDevelopers() {
  return request.get<SourceListResponse<SourceDeveloper>>({ url: `${BASE}/developers` })
}

export function freezeSourceDeveloper(id: number, note?: string) {
  return request.post({ url: `${BASE}/developers/${id}/freeze`, data: noteBody(note) })
}

export const SOURCE_APPLICATION_STATUS: Record<
  string,
  { label: string; type: 'primary' | 'success' | 'warning' | 'info' | 'danger' }
> = {
  pending: { label: '待审核', type: 'warning' },
  approved: { label: '已通过', type: 'success' },
  rejected: { label: '已拒绝', type: 'danger' },
  cancelled: { label: '已取消', type: 'info' },
  frozen: { label: '已冻结', type: 'info' }
}

/** Alias used by applications admin page (maps to freeze). */
export function cancelSourceApplication(id: number, note?: string) {
  return freezeSourceApplication(id, note)
}

/** Alias used by applications admin page (maps to freeze developer). */
export function cancelSourceDeveloper(id: number, note?: string) {
  return freezeSourceDeveloper(id, note)
}


export function fetchSourceCatalogApps() {
  return request.get<SourceListResponse<SourceCatalogApp>>({ url: `${BASE}/apps` })
}

export function fetchSourceCatalogCategories() {
  return request.get<{ list: SourceCatalogCategory[]; extras: SourceCatalogCategory[] }>({
    url: `${BASE}/categories`
  })
}

export function saveSourceCatalogCategories(extras: Array<Pick<SourceCatalogCategory, 'key' | 'label' | 'kind'>>) {
  return request.put<{ list: SourceCatalogCategory[]; extras: SourceCatalogCategory[] }>({
    url: `${BASE}/categories`,
    data: { extras }
  })
}

export function fetchSourceCatalogItems(status?: string, category?: string, appId?: number) {
  return request.get<SourceListResponse<SourceCatalogItem>>({
    url: `${BASE}/catalog-items`,
    params: {
      ...(status ? { status } : {}),
      ...(category ? { category } : {}),
      ...(appId && appId > 0 ? { app_id: appId } : {})
    }
  })
}

export function fetchSourcePlugins(status?: string, appId?: number) {
  return request.get<SourceListResponse<SourcePlugin>>({
    url: `${BASE}/plugins`,
    params: {
      ...(status ? { status } : {}),
      ...(appId && appId > 0 ? { app_id: appId } : {})
    }
  })
}

export function registerSourcePlugin(payload: SourcePluginDraft) {
  return request.put<SourcePlugin>({ url: `${BASE}/plugins`, data: payload })
}

export function updateSourcePlugin(id: string, payload: SourcePluginDraft & { note?: string }) {
  return request.put<SourcePlugin>({
    url: `${BASE}/plugins/${encodeURIComponent(id)}`,
    data: payload
  })
}

export function pullSourcePlugin(id: string) {
  return request.post<SourcePlugin>({
    url: `${BASE}/plugins/${encodeURIComponent(id)}/pull`
  })
}

export function setSourcePluginStatus(
  id: string,
  action: 'approve' | 'reject' | 'shelf' | 'unshelf' | 'deprecate',
  note?: string
) {
  return request.post<SourcePlugin>({
    url: `${BASE}/plugins/${encodeURIComponent(id)}/${action}`,
    data: noteBody(note)
  })
}

export function fetchSourcePluginVersions(id: string) {
  return request.get<SourceListResponse<SourceVersion>>({
    url: `${BASE}/plugins/${encodeURIComponent(id)}/versions`
  })
}

export function registerSourcePluginVersion(id: string, payload: SourceVersionDraft) {
  return request.post<SourceVersion>({
    url: `${BASE}/plugins/${encodeURIComponent(id)}/versions`,
    data: payload
  })
}

export function setSourcePluginVersionStatus(
  id: string,
  version: string,
  action: 'approve' | 'reject' | 'deprecate' | 'latest',
  note?: string
) {
  return request.post({
    url: `${BASE}/plugins/${encodeURIComponent(id)}/versions/${encodeURIComponent(version)}/${action}`,
    data: noteBody(note)
  })
}

export function fetchSourceTemplates(status?: string, appId?: number) {
  return request.get<SourceListResponse<SourceTemplate>>({
    url: `${BASE}/templates`,
    params: {
      ...(status ? { status } : {}),
      ...(appId && appId > 0 ? { app_id: appId } : {})
    }
  })
}

export function registerSourceTemplate(payload: SourceTemplateDraft) {
  return request.put<SourceTemplate>({ url: `${BASE}/templates`, data: payload })
}

export function updateSourceTemplate(id: string, payload: SourceTemplateDraft & { note?: string }) {
  return request.put<SourceTemplate>({
    url: `${BASE}/templates/${encodeURIComponent(id)}`,
    data: payload
  })
}

export function pullSourceTemplate(id: string) {
  return request.post<SourceTemplate>({
    url: `${BASE}/templates/${encodeURIComponent(id)}/pull`
  })
}

export function setSourceTemplateStatus(
  id: string,
  action: 'approve' | 'reject' | 'shelf' | 'unshelf' | 'deprecate',
  note?: string
) {
  return request.post<SourceTemplate>({
    url: `${BASE}/templates/${encodeURIComponent(id)}/${action}`,
    data: noteBody(note)
  })
}

export function fetchSourceTemplateVersions(id: string) {
  return request.get<SourceListResponse<SourceVersion>>({
    url: `${BASE}/templates/${encodeURIComponent(id)}/versions`
  })
}

export function registerSourceTemplateVersion(id: string, payload: SourceVersionDraft) {
  return request.post<SourceVersion>({
    url: `${BASE}/templates/${encodeURIComponent(id)}/versions`,
    data: payload
  })
}

export function setSourceTemplateVersionStatus(
  id: string,
  version: string,
  action: 'approve' | 'reject' | 'deprecate' | 'latest',
  note?: string
) {
  return request.post({
    url: `${BASE}/templates/${encodeURIComponent(id)}/versions/${encodeURIComponent(version)}/${action}`,
    data: noteBody(note)
  })
}

export function fetchSourceIndex(appId?: number) {
  return request.get<SourceIndexData>({
    url: `${BASE}/index`,
    params: appId && appId > 0 ? { app_id: appId } : undefined
  })
}

export function regenerateSourceIndex(appId?: number) {
  return request.post<SourceIndexData>({
    url: `${BASE}/index/regenerate`,
    params: appId && appId > 0 ? { app_id: appId } : undefined,
    data: {}
  })
}

export function fetchSourceAudit(limit = 100) {
  return request.get<SourceListResponse<SourceAuditEntry>>({
    url: `${BASE}/audit`,
    params: { limit }
  })
}

export interface SourceStoreSettings {
  productAppKey: string
  freePlanId: string
  graceDays?: number
  revokeOnPasswordChange?: boolean
  commercialFeatures?: string[]
}

export function fetchSourceStoreSettings() {
  return request.get<SourceStoreSettings>({ url: `${BASE}/settings/store` })
}

export function saveSourceStoreSettings(payload: SourceStoreSettings) {
  return request.put<SourceStoreSettings>({ url: `${BASE}/settings/store`, data: payload })
}

export function fetchSourceReleaseSettings() {
  return request.get<SourceReleaseSettings>({ url: `${BASE}/settings/release` })
}

export function saveSourceReleaseSettings(payload: {
  provider: string
  owner: string
  repo: string
  token?: string
  tagStrategy: string
  branch: string
}) {
  return request.put<SourceReleaseSettings>({ url: `${BASE}/settings/release`, data: payload })
}

export interface SourceReleaseTestResult {
  provider: string
  owner: string
  repo: string
  fullName?: string
  htmlUrl?: string
  private?: boolean
  permissions?: {
    admin?: boolean
    push?: boolean
    pull?: boolean
  }
}

export function testSourceReleaseSettings(payload: {
  provider: string
  owner: string
  repo: string
  token?: string
  tagStrategy: string
  branch: string
}) {
  return request.post<SourceReleaseTestResult>({
    url: `${BASE}/settings/release/test`,
    data: payload,
    timeout: PACKAGE_TIMEOUT
  })
}

export function fetchSourcePackageSchema() {
  return request.get<Record<string, unknown>>({ url: `${BASE}/packages/schema` })
}

export function parseSourcePackage(form: FormData) {
  return request.post<SourcePackageManifest>({
    url: `${BASE}/packages/parse`,
    data: form,
    timeout: PACKAGE_TIMEOUT
  })
}

export function publishSourcePackage(form: FormData) {
  return request.post<SourcePackagePublishResult>({
    url: `${BASE}/packages/publish`,
    data: form,
    timeout: PACKAGE_TIMEOUT
  })
}

export function fetchSourceAdvertisements() {
  return request.get<{ records: SourceAdvertisement[]; placeholder: SourceAdPlaceholder }>({
    url: `${BASE}/advertisements`
  })
}

export function saveSourceAdvertisement(payload: SourceAdvertisement) {
  return request.put<SourceAdvertisement>({ url: `${BASE}/advertisements`, data: payload })
}

export function uploadSourceAdvertisementImage(data: FormData) {
  return request.post<{ url: string }>({
    url: `${BASE}/advertisements/image`,
    data
  })
}

export function saveSourceAdPlaceholder(payload: SourceAdPlaceholder) {
  return request.put<SourceAdPlaceholder>({
    url: `${BASE}/advertisements/placeholder`,
    data: payload
  })
}

export function deleteSourceAdvertisement(id: string) {
  return request.del({ url: `${BASE}/advertisements/${encodeURIComponent(id)}` })
}

export interface SourceAdApplication {
  id: number
  developerId: number
  developerUsername?: string
  appId?: number
  title: string
  imageUrl?: string
  linkUrl?: string
  positions: string[]
  note?: string
  status: string
  reviewNote?: string
  reviewedBy?: string
  reviewedAt?: string
  advertisementId?: string
  createdAt?: string
  updatedAt?: string
}

export function fetchSourceAdApplications(status?: string) {
  return request.get<SourceListResponse<SourceAdApplication>>({
    url: `${BASE}/ad-applications`,
    params: status ? { status } : undefined
  })
}

export function approveSourceAdApplication(id: number, note?: string) {
  return request.post({ url: `${BASE}/ad-applications/${id}/approve`, data: noteBody(note) })
}

export function rejectSourceAdApplication(id: number, note?: string) {
  return request.post({ url: `${BASE}/ad-applications/${id}/reject`, data: noteBody(note) })
}
