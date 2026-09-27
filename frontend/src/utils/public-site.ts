// 官网顶栏和登录入口。五个内置页面由宿主提供，模板只改外观。
import axios from 'axios'
import { onMounted, ref } from 'vue'
import type { Router } from 'vue-router'

export interface PublicNavItem {
  key: string
  label: string
  href: string
  external: boolean
}

const defaultPublicNav: PublicNavItem[] = [
  { key: 'home', label: '首页', href: '/user/login', external: false },
  { key: 'purchase', label: '授权购买', href: '/buy', external: false },
  { key: 'compare', label: '系统对比', href: '/compare', external: false },
  { key: 'docs', label: '系统文档', href: '/docs', external: false },
  { key: 'changelog', label: '更新日志', href: '/changelog', external: false }
]

type AuthMode = 'login' | 'register'
type AuthOpener = (mode: AuthMode) => void

let authOpener: AuthOpener | null = null

/** 首页模板挂上现有登录框。离开页面时调用返回的函数卸掉。 */
export function registerSiteAuthOpener(opener: AuthOpener) {
  authOpener = opener
  return () => {
    if (authOpener === opener) authOpener = null
  }
}

/** 打开首页登录或注册框。当前不在首页时先回到首页再打开。 */
export function openSiteAuth(mode: AuthMode, router?: Router) {
  if (authOpener) {
    authOpener(mode)
    return
  }
  const target = { path: '/user/login', query: { auth: mode } }
  if (router) {
    void router.push(target)
    return
  }
  window.location.assign(`/user/login?auth=${mode}`)
}

export function consumeSiteAuthQuery(auth: unknown, clear: () => void, open: AuthOpener) {
  if (auth !== 'login' && auth !== 'register') return
  open(auth)
  clear()
}

export function siteUserLoggedIn() {
  return typeof localStorage !== 'undefined' && Boolean(localStorage.getItem('user_panel_token'))
}

export function usePublicNav() {
  const items = ref<PublicNavItem[]>(defaultPublicNav.map((item) => ({ ...item })))

  async function load() {
    try {
      const { data } = await axios.get('/api/v1/site/nav', { timeout: 8000 })
      const list = data?.data?.list
      if (data?.code !== 200 || !Array.isArray(list)) return
      const next: PublicNavItem[] = []
      for (const row of list) {
        const label = String(row?.label || '').trim()
        const href = String(row?.href || '').trim()
        if (!label || !href) continue
        const external = row?.kind === 'external'
        if (!external && !href.startsWith('/')) continue
        if (external && !/^https?:\/\//i.test(href)) continue
        next.push({
          key: String(row?.builtinKey || row?.id || href),
          label,
          href,
          external
        })
      }
      items.value = next
    } catch {
      // 接口不可用时保留内置五项，避免手机上又变成没有导航。
    }
  }

  onMounted(load)
  return { items, reload: load }
}

export function navItemActive(path: string, item: PublicNavItem) {
  if (item.external) return false
  if (item.href === '/user/login') return path === '/user/login' || path === '/'
  if (item.href === '/docs') return path === '/docs' || path.startsWith('/docs/')
  return path === item.href
}
