/**
 * 用户端 / 代理端 / 开发者端侧栏开关。
 *
 * 窄屏用抽屉（navOpen），宽屏用宽度折叠（collapsed）。
 * 遮罩、路由切换、关闭按钮和 Esc 只收起抽屉，不改宽屏折叠状态。
 */

export const PANEL_MOBILE_MAX_WIDTH = 768

export interface PanelNavState {
  isMobile: boolean
  navOpen: boolean
  collapsed: boolean
}

export type PanelNavEvent = 'toggle' | 'close' | 'mask' | 'route' | 'escape'

export function reducePanelNav(state: PanelNavState, event: PanelNavEvent): PanelNavState {
  if (state.isMobile) {
    if (event === 'toggle') {
      return { ...state, navOpen: !state.navOpen }
    }
    return { ...state, navOpen: false }
  }

  if (event === 'toggle') {
    return { ...state, collapsed: !state.collapsed, navOpen: false }
  }

  return { ...state, navOpen: false }
}

/** 进出窄屏时收起抽屉，避免侧栏停在盖住内容的状态。 */
export function syncPanelNavToViewport(state: PanelNavState, isMobile: boolean): PanelNavState {
  if (state.isMobile === isMobile) return state
  return { ...state, isMobile, navOpen: false }
}

export function panelNavExpanded(state: PanelNavState): boolean {
  return state.isMobile ? state.navOpen : !state.collapsed
}

export function panelToggleIcon(state: PanelNavState): string {
  return panelNavExpanded(state) ? 'ri:menu-fold-line' : 'ri:menu-unfold-line'
}

export function panelToggleLabel(state: PanelNavState): string {
  if (state.isMobile) return state.navOpen ? '关闭导航' : '打开导航'
  return state.collapsed ? '展开导航' : '收起导航'
}
