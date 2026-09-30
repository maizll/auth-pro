import { mkdirSync, readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test, type Page, type Route } from '@playwright/test'

const productVersion = readFileSync(
  path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../../VERSION'),
  'utf8'
).trim()

const shotDir = '/opt/cursor/artifacts/screenshots'
const shots = {
  unbindDesktop: `${shotDir}/screen-unbind-desktop.png`,
  unbindPhone: `${shotDir}/screen-unbind-phone.png`,
  createDesktop: `${shotDir}/screen-create-desktop.png`,
  createPhone: `${shotDir}/screen-create-phone.png`,
  appsDesktop: `${shotDir}/screen-apps-desktop.png`,
  reposDesktop: `${shotDir}/screen-repos-desktop.png`,
  rebindDesktop: `${shotDir}/screen-rebind-desktop.png`,
  rebindPhone: `${shotDir}/screen-rebind-phone.png`,
  badgeDesktop: `${shotDir}/screen-badge-desktop.png`,
  badgePhone: `${shotDir}/screen-badge-phone.png`,
  navDrawerPhone: `${shotDir}/screen-nav-drawer-phone.png`,
  tokenPhone: `${shotDir}/screen-token-phone.png`
}
mkdirSync(shotDir, { recursive: true })

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
      }
    ]
  },
  {
    name: 'SourceStation',
    path: '/source-station',
    component: '/index/index',
    redirect: '/source-station/repos',
    meta: { title: 'menus.sourceStation.title', icon: 'ri:database-2-line', roles: ['R_SUPER'] },
    children: [
      {
        name: 'SourceStationRepos',
        path: 'repos',
        component: '/source-station/repos',
        meta: {
          title: 'menus.sourceStation.repos',
          icon: 'ri:git-repository-line',
          keepAlive: true
        }
      }
    ]
  }
]

const apps = [
  {
    id: 1,
    name: '授权系统',
    appKey: 'app_f93896d80066_5811',
    appSecret: 'sk_live_demo',
    purchaseLicenseTypes: ['domain'],
    licenseCount: 2,
    recentVersion: '1.8.0',
    versionCount: 1,
    enabled: true,
    licenseRequired: true,
    remark: '',
    repo: 'acme/auth-system',
    createdAt: '2026-09-29 10:00'
  },
  {
    id: 2,
    name: '客户门户',
    appKey: 'app_customer',
    appSecret: 'sk_live_demo2',
    purchaseLicenseTypes: ['domain'],
    licenseCount: 0,
    recentVersion: '',
    versionCount: 0,
    enabled: true,
    licenseRequired: true,
    remark: '',
    createdAt: '2026-09-29 11:00'
  }
]

const account = {
  bound: true,
  account: 'owner@example.com',
  role: 'user',
  licenseNo: 'LIC-1800',
  domain: 'shop.example.com',
  requestDomain: 'shop.example.com',
  domainMismatch: false,
  edition: 'commercial',
  editionExpireAt: Math.floor(Date.UTC(2026, 9, 1, 12) / 1000),
  editionSource: '购买',
  permanent: false,
  features: ['multi_app'],
  verifiedAt: 1750000000,
  graceUntil: 0,
  offlineGrace: false,
  graceWarning: false,
  explicitRevoked: false,
  sourceVerified: true,
  reason: '',
  installId: 'e2e'
}

async function mockApis(page: Page, tokenReady: boolean) {
  await page.route(/^https?:\/\/[^/]+\/api\//, async (route: Route) => {
    const { pathname } = new URL(route.request().url())
    const ok = (data: unknown) => route.fulfill({ status: 200, json: { code: 200, msg: '', data } })
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
    if (pathname === '/api/system-config/public' || pathname === '/api/home-template/active') {
      await ok({ siteName: '授权服务', registrationEnabled: true, isDefault: true })
      return
    }
    if (pathname === '/api/app/list') {
      await ok(apps)
      return
    }
    if (pathname === '/api/store/account' || pathname === '/api/store/refresh') {
      await ok(account)
      return
    }
    if (pathname.endsWith('/app-repos/token')) {
      await ok(
        tokenReady
          ? {
              ready: true,
              location: '主存储',
              message: '使用存储管理中的 GitHub 主存储，密钥已保存'
            }
          : {
              ready: false,
              message: '还没有可用的令牌。请到存储管理添加。也可以先创建应用，稍后再绑定。'
            }
      )
      return
    }
    if (pathname.endsWith('/app-repos/suggest')) {
      await ok({ repo: 'acme/customer-portal', tokenReady })
      return
    }
    if (pathname.endsWith('/app-repos/impact')) {
      await ok({ published: 3, drafts: 1, blocked: true, busy: false })
      return
    }
    if (pathname.endsWith('/app-repos/preview')) {
      await ok({
        from: 'acme/auth-system',
        to: 'acme/customer-b',
        groups: [
          { prefix: 'client/', text: '2 个 · 18 MB' },
          { prefix: 'plugins/paid/', text: '4 个 · 6 MB' },
          { prefix: 'plugins/free/', text: '没有文件' },
          { prefix: 'templates/paid/', text: '1 个 · 2 MB' },
          { prefix: 'templates/free/', text: '没有文件' }
        ]
      })
      return
    }
    if (pathname.endsWith('/app-repos')) {
      await ok({
        tokenReady,
        note: '现有安装包还在存储管理的主存储里，不会自动拆给每个应用。',
        list: [
          {
            appId: 1,
            name: '授权系统',
            appKey: 'app_f93896d80066_5811',
            status: 'ready',
            repo: 'acme/auth-system',
            health: '私有，安装包都在'
          },
          {
            appId: 2,
            name: '客户门户',
            appKey: 'app_customer',
            status: 'unbound',
            repo: '',
            health: '尚未绑定'
          },
          ...(tokenReady
            ? []
            : [
                {
                  appId: 3,
                  name: '旧商店',
                  appKey: 'app_old',
                  status: 'degraded',
                  repo: 'acme/old-shop',
                  health: '令牌已失效'
                }
              ])
        ]
      })
      return
    }
    if (pathname.includes('/notifications')) {
      await ok({ list: [], total: 0, count: 0, unread: 0 })
      return
    }
    await ok({})
  })
}

async function login(page: Page) {
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
    localStorage.setItem('user_panel_token', 'e2e-site-user')
  }, productVersion)
}

async function shot(page: Page, file: string) {
  await page.addStyleTag({
    content:
      '#__vue-devtools-container__, .vue-devtools__anchor, [class*="vue-devtools"] { display: none !important; }'
  })
  await page.screenshot({ path: file, animations: 'disabled' })
}

test('仓库绑定和统一弹窗在电脑上可操作', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 900 })
  await mockApis(page, true)
  await login(page)
  await page.goto('/source-station/repos')
  await expect(page.getByText('仓库绑定').first()).toBeVisible()
  await shot(page, shots.reposDesktop)

  await page.getByRole('button', { name: '更换' }).click()
  await expect(page.getByRole('button', { name: '确认更换' })).toBeVisible()
  await expect(page.getByText('只切换')).toBeVisible()
  await shot(page, shots.rebindDesktop)
  await page.keyboard.press('Escape')

  await page.goto('/source-station/repos')
  await page.getByRole('button', { name: '解除' }).click()
  await expect(page.getByText('已发布的版本还依赖这个仓库')).toBeVisible()
  await shot(page, shots.unbindDesktop)

  await page.goto('/license/apps')
  await expect(page.getByRole('columnheader', { name: '仓库' })).toBeVisible()
  await shot(page, shots.appsDesktop)
  await page.getByRole('button', { name: '新增应用' }).click()
  await page.getByPlaceholder('请输入应用名称').fill('Customer Portal')
  await page.getByRole('button', { name: '下一步' }).click()
  await expect(page.getByText('密钥已保存')).toBeVisible()
  await expect(page.getByText('应用信息')).toBeVisible()
  await expect(page.getByPlaceholder('所有者/仓库')).toBeVisible()
  await shot(page, shots.createDesktop)

  await page.goto('/license/apps')
  await expect(page.getByText('2026-10-01 到期')).toBeVisible()
  await page.locator('.commercial-header-entry__hit').first().click()
  await expect(page.locator('.commercial-header-entry__pop').getByText('到期时间')).toBeVisible()
  await expect(page.locator('.commercial-header-entry__pop').getByText('授权域名')).toBeVisible()
  await shot(page, shots.badgeDesktop)
})

test('仓库绑定和官网导航在手机上可操作', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await mockApis(page, false)
  await login(page)
  await page.goto('/source-station/repos')
  await expect(page.getByText('令牌已失效').first()).toBeVisible()
  await shot(page, shots.tokenPhone)

  await page.getByRole('button', { name: '更换' }).first().click()
  await expect(page.getByRole('button', { name: '确认更换' })).toBeVisible()
  await shot(page, shots.rebindPhone)
  await page.keyboard.press('Escape')

  await page.getByRole('button', { name: '解除' }).first().click()
  await expect(page.getByText('已发布的版本还依赖这个仓库')).toBeVisible()
  await shot(page, shots.unbindPhone)

  await page.goto('/license/apps')
  await expect(page.getByRole('button', { name: '2026-10-01 到期' })).toBeVisible()
  await page.getByRole('button', { name: '2026-10-01 到期' }).click()
  const flag = page.locator('.commercial-header-entry')
  await expect(flag).toHaveAttribute('data-detail', '1')
  await expect(flag).toHaveAttribute('data-narrow', '1')
  const sheet = page.locator('.el-drawer').filter({ hasText: '到期时间' })
  await expect(sheet.getByText('来源')).toBeVisible()
  await expect(sheet.getByText('绑定账号')).toBeVisible()
  await expect(sheet.getByText('授权域名')).toBeVisible()
  await shot(page, shots.badgePhone)

  await page.goto('/user/login')
  await expect(page.getByRole('link', { name: '个人中心' })).toBeVisible()
  await page.getByRole('button', { name: '打开菜单' }).click()
  const drawer = page
    .locator('.el-drawer')
    .filter({ has: page.locator('.public-nav__drawer-link') })
  await expect(
    drawer.locator('.public-nav__drawer-link.is-active', { hasText: '首页' })
  ).toBeVisible()
  await expect
    .poll(async () => {
      const box = await drawer.boundingBox()
      return box ? box.x >= -1 && box.width > 180 : false
    })
    .toBe(true)
  await shot(page, shots.navDrawerPhone)
})

test('手机上创建应用的仓库步骤是竖排卡片', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await mockApis(page, true)
  await login(page)
  await page.goto('/license/apps')
  await page.getByRole('button', { name: '新增应用' }).click()
  await page.getByPlaceholder('请输入应用名称').fill('Customer Portal')
  await page.getByRole('button', { name: '下一步' }).click()
  await expect(page.getByText('密钥已保存')).toBeVisible()
  await expect(
    page.locator('.repo-options__card.is-on').getByPlaceholder('所有者/仓库')
  ).toBeVisible()
  await shot(page, shots.createPhone)
})
