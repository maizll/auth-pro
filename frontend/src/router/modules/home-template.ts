import { AppRouteRecord } from '@/types/router'

/** 首页模板已并入应用商店 / 软件目录，保留路由文件但不在侧栏导出。 */
export const homeTemplateRoutes: AppRouteRecord = {
  path: '/home-template',
  name: 'HomeTemplate',
  component: '/home-template/index',
  meta: {
    title: 'menus.homeTemplate',
    icon: 'ri:layout-4-line',
    keepAlive: true,
    isHide: true,
    roles: ['R_SUPER']
  }
}
