import { readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test, type Page, type Route } from '@playwright/test'

const productVersion = readFileSync(
  path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../../VERSION'),
  'utf8'
).trim()

const menus = [
  {
    name: 'License',
    path: '/license',
    component: '/index/index',
    redirect: '/license/apps',
    meta: { title: 'menus.license.title', icon: 'ri:apps-line', roles: ['R_SUPER'] },
    children: [
      {
        name: 'LicenseApps',
        path: 'apps',
        component: '/license/apps',
        meta: { title: 'menus.license.apps', icon: 'ri:apps-2-line', keepAlive: true }
      },
      {
        name: 'LicensePlans',
        path: 'plans',
        component: '/license/plans',
        meta: { title: 'menus.license.plans', icon: 'ri:price-tag-3-line', keepAlive: true }
      },
      {
        name: 'LicenseList',
        path: 'list',
        component: '/license/list',
        meta: { title: 'menus.license.list', icon: 'ri:file-list-3-line', keepAlive: true }
      }
    ]
  },
  {
    name: 'CustomerService',
    path: '/customer-service',
    component: '/index/index',
    redirect: '/order-list',
    meta: { title: 'menus.customerService.title', icon: 'ri:customer-service-2-line', roles: ['R_SUPER'] },
    children: [
      {
        name: 'OrderList',
        path: '/order-list',
        component: '/system/payment-orders',
        meta: { title: 'menus.customerService.orders', icon: 'ri:file-list-3-line', keepAlive: true }
      }
    ]
  },
  {
    name: 'SourceStation',
    path: '/source-station',
    component: '/index/index',
    redirect: '/source-station/packages',
    meta: { title: 'menus.sourceStation.title', icon: 'ri:database-2-line', roles: ['R_SUPER'] },
    children: [
      {
        name: 'SourceStationPackages',
        path: 'packages',
        component: '/source-station/packages',
        meta: { title: 'menus.sourceStation.packages', icon: 'ri:apps-2-line' }
      },
      {
        name: 'SourceStationSettings',
        path: 'settings',
        component: '/source-station/settings',
        meta: { title: 'menus.sourceStation.settings', icon: 'ri:settings-3-line' }
      }
    ]
  }
]

const appRow = {
  id: 1,
  name: '商城系统',
  appKey: 'shop',
  appSecret: 'sk_live_demo',
  purchaseLicenseTypes: ['domain'],
  licenseCount: 2,
  recentVersion: '1.5.9',
  versionCount: 1,
  enabled: true,
  licenseRequired: true,
  commercialProduct: true,
  saleGaps: [{ code: 'plan', label: '没有有价格的套餐', path: '/license/plans' }],
  graceDays: 7,
  revokeOnPasswordChange: true,
  commercialFeatures: ['multi_app'],
  remark: '',
  createdAt: '2026-09-26 10:00'
}

async function mockAdmin(page: Page) {
  await page.route(/^https?:\/\/[^/]+\/api\//, async (route: Route) => {
    const { pathname } = new URL(route.request().url())
    const ok = (data: unknown) =>
      route.fulfill({ status: 200, json: { code: 200, msg: '', data } })
    if (pathname === '/api/install/status') {
      await route.fulfill({ status: 200, json: { installed: true } })
      return
    }
    if (pathname === '/api/user/info') {
      await ok({
        userId: 1,
        userName: 'admin',
        nickname: '管理员',
        email: 'admin@example.com',
        roles: ['R_SUPER'],
        buttons: []
      })
      return
    }
    if (pathname === '/api/system/menus') {
      await ok(menus)
      return
    }
    if (pathname === '/api/app/list') {
      await ok([appRow])
      return
    }
    if (pathname === '/api/license/apps') {
      await ok([{ id: 1, name: '商城系统' }])
      return
    }
    if (pathname === '/api/plan/list') {
      await ok([
        {
          id: 9,
          appId: 1,
          appName: '商城系统',
          name: '永久商业版',
          licenseType: '',
          durationDays: 0,
          durationText: '永久',
          price: 199,
          maxSites: 0,
          sort: 1,
          enabled: true,
          remark: '',
          createdAt: '2026-09-26 10:00'
        }
      ])
      return
    }
    if (pathname === '/api/license/list') {
      await ok({
        list: [
          {
            id: 3,
            domain: 'shop.example.com',
            appName: '商城系统',
            appId: 1,
            type: 'domain',
            typeLabel: '单域名',
            status: 'active',
            statusLabel: '正常',
            source: 'store_bind',
            sourceLabel: '商店绑定',
            commercialActive: false,
            ownerType: 'user',
            ownerId: 1,
            ownerName: '买家',
            expireAt: '',
            verifyCount: 0,
            boundSites: 0,
            maxSites: 0,
            remark: '',
            createdAt: '2026-09-26 10:00'
          }
        ],
        total: 1
      })
      return
    }
    if (pathname === '/api/system/payment-orders') {
      await ok({
        list: [
          {
            orderNo: 'PP1001',
            subjectType: 'store_edition',
            subjectId: 1,
            subjectName: '买家',
            amount: 199,
            paidAmount: 199,
            payChannel: 'easypay',
            payMethod: 'alipay',
            status: 'paid',
            gatewayTradeNo: '',
            remark: '永久商业版',
            createdAt: '2026-09-26 10:01:00',
            paidAt: '2026-09-26 10:02:00'
          }
        ],
        total: 1,
        page: 1,
        pageSize: 20,
        commercialPaidYuan: 199
      })
      return
    }
    if (pathname === '/api/store/account') {
      await ok({ edition: 'commercial', features: ['multi_app'], domainMismatch: false })
      return
    }
    if (pathname.includes('/notifications')) {
      await ok({ list: [], total: 0, count: 0, unread: 0 })
      return
    }
    await ok({})
  })
}

async function signIn(page: Page) {
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
}

test('应用弹窗可以打开商业版出售开关', async ({ page }, testInfo) => {
  await mockAdmin(page)
  await signIn(page)
  await page.goto('/license/apps')
  await page.getByRole('button', { name: '新增应用' }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByText('作为本站商业版出售')).toBeVisible()
  await dialog
    .locator('.el-form-item')
    .filter({ hasText: '作为本站商业版出售' })
    .locator('.el-switch')
    .click()
  await dialog.getByText('高级设置').click()
  await expect(dialog.getByText('离线宽限天数')).toBeVisible()
  await expect(dialog.getByText('一般不用改')).toBeVisible()
  await dialog.screenshot({ path: testInfo.outputPath('app-commercial-switch.png') })
})

test('套餐管理展示永久价格', async ({ page }, testInfo) => {
  await mockAdmin(page)
  await signIn(page)
  await page.goto('/license/plans')
  await expect(page.getByRole('button', { name: '新增套餐' })).toBeVisible()
  await expect(page.getByText('永久商业版')).toBeVisible()
  await expect(page.getByText('永久').first()).toBeVisible()
  await page.locator('.license-plans-page').screenshot({ path: testInfo.outputPath('license-plans.png') })
})

test('授权列表可以按商店来源筛选', async ({ page }, testInfo) => {
  await mockAdmin(page)
  await signIn(page)
  await page.goto('/license/list')
  await expect(page.getByText('商店绑定')).toBeVisible()
  await page.locator('.license-list-page').screenshot({ path: testInfo.outputPath('license-source.png') })
})

test('订单列表展示商业版订单和合计', async ({ page }, testInfo) => {
  await mockAdmin(page)
  await signIn(page)
  await page.goto('/order-list')
  await expect(page.getByText('商业版').first()).toBeVisible()
  await expect(page.getByText('商业版已支付合计 ¥199.00')).toBeVisible()
  await page.locator('.payment-orders-page').screenshot({ path: testInfo.outputPath('payment-orders.png') })
})
