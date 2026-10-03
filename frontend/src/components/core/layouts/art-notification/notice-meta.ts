/**
 * 站内通知的纯函数：图标配色、是否有可前往的链接、通知中心地址、时间格式。
 * 顶栏铃铛和通知中心页共用，不依赖 Vue，方便单测。
 */
export type NoticeTab = 'notice' | 'message' | 'todo'

const NOTICE_TABS: NoticeTab[] = ['notice', 'message', 'todo']

interface NoticeStyle {
  icon: string
  iconClass: string
}

/** 安全类（改密、到期、登录异常）用危险色加锁/盾图标；不用黄色、橙色。 */
const STYLES: Record<string, NoticeStyle> = {
  security: { icon: 'ri:shield-keyhole-line', iconClass: 'bg-danger/12 text-danger' },
  review: { icon: 'ri:user-add-line', iconClass: 'bg-theme/12 text-theme' },
  rejected: { icon: 'ri:close-circle-line', iconClass: 'bg-danger/12 text-danger' },
  done: { icon: 'ri:checkbox-circle-line', iconClass: 'bg-success/12 text-success' },
  message: { icon: 'ri:message-3-line', iconClass: 'bg-success/12 text-success' },
  todo: { icon: 'ri:task-line', iconClass: 'bg-theme/12 text-theme' },
  notice: { icon: 'ri:notification-3-line', iconClass: 'bg-theme/12 text-theme' }
}

function noticeKind(eventType = '', tab: NoticeTab = 'notice'): keyof typeof STYLES {
  const e = eventType.toLowerCase()
  if (/password|security|expir|login|lock/.test(e)) return 'security'
  if (/reject|deprecat|cancel/.test(e)) return 'rejected'
  if (/approved|paid|success/.test(e)) return 'done'
  if (/apply|ticket|review|submit/.test(e)) return 'review'
  if (tab === 'todo' || e.startsWith('todo_')) return 'todo'
  if (tab === 'message') return 'message'
  return 'notice'
}

export function noticeStyle(eventType = '', tab: NoticeTab = 'notice'): NoticeStyle {
  return STYLES[noticeKind(eventType, tab)]
}

/** 空链接和「/」（首页）都不算可处理的去处，不显示「前往处理」。 */
export function hasActionLink(link?: string | null): boolean {
  const value = (link || '').trim()
  return value !== '' && value !== '/'
}

export function normalizeNoticeTab(raw: unknown): NoticeTab {
  const value = Array.isArray(raw) ? raw[0] : raw
  return NOTICE_TABS.includes(value as NoticeTab) ? (value as NoticeTab) : 'notice'
}

function inPanel(path: string, prefix: string): boolean {
  return path === prefix || path.startsWith(`${prefix}/`)
}

/** 通知中心在四端各有一个地址；按当前所在端返回，带上要打开的标签。 */
export function notificationCenterRoute(
  pathname: string,
  tab: NoticeTab
): { path: string; query: { tab: NoticeTab } } {
  let path = '/system/notifications'
  if (inPanel(pathname, '/developer-panel')) path = '/developer-panel/notifications'
  else if (inPanel(pathname, '/agent-panel')) path = '/agent-panel/notifications'
  else if (inPanel(pathname, '/user')) path = '/user/notifications'
  return { path, query: { tab } }
}

export function formatNoticeTime(raw?: string): string {
  if (!raw) return ''
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return raw
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}
