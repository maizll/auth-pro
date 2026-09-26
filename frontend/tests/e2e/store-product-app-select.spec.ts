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
  redirect: '/source-station/settings',
  meta: {
    title: 'menus.sourceStation.title',
    icon: 'ri:database-2-line',
    roles: ['R_SUPER', 'R_ADMIN']
  },
  children: [
    {
      name: 'SourceStationSettings',
      path: 'settings',
      component: '/source-station/settings',
      meta: { title: 'menus.sourceStation.settings', icon: 'ri:settings-3-line', keepAlive: true }
    }
  ]
}

async function mockSettingsAPIs(page: Page) {
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
      await route.fulfill({ status: 200, json: { code: 200, msg: '', data: [sourceStationMenu] } })
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
    if (pathname === '/api/app/list') {
      await route.fulfill({
        status: 200,
        json: {
          code: 200,
          msg: '',
          data: [
            { id: 1, name: '正式产品', appKey: 'good-app', enabled: true },
            { id: 2, name: '已停用', appKey: 'old-app', enabled: false }
          ]
        }
      })
      return
    }
    if (pathname === '/api/v1/source/admin/settings/store') {
      await route.fulfill({
        status: 200,
        json: {
          code: 200,
          msg: '',
          data: {
            productAppKey: 'orphan-app',
            freePlanId: '',
            graceDays: 7,
            revokeOnPasswordChange: true,
            commercialFeatures: ['multi_app']
          }
        }
      })
      return
    }
    if (pathname === '/api/v1/source/admin/settings/release') {
      await route.fulfill({
        status: 200,
        json: {
          code: 200,
          msg: '',
          data: {
            provider: 'github',
            owner: '',
            repo: '',
            tagStrategy: '{id}-{version}',
            branch: 'master',
            hasToken: false,
            tokenMasked: '',
            configured: false
          }
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

test('产品应用标识从已启用应用中选择', async ({ page }, testInfo) => {
  await mockSettingsAPIs(page)
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
  await page.goto('/source-station/settings')

  await expect(page.getByText('从已启用的应用里选择产品应用标识')).toBeVisible()
  const select = page.locator('.product-app-select')
  await expect(select).toBeVisible()
  await select.click()
  const dropdown = page.locator('.el-select-dropdown:visible')
  await expect(dropdown.getByText('正式产品（good-app）')).toBeVisible()
  await expect(dropdown.getByText('orphan-app（未启用或不存在）')).toBeVisible()
  await expect(dropdown.getByText('已停用（old-app）')).toHaveCount(0)
  await dropdown.getByText('正式产品（good-app）').click()
  await expect(select).toContainText('正式产品（good-app）')

  await page.locator('.store-card').screenshot({
    path: testInfo.outputPath('store-product-app-select.png')
  })
})
