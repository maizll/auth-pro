import { AppRouteRecord } from '@/types/router'

export const adsPromoteRoutes: AppRouteRecord = {
  path: '/ads-promote',
  name: 'AdsPromote',
  component: '/ads-promote/index',
  meta: {
    title: '推广投放',
    icon: 'ri:megaphone-line',
    keepAlive: true,
    roles: ['R_SUPER', 'R_ADMIN']
  }
}
