import { AppRouteRecord } from '@/types/router'

export const sourceStationRoutes: AppRouteRecord = {
  path: '/source-station',
  name: 'SourceStation',
  component: '/index/index',
  redirect: '/source-station/plugins',
  meta: {
    title: 'menus.sourceStation.title',
    icon: 'ri:database-2-line',
    roles: ['R_SUPER', 'R_ADMIN']
  },
  children: [
    {
      path: 'applications',
      name: 'SourceStationApplications',
      component: '/source-station/applications',
      meta: {
        title: 'menus.sourceStation.applications',
        icon: 'ri:user-add-line',
        keepAlive: true
      }
    },
    {
      path: 'plugins',
      name: 'SourceStationPlugins',
      component: '/source-station/plugins',
      meta: {
        title: 'menus.sourceStation.plugins',
        icon: 'ri:puzzle-line',
        keepAlive: true
      }
    },
    {
      path: 'templates',
      name: 'SourceStationTemplates',
      component: '/source-station/templates',
      meta: {
        title: 'menus.sourceStation.templates',
        icon: 'ri:layout-4-line',
        keepAlive: true
      }
    },
    {
      path: 'catalog',
      name: 'SourceStationCatalog',
      component: '/source-station/catalog',
      meta: {
        title: 'menus.sourceStation.catalog',
        icon: 'ri:file-list-3-line',
        keepAlive: false
      }
    },
    {
      path: 'ads',
      name: 'SourceStationAds',
      component: '/source-station/ads',
      meta: {
        title: 'menus.sourceStation.ads',
        icon: 'ri:advertisement-line',
        keepAlive: true
      }
    },
    {
      path: 'settings',
      name: 'SourceStationSettings',
      component: '/source-station/settings',
      meta: {
        title: 'menus.sourceStation.settings',
        icon: 'ri:key-2-line',
        keepAlive: true
      }
    }
  ]
}
