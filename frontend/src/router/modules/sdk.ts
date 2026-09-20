import { AppRouteRecord } from '@/types/router'
import { pluginStoreRoutes } from './plugin-store'
import { onlineUpdateRoutes } from './online-update'

export const sdkRoutes: AppRouteRecord = {
  path: '/sdk',
  name: 'Sdk',
  component: '/index/index',
  meta: {
    title: 'menus.integration.title',
    icon: 'ri:code-box-line',
    roles: ['R_SUPER', 'R_ADMIN']
  },
  children: [
    {
      path: 'index',
      name: 'SdkIndex',
      component: '/sdk/index',
      meta: {
        title: 'menus.integration.sdk',
        icon: 'ri:code-s-slash-line',
        keepAlive: true,
        roles: ['R_SUPER', 'R_ADMIN']
      }
    },
    {
      path: 'developer-doc',
      name: 'DeveloperDoc',
      component: '/sdk/developer-doc',
      meta: {
        title: 'menus.integration.docs',
        icon: 'ri:file-code-line',
        keepAlive: true,
        roles: ['R_SUPER', 'R_ADMIN']
      }
    },
    {
      path: 'default-home-template',
      name: 'DefaultHomeTemplateDoc',
      component: '/sdk/default-home-template-doc',
      meta: {
        title: 'menus.integration.templateDoc',
        icon: 'ri:layout-4-line',
        keepAlive: true,
        roles: ['R_SUPER', 'R_ADMIN']
      }
    },
    pluginStoreRoutes,
    onlineUpdateRoutes
  ]
}
