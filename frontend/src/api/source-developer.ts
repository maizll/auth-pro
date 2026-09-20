import axios, { type AxiosRequestConfig } from 'axios'

const BASE = '/api/v1/source/developer'

export const DEVELOPER_TOKEN_KEY = 'developer_panel_token'
export const DEVELOPER_INFO_KEY = 'developer_panel_info'
export const AGENT_DEVELOPER_APPLY_KEY = 'agent_panel_developer_apply_username'

export const DEVELOPER_USERNAME_PATTERN = /^[a-z0-9][a-z0-9-]{1,58}$/

export interface SourceDeveloperApplyPayload {
  username: string
  password: string
  email: string
  displayName: string
  reason: string
}

export interface SourceDeveloperApplyResult {
  id: number
  username: string
  status: string
}

export interface SourceDeveloperApplyStatus {
  id?: number
  username: string
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
    description: '入驻申请已提交，请等待管理员审核。'
  },
  approved: {
    label: '已通过',
    type: 'success',
    description: '申请已通过，可使用开发者账号登录开发者端。'
  },
  rejected: {
    label: '已拒绝',
    type: 'danger',
    description: '入驻申请未通过。同一用户名不可重复申请，请更换用户名后重新提交。'
  },
  frozen: {
    label: '已取消',
    type: 'danger',
    description: '开发者资格已被取消，无法再登录开发者端或发布内容。如需重新入驻，请更换用户名后再次申请。'
  },
  cancelled: {
    label: '已取消',
    type: 'danger',
    description: '开发者资格已被取消，无法再登录开发者端或发布内容。如需重新入驻，请更换用户名后再次申请。'
  }
}

export function developerAuthHeaders(): AxiosRequestConfig['headers'] {
  return { Authorization: `Bearer ${localStorage.getItem(DEVELOPER_TOKEN_KEY) || ''}` }
}

export function applySourceDeveloper(payload: SourceDeveloperApplyPayload) {
  return axios.post<{ code: number; msg: string; data: SourceDeveloperApplyResult }>(
    `${BASE}/apply`,
    payload
  )
}

export function fetchSourceDeveloperApplyStatus(username: string) {
  return axios.post<{ code: number; msg: string; data: SourceDeveloperApplyStatus }>(
    `${BASE}/apply/status`,
    { username }
  )
}

export function loginSourceDeveloper(username: string, password: string) {
  return axios.post<{
    code: number
    msg: string
    data: { token: string; username: string; displayName: string }
  }>(`${BASE}/login`, { username, password })
}

export function fetchSourceDeveloperMe() {
  return axios.get<{ code: number; msg: string; data: SourceDeveloperProfile }>(`${BASE}/me`, {
    headers: developerAuthHeaders()
  })
}

export function fetchSourceDeveloperItems() {
  return axios.get<{ code: number; msg: string; data: SourceDeveloperItems }>(`${BASE}/items`, {
    headers: developerAuthHeaders()
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

export function rememberDeveloperApplyUsername(username: string) {
  const value = username.trim().toLowerCase()
  if (!value) return
  localStorage.setItem(AGENT_DEVELOPER_APPLY_KEY, value)
}

export function loadRememberedDeveloperApplyUsername() {
  return (localStorage.getItem(AGENT_DEVELOPER_APPLY_KEY) || '').trim().toLowerCase()
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
