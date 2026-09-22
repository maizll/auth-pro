import assert from 'node:assert/strict'
import {
  panelNavExpanded,
  panelToggleIcon,
  panelToggleLabel,
  reducePanelNav,
  syncPanelNavToViewport,
  type PanelNavState
} from './panelNavState'

const mobileClosed: PanelNavState = { isMobile: true, navOpen: false, collapsed: false }
const mobileOpen: PanelNavState = { isMobile: true, navOpen: true, collapsed: true }
const desktopOpen: PanelNavState = { isMobile: false, navOpen: false, collapsed: false }
const desktopCollapsed: PanelNavState = { isMobile: false, navOpen: false, collapsed: true }

const opened = reducePanelNav(mobileClosed, 'toggle')
assert.equal(opened.navOpen, true, '窄屏汉堡应打开抽屉')
assert.equal(opened.collapsed, false, '窄屏开关不应改宽屏折叠状态')

for (const event of ['toggle', 'mask', 'route', 'close', 'escape'] as const) {
  const closed = reducePanelNav(mobileOpen, event)
  assert.equal(closed.navOpen, false, `窄屏 ${event} 应关闭抽屉`)
  assert.equal(closed.collapsed, true, `窄屏 ${event} 不应改动 collapsed`)
}

const collapsed = reducePanelNav(desktopOpen, 'toggle')
assert.equal(collapsed.collapsed, true, '宽屏汉堡应收起侧栏宽度')
assert.equal(collapsed.navOpen, false)

const expanded = reducePanelNav(desktopCollapsed, 'toggle')
assert.equal(expanded.collapsed, false, '宽屏再次点击应展开侧栏')

for (const event of ['mask', 'route', 'close', 'escape'] as const) {
  const next = reducePanelNav(desktopCollapsed, event)
  assert.equal(next.collapsed, true, `宽屏 ${event} 应保持折叠`)
  assert.equal(next.navOpen, false)
}

const leaveMobile = syncPanelNavToViewport(mobileOpen, false)
assert.equal(leaveMobile.isMobile, false)
assert.equal(leaveMobile.navOpen, false, '离开窄屏应关掉抽屉')
assert.equal(leaveMobile.collapsed, true, '离开窄屏应保留宽屏折叠')

const enterMobile = syncPanelNavToViewport(
  { isMobile: false, navOpen: true, collapsed: true },
  true
)
assert.equal(enterMobile.isMobile, true)
assert.equal(enterMobile.navOpen, false, '进入窄屏时抽屉默认关闭')
assert.equal(enterMobile.collapsed, true)

assert.equal(syncPanelNavToViewport(mobileOpen, true), mobileOpen)

assert.equal(panelNavExpanded(mobileOpen), true)
assert.equal(panelNavExpanded(mobileClosed), false)
assert.equal(panelNavExpanded(desktopCollapsed), false)
assert.equal(panelToggleIcon(mobileClosed), 'ri:menu-unfold-line')
assert.equal(panelToggleIcon(mobileOpen), 'ri:menu-fold-line')
assert.equal(panelToggleLabel(mobileOpen), '关闭导航')
assert.equal(panelToggleLabel(mobileClosed), '打开导航')
assert.equal(panelToggleLabel(desktopCollapsed), '展开导航')
