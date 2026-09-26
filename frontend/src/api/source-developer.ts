import axios, { type AxiosRequestConfig } from 'axios'

const BASE = '/api/v1/source/developer'

export const DEVELOPER_TOKEN_KEY = 'developer_panel_token'
export const DEVELOPER_INFO_KEY = 'developer_panel_info'
export const AGENT_TOKEN_KEY = 'agent_panel_token'
export const AGENT_INFO_KEY = 'agent_panel_info'

export const DEVELOPER_USERNAME_PATTERN = /^[a-z0-9][a-z0-9-]{1,58}$/

export interface SourceDeveloperApplyResult {
  id: number
  agentId?: number
  username: string
  displayName?: string
  status: string
}

export interface SourceDeveloperApplyStatus {
  id?: number
  agentId?: number
  username: string
  displayName?: string
  email?: string
  status: string
  enabled?: boolean
  reviewNote?: string
}

export interface SourceDeveloperProfile {
  id: number
  username: string
  displayName: string
  email: string
  roleCode: string
}

export interface SourceDeveloperCatalogItem {
  id: string
  name: string
  version: string
  status: string
  appId?: number
  category?: string
  description?: string
  icon?: string
  sha256?: string
  downloadUrl?: string
  templateUrl?: string
  priceCents?: number
  billing?: string
  delivery?: string
  originUrl?: string
  originHealth?: string
  originHint?: string
  fulfillmentHint?: string
  packageSource?: string
  storedBySite?: boolean
  templateKey?: string
  changelog?: string
  latestVersion?: string
  minVersion?: string
  forceUpdate?: boolean
  schemaVersion?: number
  reviewNote?: string
  reviewedBy?: string
  author?: { name?: string; url?: string; email?: string }
  updatedAt?: string
  createdAt?: string
}

export interface SourceDeveloperCatalogApp {
  id: number
  appKey: string
  name: string
  enabled: boolean
  indexUrl?: string
}

export interface SourceDeveloperItems {
  plugins: SourceDeveloperCatalogItem[]
  homeTemplates: SourceDeveloperCatalogItem[]
}

export const DEVELOPER_APPLY_STATUS: Record<
  string,
  { label: string; type: 'primary' | 'success' | 'warning' | 'info' | 'danger'; description: string }
> = {
  pending: {
    label: '审核中',
    type: 'warning',
    description: '入驻申请已提交，请等待管理员在源站「入驻审核」中处理。'
  },
  approved: {
    label: '已通过',
    type: 'success',
    description: '申请已通过，可使用当前代理商账号进入开发者端。'
  },
  rejected: {
    label: '已拒绝',
    type: 'danger',
    description: '入驻申请未通过。可再次一键申请，仍使用当前代理商账号。'
  },
  frozen: {
    label: '已取消',
    type: 'danger',
    description: '开发者资格已被取消，无法进入开发者端或发布内容。可再次一键申请。'
  },
  cancelled: {
    label: '已取消',
    type: 'danger',
    description: '开发者资格已被取消，无法进入开发者端或发布内容。可再次一键申请。'
  }
}

export function agentAuthHeaders(): AxiosRequestConfig['headers'] {
  return { Authorization: `Bearer ${localStorage.getItem(AGENT_TOKEN_KEY) || ''}` }
}

export function developerAuthHeaders(): AxiosRequestConfig['headers'] {
  const token =
    localStorage.getItem(DEVELOPER_TOKEN_KEY) || localStorage.getItem(AGENT_TOKEN_KEY) || ''
  return { Authorization: `Bearer ${token}` }
}

export function enterDeveloperSessionFromAgent() {
  const token = localStorage.getItem(AGENT_TOKEN_KEY) || ''
  if (!token) return false
  localStorage.setItem(DEVELOPER_TOKEN_KEY, token)
  try {
    const info = JSON.parse(localStorage.getItem(AGENT_INFO_KEY) || '{}') as {
      email?: string
      name?: string
    }
    localStorage.setItem(
      DEVELOPER_INFO_KEY,
      JSON.stringify({
        username: info.email || '',
        displayName: info.name || info.email || ''
      })
    )
  } catch {
    /* Ignore malformed agent profile. */
  }
  return true
}

export function applySourceDeveloper() {
  return axios.post<{ code: number; msg: string; data: SourceDeveloperApplyResult }>(
    `${BASE}/apply`,
    {},
    { headers: agentAuthHeaders() }
  )
}

export function fetchSourceDeveloperApplyStatus() {
  return axios.get<{ code: number; msg: string; data: SourceDeveloperApplyStatus }>(
    `${BASE}/apply/status`,
    { headers: agentAuthHeaders() }
  )
}

export function fetchSourceDeveloperMe() {
  return axios.get<{ code: number; msg: string; data: SourceDeveloperProfile }>(`${BASE}/me`, {
    headers: developerAuthHeaders()
  })
}

export function rebindSourceDeveloperCatalogItems(
  appId: number,
  items: Array<{ kind: 'plugin' | 'template'; id: string }>
) {
  return axios.post<{ code: number; msg: string; data: { count: number } }>(
    `${BASE}/items/rebind`,
    { appId, items },
    { headers: developerAuthHeaders() }
  )
}

export function fetchSourceDeveloperItems(status?: string) {
  return axios.get<{ code: number; msg: string; data: SourceDeveloperItems }>(`${BASE}/items`, {
    headers: developerAuthHeaders(),
    params: status ? { status } : undefined
  })
}

export function fetchSourceDeveloperCatalogApps() {
  return axios.get<{
    code: number
    msg: string
    data: { list: SourceDeveloperCatalogApp[]; total: number }
  }>(`${BASE}/apps`, {
    headers: developerAuthHeaders()
  })
}

export interface SourceDeveloperCategory {
  key: string
  label: string
  kind: 'plugin' | 'template'
  builtin?: boolean
}

export interface SourceDeveloperVersion {
  kind: string
  itemId: string
  version: string
  changelog: string
  sha256: string
  status: string
  downloadUrl?: string
  templateUrl?: string
  originUrl?: string
  packageSource?: string
  storedBySite?: boolean
  reviewNote?: string
  reviewedBy?: string
  createdAt?: string
  updatedAt?: string
}

export interface SourceDeveloperPluginDraft {
  id: string
  appId: number
  category?: string
  name: string
  description?: string
  icon?: string
  version: string
  sha256?: string
  downloadUrl?: string
  packageSource?: 'upload' | 'public'
  storedBySite?: boolean
  priceCents?: number
  billing?: string
  changelog?: string
  minVersion?: string
  forceUpdate?: boolean
  author?: { name?: string; url?: string; email?: string }
}

export interface SourceDeveloperTemplateDraft {
  id?: string
  appId: number
  category?: string
  templateKey: string
  name: string
  description?: string
  version: string
  schemaVersion?: number
  sha256?: string
  templateUrl?: string
  packageSource?: 'upload' | 'public'
  storedBySite?: boolean
  priceCents?: number
  billing?: string
  changelog?: string
  minVersion?: string
  forceUpdate?: boolean
  author?: { name?: string; url?: string; email?: string }
}

export interface SourceDeveloperVersionDraft {
  version: string
  changelog?: string
  sha256?: string
  downloadUrl?: string
  templateUrl?: string
  packageSource?: 'upload' | 'public'
  storedBySite?: boolean
}

export interface SourceDeveloperAdApplication {
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

export interface SourceDeveloperAdApplicationDraft {
  appId?: number
  title: string
  imageUrl?: string
  linkUrl?: string
  positions: string[]
  note?: string
}

function developerConfig() {
  return { headers: developerAuthHeaders() }
}

export function fetchSourceDeveloperCategories() {
  return axios.get<{
    code: number
    msg: string
    data: { list: SourceDeveloperCategory[]; total: number }
  }>(`${BASE}/categories`, developerConfig())
}

export function upsertSourceDeveloperPlugin(payload: SourceDeveloperPluginDraft) {
  return axios.post<{ code: number; msg: string; data: SourceDeveloperCatalogItem }>(
    `${BASE}/plugins`,
    payload,
    developerConfig()
  )
}

export function pullSourceDeveloperPlugin(id: string) {
  return axios.post<{ code: number; msg: string; data: SourceDeveloperCatalogItem }>(
    `${BASE}/plugins/${encodeURIComponent(id)}/pull`,
    {},
    developerConfig()
  )
}

export function submitSourceDeveloperPlugin(id: string, note?: string) {
  return axios.post<{ code: number; msg: string; data: SourceDeveloperCatalogItem }>(
    `${BASE}/plugins/${encodeURIComponent(id)}/submit`,
    note ? { note } : {},
    developerConfig()
  )
}

export function fetchSourceDeveloperPluginVersions(id: string) {
  return axios.get<{
    code: number
    msg: string
    data: { list: SourceDeveloperVersion[]; total: number }
  }>(`${BASE}/plugins/${encodeURIComponent(id)}/versions`, developerConfig())
}

export function upsertSourceDeveloperPluginVersion(id: string, payload: SourceDeveloperVersionDraft) {
  return axios.post<{ code: number; msg: string; data: SourceDeveloperVersion }>(
    `${BASE}/plugins/${encodeURIComponent(id)}/versions`,
    payload,
    developerConfig()
  )
}

export function submitSourceDeveloperPluginVersion(id: string, version: string, note?: string) {
  return axios.post<{ code: number; msg: string; data: SourceDeveloperVersion }>(
    `${BASE}/plugins/${encodeURIComponent(id)}/versions/${encodeURIComponent(version)}/submit`,
    note ? { note } : {},
    developerConfig()
  )
}

export function upsertSourceDeveloperTemplate(payload: SourceDeveloperTemplateDraft) {
  return axios.post<{ code: number; msg: string; data: SourceDeveloperCatalogItem }>(
    `${BASE}/templates`,
    payload,
    developerConfig()
  )
}

export function pullSourceDeveloperTemplate(id: string) {
  return axios.post<{ code: number; msg: string; data: SourceDeveloperCatalogItem }>(
    `${BASE}/templates/${encodeURIComponent(id)}/pull`,
    {},
    developerConfig()
  )
}

export function submitSourceDeveloperTemplate(id: string, note?: string) {
  return axios.post<{ code: number; msg: string; data: SourceDeveloperCatalogItem }>(
    `${BASE}/templates/${encodeURIComponent(id)}/submit`,
    note ? { note } : {},
    developerConfig()
  )
}

export function fetchSourceDeveloperTemplateVersions(id: string) {
  return axios.get<{
    code: number
    msg: string
    data: { list: SourceDeveloperVersion[]; total: number }
  }>(`${BASE}/templates/${encodeURIComponent(id)}/versions`, developerConfig())
}

export function upsertSourceDeveloperTemplateVersion(
  id: string,
  payload: SourceDeveloperVersionDraft
) {
  return axios.post<{ code: number; msg: string; data: SourceDeveloperVersion }>(
    `${BASE}/templates/${encodeURIComponent(id)}/versions`,
    payload,
    developerConfig()
  )
}

export function submitSourceDeveloperTemplateVersion(id: string, version: string, note?: string) {
  return axios.post<{ code: number; msg: string; data: SourceDeveloperVersion }>(
    `${BASE}/templates/${encodeURIComponent(id)}/versions/${encodeURIComponent(version)}/submit`,
    note ? { note } : {},
    developerConfig()
  )
}

export function fetchSourceDeveloperAdApplications(status?: string) {
  return axios.get<{
    code: number
    msg: string
    data: { list: SourceDeveloperAdApplication[]; total: number }
  }>(`${BASE}/ad-applications`, {
    ...developerConfig(),
    params: status ? { status } : undefined
  })
}

export function createSourceDeveloperAdApplication(payload: SourceDeveloperAdApplicationDraft) {
  return axios.post<{ code: number; msg: string; data: SourceDeveloperAdApplication }>(
    `${BASE}/ad-applications`,
    payload,
    developerConfig()
  )
}

export interface SourceDeveloperPackageUpload {
  url: string
  sha256: string
  kind: string
  id: string
  name: string
  version: string
  description?: string
  category?: string
  icon?: string
  schemaVersion?: number
  stored: boolean
  storedBySite?: boolean
}

export function uploadSourceDeveloperPackage(data: FormData) {
  return axios.post<{ code: number; msg: string; data: SourceDeveloperPackageUpload }>(
    `${BASE}/packages/upload`,
    data,
    developerConfig()
  )
}

export function uploadSourceDeveloperAdvertisementImage(data: FormData) {
  return axios.post<{ code: number; msg: string; data: { url: string } }>(
    `${BASE}/advertisements/image`,
    data,
    developerConfig()
  )
}

async function triggerBlobDownload(blob: Blob, filename: string, fallbackType: string) {
  if (blob.type.includes('application/json')) {
    const body = JSON.parse(await blob.text()) as { msg?: string }
    throw new Error(body.msg || '下载失败')
  }
  const file = blob.type ? blob : new Blob([blob], { type: fallbackType })
  const url = URL.createObjectURL(file)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

export async function downloadSourceDeveloperStarter() {
  const res = await axios.get<Blob>(`${BASE}/starter.zip`, {
    ...developerConfig(),
    responseType: 'blob'
  })
  await triggerBlobDownload(res.data, 'auth-pro-developer-starter.zip', 'application/zip')
}

export async function downloadSourceDeveloperSkill() {
  const res = await axios.get<Blob>(`${BASE}/skill.md`, {
    ...developerConfig(),
    responseType: 'blob'
  })
  await triggerBlobDownload(res.data, 'SKILL.md', 'text/markdown;charset=utf-8')
}
