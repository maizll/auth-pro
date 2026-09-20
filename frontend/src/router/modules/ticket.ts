import { AppRouteRecord } from '@/types/router'

export const ticketRoutes: AppRouteRecord = {
  path: '/tickets',
  name: 'TicketManage',
  component: '/system/tickets',
  meta: {
    title: 'menus.customerService.tickets',
    icon: 'ri:question-answer-line',
    keepAlive: true,
    roles: ['R_SUPER', 'R_ADMIN']
  }
}
