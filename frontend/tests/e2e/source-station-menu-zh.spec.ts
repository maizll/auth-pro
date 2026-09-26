import { readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test, type Page, type Route } from '@playwright/test'

const productVersion = readFileSync(
  path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../../VERSION'),
  'utf8'
).trim()

const sourceStationMenu = {
  name: 'SourceStation',
  path: '/source-station',
  component: '/index/index',
  redirect: '/source-station/packages',
  meta: {
    title: 'menus.sourceStation.title',
    icon: 'ri:database-2-line',
    roles: ['R_SUPER', 'R_ADMIN']
  },
  children: [
    {
      name: 'SourceStationPackages',
      path: 'packages',
      component: '/source-station/packages',
      meta: { title: 'menus.sourceStation.packages', icon: 'ri:apps-2-line', keepAlive: true }
    },
    {
      name: 'SourceStationApplications',
      path: 'applications',
      component: '/source-station/applications',
      meta: { title: 'menus.sourceStation.applications', icon: 'ri:user-add-line', keepAlive: true }
    },
    {
      name: 'SourceStationCatalog',
      path: 'catalog',
      component: '/source-station/catalog',
      meta: { title: 'menus.sourceStation.catalog', icon: 'ri:file-list-3-line' }
    },
    {
      name: 'SourceStationAds',
      path: 'ads',
      component: '/source-station/ads',
      meta: { title: 'menus.sourceStation.ads', icon: 'ri:advertisement-line', keepAlive: true }
    },
    {
      name: 'SourceStationSettings',
      path: 'settings',
      component: '/source-station/settings',
      meta: { title: 'menus.sourceStation.settings', icon: 'ri:settings-3-line', keepAlive: true }
    },
    {
      name: 'SourceStationEdition',
      path: 'edition',
      component: '/source-station/edition',
      meta: { title: 'menus.sourceStation.edition', icon: 'ri:vip-crown-line', keepAlive: true }
    },
    {
      name: 'SourceStationStoreOrders',
      path: 'store-orders',
      component: '/source-station/store-orders',
      meta: { title: 'menus.sourceStation.storeOrders', icon: 'ri:bill-line', keepAlive: true }
    },
    {
      name: 'SourceStationStoreLicenses',
      path: 'store-licenses',
      component: '/source-station/store-licenses',
      meta: { title: 'menus.sourceStation.storeLicenses', icon: 'ri:key-2-line', keepAlive: true }
    },
    {
      name: 'SourceStationStoreRevenue',
      path: 'store-revenue',
      component: '/source-station/store-revenue',
      meta: { title: 'menus.sourceStation.storeRevenue', icon: 'ri:money-cny-circle-line', keepAlive: true }
    }
  ]
}

async function mockAdminAPIs(page: Page) {
  await page.route(/^https?:\/\/[^/]+\/api\//, async (route: Route) => {
    const { pathname } = new URL(route.request().url())
    if (pathname === '/api/install/status') {
      await route.fulfill({ status: 200, json: { installed: true } })
      return
    }
    if (pathname === '/api/user/info') {
      await route.fulfill({
        status: 200,
        json: {
          code: 200,
          msg: '',
          data: {
            userId: 1,
            userName: 'admin',
            nickname: '管理员',
            email: 'admin@example.com',
            roles: ['R_SUPER'],
            buttons: []
          }
        }
      })
      return
    }
    if (pathname === '/api/system/menus') {
      await route.fulfill({
        status: 200,
        json: { code: 200, msg: '', data: [sourceStationMenu] }
      })
      return
    }
    if (pathname === '/api/system-config/public') {
      await route.fulfill({
        status: 200,
        json: {
          code: 200,
          msg: '',
          data: { siteName: '授权管理系统', siteSubtitle: '', siteLogo: '', registrationEnabled: true }
        }
      })
      return
    }
    if (pathname.includes('/notifications')) {
      await route.fulfill({
        status: 200,
        json: { code: 200, msg: '', data: { list: [], total: 0, count: 0, unread: 0 } }
      })
      return
    }
    await route.fulfill({ status: 200, json: { code: 200, msg: '', data: {} } })
  })
}

test('源站运营侧栏展开后显示中文', async ({ page }, testInfo) => {
  await mockAdminAPIs(page)
  await page.addInitScript((version) => {
    localStorage.setItem(
      `sys-v${version}-user`,
      JSON.stringify({
        language: 'zh',
        isLogin: true,
        isLock: false,
        lockPassword: '',
        info: {
          userId: 1,
          userName: 'admin',
          nickname: '管理员',
          email: 'admin@example.com',
          roles: ['R_SUPER'],
          buttons: []
        },
        searchHistory: [],
        accessToken: 'e2e-admin-token',
        refreshToken: 'e2e-admin-refresh'
      })
    )
  }, productVersion)
  await page.setViewportSize({ width: 1440, height: 900 })
  await page.goto('/source-station/edition')

  const sidebar = page.locator('.layout-sidebar')
  await expect(sidebar.getByText('源站运营', { exact: true })).toBeVisible()
  for (const label of ['商业版设置', '商店订单', '主授权与权益', '商业版收入']) {
    await expect(sidebar.getByText(label, { exact: true })).toBeVisible()
  }
  await expect(sidebar.getByText('edition', { exact: true })).toHaveCount(0)
  await expect(sidebar.getByText('storeOrders', { exact: true })).toHaveCount(0)

  await sidebar.screenshot({ path: testInfo.outputPath('source-station-menu-zh.png') })
})
