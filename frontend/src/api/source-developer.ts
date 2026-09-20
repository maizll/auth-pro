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
  reviewNote?: string
  updatedAt?: string
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
