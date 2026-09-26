/**
 * 路由工具函数
 *
 * 提供路由相关的工具函数
 *
 * @module utils/router
 */
import { RouteLocationNormalized, RouteRecordRaw } from 'vue-router'
import NProgress from 'nprogress'
import 'nprogress/nprogress.css'
import { useSystemConfigStore } from '@/store/modules/system-config'
import { resolveMenuTitle } from '@/utils/form/menu-title'

/** 扩展的路由配置类型 */
export type AppRouteRecordRaw = RouteRecordRaw & {
  hidden?: boolean
}

/** 顶部进度条配置 */
export const configureNProgress = () => {
  NProgress.configure({
    easing: 'ease',
    speed: 600,
    showSpinner: false,
    parent: 'body'
  })
}

/**
 * 设置页面标题，根据路由元信息和系统信息拼接标题
 * @param to 当前路由对象
 */
export const setPageTitle = (to: RouteLocationNormalized): void => {
  const siteName = useSystemConfigStore().siteName
  if (to.path === '/user' || to.path.startsWith('/user/')) {
    document.title = siteName
    return
  }

  const { title } = to.meta
  document.title = title ? `${formatMenuTitle(String(title))} - ${siteName}` : siteName
}

/**
 * 格式化菜单标题
 * @param title 菜单标题，可以是 i18n 的 key，也可以是字符串
 * @returns 格式化后的菜单标题
 */
export const formatMenuTitle = (title: string): string => resolveMenuTitle(title)
