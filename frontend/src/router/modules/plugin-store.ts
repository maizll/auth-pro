import { AppRouteRecord } from '@/types/router'

export const pluginStoreRoutes: AppRouteRecord = {
  path: '/plugin-store',
  name: 'PluginStore',
  component: '/plugin-store/index',
  meta: {
    title: 'menus.integration.store',
    icon: 'ri:store-2-line',
    keepAlive: true,
    roles: ['R_SUPER']
  }
}
