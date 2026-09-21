import axios from 'axios'
import { AGENT_TOKEN_KEY, DEVELOPER_TOKEN_KEY } from '@/api/source-developer'
import { useUserStore } from '@/store/modules/user'

const BASE = '/api/v1/notifications'

export type NotificationTab = 'notice' | 'message' | 'todo'

export interface InAppNotification {
  id: number
  category: NotificationTab
  title: string
  body?: string
  link?: string
  read: boolean
  createdAt: string
  eventType: string
  refType?: string
  refId?: string
  derived?: boolean
}

export interface NotificationUnreadCount {
  count: number
  todo?: number
}

function isPanelPath(path: string, prefix: string): boolean {
  return path === prefix || path.startsWith(`${prefix}/`)
}

/**
 * 管理端令牌与 axios 拦截器相同，读取 Pinia userStore.accessToken
 * （持久化键 sys-v{version}-user）。仅 /user 与 /user/* 读取用户面板令牌。
 */
function panelToken(): string {
  if (typeof window === 'undefined') return ''
  const path = window.location.pathname
  if (isPanelPath(path, '/developer-panel')) {
    return localStorage.getItem(DEVELOPER_TOKEN_KEY) || localStorage.getItem(AGENT_TOKEN_KEY) || ''
  }
  if (isPanelPath(path, '/agent-panel')) {
    return localStorage.getItem(AGENT_TOKEN_KEY) || ''
  }
  if (isPanelPath(path, '/user')) {
    return localStorage.getItem('user_panel_token') || ''
  }
  return useUserStore().accessToken || ''
}

function authConfig(): { headers: { Authorization: string } } | null {
  const token = panelToken().trim()
  if (!token) {
    console.warn('[notifications] 缺少登录令牌，已跳过通知请求')
    return null
  }
  return { headers: { Authorization: `Bearer ${token}` } }
}

function rejectMissingToken(): Promise<never> {
  return Promise.reject(new Error('缺少登录令牌，已跳过通知请求'))
}

/** 角标 = 持久化未读 + 派生待办。入驻申请即使通知写入失败，待办仍能让角标增加。 */
export function notificationBadgeCount(count = 0, todo = 0): number {
  return Math.max(0, Number(count) || 0) + Math.max(0, Number(todo) || 0)
}

export function fetchNotifications(tab?: NotificationTab | '', unread?: boolean) {
  const config = authConfig()
  if (!config) return rejectMissingToken()
  return axios.get<{
    code: number
    msg: string
    data: { list: InAppNotification[]; total: number }
  }>(BASE, {
    ...config,
    params: {
      ...(tab ? { tab } : {}),
      ...(unread ? { unread: 1 } : {})
    }
  })
}

export function fetchNotificationUnreadCount() {
  const config = authConfig()
  if (!config) return rejectMissingToken()
  return axios.get<{ code: number; msg: string; data: NotificationUnreadCount }>(
    `${BASE}/unread-count`,
    config
  )
}

export function markNotificationRead(id: number) {
  const config = authConfig()
  if (!config) return rejectMissingToken()
  return axios.post<{ code: number; msg: string }>(`${BASE}/${id}/read`, {}, config)
}

export function markAllNotificationsRead() {
  const config = authConfig()
  if (!config) return rejectMissingToken()
  return axios.post<{ code: number; msg: string }>(`${BASE}/read-all`, {}, config)
}

export function notificationDefaultLink(tab: NotificationTab): string {
  const path = typeof window === 'undefined' ? '' : window.location.pathname
  if (isPanelPath(path, '/developer-panel')) {
    return '/developer-panel/dashboard'
  }
  if (isPanelPath(path, '/agent-panel')) {
    return tab === 'todo' ? '/agent-panel/tickets' : '/agent-panel/dashboard'
  }
  if (isPanelPath(path, '/user')) {
    return tab === 'todo' ? '/user/tickets' : '/user/licenses'
  }
  if (tab === 'todo' || tab === 'notice') return '/source-station/applications'
  if (tab === 'message') return '/source-station/catalog'
  return '/tickets'
}
