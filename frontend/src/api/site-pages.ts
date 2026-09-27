// 官网页面后台接口。公开页不走这里，避免带上管理员令牌。
import request from '@/utils/http'

export interface SiteNavRow {
  id: number
  label: string
  kind: 'builtin' | 'external'
  builtinKey: string
  href: string
  enabled: boolean
  sort: number
}

export interface SiteDocCategory {
  id: number
  name: string
  slug: string
  sort: number
  hidden: boolean
}

export interface SiteDocRow {
  id: number
  categoryId: number
  category: string
  title: string
  slug: string
  summary: string
  body?: string
  sort: number
  hidden: boolean
}

export interface SiteChangelogRow {
  id: number
  version: string
  releasedOn: string
  tag: string
  body: string
  hidden: boolean
  source: string
  sort: number
}

export interface SiteCompareRow {
  id: number
  body: string
  sort: number
  hidden: boolean
}

export function fetchAdminNav() {
  return request.get<{ list: SiteNavRow[] }>({ url: '/api/system/site-pages/nav' })
}

export function createAdminNav(data: { label: string; href: string }) {
  return request.post<{ id: number }>({ url: '/api/system/site-pages/nav', data })
}

export function updateAdminNav(id: number, data: Partial<SiteNavRow> & { label: string }) {
  return request.put({ url: `/api/system/site-pages/nav/${id}`, data })
}

export function deleteAdminNav(id: number) {
  return request.del({ url: `/api/system/site-pages/nav/${id}` })
}

export function fetchAdminDocCategories() {
  return request.get<{ list: SiteDocCategory[] }>({ url: '/api/system/site-pages/doc-categories' })
}

export function saveAdminDocCategory(id: number | null, data: Record<string, unknown>) {
  if (id) return request.put({ url: `/api/system/site-pages/doc-categories/${id}`, data })
  return request.post({ url: '/api/system/site-pages/doc-categories', data })
}

export function deleteAdminDocCategory(id: number) {
  return request.del({ url: `/api/system/site-pages/doc-categories/${id}` })
}

export function fetchAdminDocs() {
  return request.get<{ list: SiteDocRow[] }>({ url: '/api/system/site-pages/docs' })
}

export function saveAdminDoc(id: number | null, data: Record<string, unknown>) {
  if (id) return request.put({ url: `/api/system/site-pages/docs/${id}`, data })
  return request.post({ url: '/api/system/site-pages/docs', data })
}

export function deleteAdminDoc(id: number) {
  return request.del({ url: `/api/system/site-pages/docs/${id}` })
}

export function fetchAdminChangelog() {
  return request.get<{ list: SiteChangelogRow[] }>({ url: '/api/system/site-pages/changelog' })
}

export function saveAdminChangelog(id: number | null, data: Record<string, unknown>) {
  if (id) return request.put({ url: `/api/system/site-pages/changelog/${id}`, data })
  return request.post({ url: '/api/system/site-pages/changelog', data })
}

export function deleteAdminChangelog(id: number) {
  return request.del({ url: `/api/system/site-pages/changelog/${id}` })
}

export function fetchAdminCompare() {
  return request.get<{ list: SiteCompareRow[] }>({ url: '/api/system/site-pages/compare' })
}

export function saveAdminCompare(id: number | null, data: Record<string, unknown>) {
  if (id) return request.put({ url: `/api/system/site-pages/compare/${id}`, data })
  return request.post({ url: '/api/system/site-pages/compare', data })
}

export function deleteAdminCompare(id: number) {
  return request.del({ url: `/api/system/site-pages/compare/${id}` })
}
