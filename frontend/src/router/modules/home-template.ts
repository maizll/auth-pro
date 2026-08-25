import { AppRouteRecord } from '@/types/router'

export const homeTemplateRoutes: AppRouteRecord = {
  path: '/home-template',
  name: 'HomeTemplate',
  component: '/home-template/index',
  meta: {
    title: 'menus.homeTemplate',
    icon: 'ri:layout-4-line',
    keepAlive: true,
    roles: ['R_SUPER']
  }
}
