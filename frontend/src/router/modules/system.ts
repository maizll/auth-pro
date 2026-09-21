import { AppRouteRecord } from '@/types/router'

export const systemRoutes: AppRouteRecord = {
  path: '/system',
  name: 'System',
  component: '/index/index',
  meta: {
    title: 'menus.system.title',
    icon: 'ri:settings-3-line',
    roles: ['R_SUPER', 'R_ADMIN']
  },
  children: [
    {
      path: 'role',
      name: 'Role',
      component: '/system/role',
      meta: {
        title: 'menus.system.role',
        icon: 'ri:shield-user-line',
        keepAlive: true,
        roles: ['R_SUPER']
      }
    },
    {
      path: 'menu',
      name: 'Menus',
      component: '/system/menu',
      meta: {
        title: 'menus.system.menu',
        icon: 'ri:menu-2-line',
        keepAlive: true,
        roles: ['R_SUPER'],
        authList: [
          { title: '新增', authMark: 'add' },
          { title: '编辑', authMark: 'edit' },
          { title: '删除', authMark: 'delete' }
        ]
      }
    },
    {
      path: 'config',
      name: 'SystemConfig',
      component: '/system/config',
      meta: {
        title: 'menus.system.config',
        icon: 'ri:settings-3-line',
        keepAlive: true,
        roles: ['R_SUPER']
      }
    },
    {
      path: 'epay-config',
      name: 'EpayConfig',
      component: '/system/epay-config',
      meta: {
        title: 'menus.system.epayConfig',
        icon: 'ri:bank-card-line',
        keepAlive: true,
        roles: ['R_SUPER']
      }
    },
    {
      path: 'alipay-f2f-config',
      name: 'AlipayF2FConfig',
      component: '/system/alipay-f2f-config',
      meta: {
        title: 'menus.system.alipayF2FConfig',
        icon: 'ri:alipay-fill',
        keepAlive: true,
        roles: ['R_SUPER']
      }
    },
    {
      path: 'mail-config',
      name: 'MailConfig',
      component: '/system/mail-config',
      meta: {
        title: 'menus.system.mailConfig',
        icon: 'ri:mail-settings-line',
        keepAlive: true,
        roles: ['R_SUPER']
      }
    },
    {
      path: 'mail-logs',
      name: 'MailLogs',
      component: '/system/mail-logs',
      meta: {
        title: 'menus.system.mailLogs',
        icon: 'ri:mail-check-line',
        keepAlive: true,
        roles: ['R_SUPER']
      }
    },
    {
      path: 'user-center',
      name: 'UserCenter',
      component: '/system/user-center',
      meta: {
        title: 'menus.system.userCenter',
        icon: 'ri:user-settings-line',
        keepAlive: true,
        isHideTab: true
      }
    }
  ]
}
