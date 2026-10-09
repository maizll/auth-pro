import assert from 'node:assert/strict'
import {
  formatNoticeTime,
  hasActionLink,
  normalizeNoticeTab,
  notificationCenterRoute,
  noticeStyle
} from './notice-meta'

// 「前往处理」只在链接有意义时出现：空串和首页「/」都不算
assert.equal(hasActionLink(''), false)
assert.equal(hasActionLink('   '), false)
assert.equal(hasActionLink('/'), false)
assert.equal(hasActionLink(undefined), false)
assert.equal(hasActionLink('/system/user-center'), true)
assert.equal(hasActionLink('/user/tickets'), true)

// 改密、到期等安全类：危险色 + 锁/盾图标，不能是黄色/橙色
for (const event of ['password_changed', 'license_expiring', 'license_expired', 'security_alert']) {
  const style = noticeStyle(event)
  assert.match(style.iconClass, /danger/, `${event} 应使用危险色`)
  assert.match(style.icon, /shield|lock/, `${event} 应使用锁或盾图标`)
}
for (const event of ['password_changed', 'order_paid', 'ticket_created', 'plugin_rejected', 'x']) {
  for (const tab of ['notice', 'message', 'todo'] as const) {
    assert.doesNotMatch(noticeStyle(event, tab).iconClass, /warning|orange|yellow|gold/)
  }
}
assert.match(noticeStyle('order_paid', 'message').iconClass, /success/)

// 「查看全部」按所在端进入各自的通知中心，并带上当前标签
assert.deepEqual(notificationCenterRoute('/dashboard/console', 'todo'), {
  path: '/system/notifications',
  query: { tab: 'todo' }
})
assert.equal(notificationCenterRoute('/user/licenses', 'notice').path, '/user/notifications')
assert.equal(notificationCenterRoute('/user', 'notice').path, '/user/notifications')
assert.equal(notificationCenterRoute('/user-manage', 'notice').path, '/system/notifications')
assert.equal(
  notificationCenterRoute('/agent-panel/finance', 'message').path,
  '/agent-panel/notifications'
)
assert.equal(
  notificationCenterRoute('/developer-panel/plugins', 'message').path,
  '/developer-panel/notifications'
)

assert.equal(normalizeNoticeTab('message'), 'message')
assert.equal(normalizeNoticeTab(['todo']), 'todo')
assert.equal(normalizeNoticeTab('bogus'), 'notice')
assert.equal(normalizeNoticeTab(undefined), 'notice')

assert.equal(formatNoticeTime(''), '')
assert.equal(formatNoticeTime('not-a-date'), 'not-a-date')
assert.match(formatNoticeTime('2026-10-03T12:30:00Z'), /^2026-10-03 \d{2}:30$/)

console.log('notice-meta tests passed')
