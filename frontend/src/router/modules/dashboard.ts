import { AppRouteRecord } from '@/types/router'

export const dashboardRoutes: AppRouteRecord = {
  name: 'Dashboard',
  path: '/dashboard',
  component: '/index/index',
  redirect: '/dashboard/console',
  meta: {
    title: 'menus.dashboard.title',
    icon: 'ri:home-smile-2-line',
    roles: ['R_SUPER', 'R_ADMIN']
  },
  children: [
    {
      path: 'console',
      name: 'Console',
      component: '/dashboard/console',
      meta: {
        title: 'menus.dashboard.console',
        keepAlive: false,
        fixedTab: true,
        isHide: true,
        activePath: '/dashboard'
      }
    }
  ]
}
