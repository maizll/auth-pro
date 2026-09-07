import { AppRouteRecord } from '@/types/router'

export const ticketRoutes: AppRouteRecord = {
  path: '/tickets',
  name: 'TicketManage',
  component: '/system/tickets',
  meta: {
    title: '工单管理',
    icon: 'ri:customer-service-2-line',
    keepAlive: true,
    roles: ['R_SUPER', 'R_ADMIN']
  }
}
