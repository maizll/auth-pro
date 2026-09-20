import { AppRouteRecord } from '@/types/router'

export const agentRoutes: AppRouteRecord = {
  path: '/admin/agent',
  name: 'Agent',
  component: '/index/index',
  meta: {
    title: 'menus.agent.title',
    icon: 'ri:team-line',
    roles: ['R_SUPER', 'R_ADMIN']
  },
  children: [
    {
      path: 'list',
      name: 'AgentList',
      component: '/agent/list',
      meta: {
        title: 'menus.agent.list',
        icon: 'ri:user-star-line',
        keepAlive: true
      }
    },
    {
      path: 'level',
      name: 'AgentLevel',
      component: '/agent/level',
      meta: {
        title: 'menus.agent.level',
        icon: 'ri:vip-crown-line',
        keepAlive: true
      }
    },
    {
      path: 'quota',
      name: 'AgentQuota',
      component: '/agent/quota',
      meta: {
        title: 'menus.agent.quota',
        icon: 'ri:key-2-line',
        keepAlive: true
      }
    },
    {
      path: 'recharge',
      name: 'AgentRecharge',
      component: '/agent/recharge',
      meta: {
        title: 'menus.agent.finance',
        icon: 'ri:money-cny-circle-line',
        keepAlive: true
      }
    },
    {
      path: 'upgrade',
      name: 'AgentUpgrade',
      component: '/agent/upgrade',
      meta: {
        title: 'menus.agent.upgrade',
        icon: 'ri:user-shared-line',
        keepAlive: true
      }
    }
  ]
}
