import axios from 'axios'
import { AGENT_TOKEN_KEY, DEVELOPER_TOKEN_KEY } from '@/api/source-developer'

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

function panelToken(): string {
  if (typeof window === 'undefined') return ''
  const path = window.location.pathname
  if (path.startsWith('/developer-panel')) {
    return localStorage.getItem(DEVELOPER_TOKEN_KEY) || localStorage.getItem(AGENT_TOKEN_KEY) || ''
  }
  if (path.startsWith('/agent-panel')) {
    return localStorage.getItem(AGENT_TOKEN_KEY) || ''
  }
  if (path.startsWith('/user')) {
    return localStorage.getItem('user_panel_token') || ''
  }
  try {
    const stored = JSON.parse(localStorage.getItem('user') || '{}') as { accessToken?: string }
    return stored.accessToken || ''
  } catch {
    return ''
  }
}

function authConfig() {
  const token = panelToken()
  return token ? { headers: { Authorization: `Bearer ${token}` } } : {}
}

export function fetchNotifications(tab?: NotificationTab | '', unread?: boolean) {
  return axios.get<{
    code: number
    msg: string
    data: { list: InAppNotification[]; total: number }
  }>(BASE, {
    ...authConfig(),
    params: {
      ...(tab ? { tab } : {}),
      ...(unread ? { unread: 1 } : {})
    }
  })
}

export function fetchNotificationUnreadCount() {
  return axios.get<{ code: number; msg: string; data: NotificationUnreadCount }>(
    `${BASE}/unread-count`,
    authConfig()
  )
}

export function markNotificationRead(id: number) {
  return axios.post<{ code: number; msg: string }>(`${BASE}/${id}/read`, {}, authConfig())
}

export function markAllNotificationsRead() {
  return axios.post<{ code: number; msg: string }>(`${BASE}/read-all`, {}, authConfig())
}

export function notificationDefaultLink(tab: NotificationTab): string {
  const path = typeof window === 'undefined' ? '' : window.location.pathname
  if (path.startsWith('/developer-panel')) {
    return '/developer-panel/dashboard'
  }
  if (path.startsWith('/agent-panel')) {
    return tab === 'todo' ? '/agent-panel/tickets' : '/agent-panel/dashboard'
  }
  if (path.startsWith('/user')) {
    return tab === 'todo' ? '/user/tickets' : '/user/licenses'
  }
  if (tab === 'todo' || tab === 'notice') return '/source-station/applications'
  if (tab === 'message') return '/source-station/catalog'
  return '/tickets'
}
