import { AppRouteRecord } from '@/types/router'

export const piracyRoutes: AppRouteRecord = {
  path: '/piracy',
  name: 'Piracy',
  component: '/index/index',
  meta: {
    title: 'menus.security.title',
    icon: 'ri:shield-flash-line',
    roles: ['R_SUPER', 'R_ADMIN']
  },
  children: [
    {
      path: 'tracking',
      name: 'PiracyTracking',
      component: '/piracy/tracking',
      meta: {
        title: 'menus.security.tracking',
        icon: 'ri:spy-line',
        keepAlive: true
      }
    },
    {
      path: 'blacklist',
      name: 'PiracyBlacklist',
      component: '/piracy/blacklist',
      meta: {
        title: 'menus.security.blacklist',
        icon: 'ri:forbid-line',
        keepAlive: true
      }
    },
    {
      path: 'alerts',
      name: 'PiracyAlerts',
      component: '/piracy/alerts',
      meta: {
        title: 'menus.security.alerts',
        icon: 'ri:alarm-warning-line',
        keepAlive: false
      }
    },
    {
      path: 'reports',
      name: 'PiracyReports',
      component: '/piracy/reports',
      meta: {
        title: 'menus.security.reports',
        icon: 'ri:bar-chart-box-line',
        keepAlive: false
      }
    }
  ]
}
