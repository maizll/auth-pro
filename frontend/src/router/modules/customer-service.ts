import { AppRouteRecord } from '@/types/router'
import { userManageRoutes } from './user-manage'
import { orderListRoutes } from './order-list'
import { promotionCampaignRoutes } from './promotion-campaigns'
import { ticketRoutes } from './ticket'

export const customerServiceRoutes: AppRouteRecord = {
  path: '/customer-service',
  name: 'CustomerService',
  component: '/index/index',
  redirect: '/user-manage',
  meta: {
    title: 'menus.customerService.title',
    icon: 'ri:customer-service-2-line',
    roles: ['R_SUPER', 'R_ADMIN']
  },
  children: [userManageRoutes, orderListRoutes, promotionCampaignRoutes, ticketRoutes]
}
