import { AppRouteRecord } from '@/types/router'

export const licenseRoutes: AppRouteRecord = {
  path: '/license',
  name: 'License',
  component: '/index/index',
  redirect: '/license/apps',
  meta: {
    title: 'menus.license.title',
    icon: 'ri:apps-line',
    roles: ['R_SUPER', 'R_ADMIN']
  },
  children: [
    {
      path: 'apps',
      name: 'LicenseApps',
      component: '/license/apps',
      meta: {
        title: 'menus.license.apps',
        icon: 'ri:apps-2-line',
        keepAlive: true
      }
    },
    {
      path: 'versions',
      name: 'LicenseVersions',
      component: '/license/versions',
      meta: {
        title: 'menus.license.versions',
        icon: 'ri:git-branch-line',
        keepAlive: true
      }
    },
    {
      path: 'apps/:id/versions',
      name: 'AppVersions',
      component: '/license/app-versions',
      meta: {
        title: 'menus.license.versions',
        icon: 'ri:git-branch-line',
        isHide: true,
        keepAlive: false,
        activePath: '/license/versions'
      }
    },
    {
      path: 'plans',
      name: 'LicensePlans',
      component: '/license/plans',
      meta: {
        title: 'menus.license.plans',
        icon: 'ri:price-tag-3-line',
        keepAlive: true
      }
    },
    {
      path: 'cards',
      name: 'LicenseCards',
      component: '/license/cards',
      meta: {
        title: 'menus.license.cards',
        icon: 'ri:coupon-3-line',
        keepAlive: true
      }
    },
    {
      path: 'list',
      name: 'LicenseList',
      component: '/license/list',
      meta: {
        title: 'menus.license.list',
        icon: 'ri:file-list-3-line',
        keepAlive: true
      }
    },
    {
      path: 'logs',
      name: 'LicenseLogs',
      component: '/license/logs',
      meta: {
        title: 'menus.license.logs',
        icon: 'ri:file-text-line',
        keepAlive: true
      }
    },
    {
      path: 'dashboard',
      name: 'LicenseDashboard',
      component: '/license/dashboard',
      meta: {
        title: 'menus.license.overview',
        icon: 'ri:dashboard-line',
        keepAlive: false
      }
    }
  ]
}
