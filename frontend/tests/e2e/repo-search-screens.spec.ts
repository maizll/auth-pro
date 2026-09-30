import { mkdirSync, readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test, type Page, type Route } from '@playwright/test'

const productVersion = readFileSync(
  path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../../VERSION'),
  'utf8'
).trim()

const shotDir = '/opt/cursor/artifacts/screenshots'
mkdirSync(shotDir, { recursive: true })

const repos = [
  {
    repo: 'acme/paid-packages',
    private: true,
    updatedAt: '2026-09-29T10:20:00Z',
    boundApp: ''
  },
  {
    repo: 'acme/shop-web',
    private: true,
    updatedAt: '2026-09-28T16:05:00Z',
    boundApp: ''
  },
  {
    repo: 'acme/public-docs',
    private: false,
    updatedAt: '2026-09-27T09:40:00Z',
    boundApp: ''
  },
  {
    repo: 'acme/auth-system',
    private: true,
    updatedAt: '2026-09-26T11:15:00Z',
    boundApp: '授权系统'
  }
]

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
  }
]

type Scene = 'list' | 'search' | 'token' | 'timeout'

async function mockApis(page: Page, scene: Scene) {
  const pending: Array<() => void> = []
  const release = () => pending.splice(0).forEach((fn) => fn())
  await page.route(/^https?:\/\/[^/]+\/api\//, async (route: Route) => {
    const url = new URL(route.request().url())
    const ok = (data: unknown) => route.fulfill({ status: 200, json: { code: 200, msg: '', data } })
    if (url.pathname === '/api/install/status') {
      await route.fulfill({ status: 200, json: { installed: true } })
      return
    }
    if (url.pathname === '/api/user/info') {
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
    if (url.pathname === '/api/system/menus') {
      await ok(menus)
      return
    }
    if (
      url.pathname === '/api/system-config/public' ||
      url.pathname === '/api/home-template/active'
    ) {
      await ok({ siteName: '授权服务', registrationEnabled: true, isDefault: true })
      return
    }
    if (url.pathname === '/api/app/list') {
      await ok([
        {
          id: 1,
          name: '授权系统',
          appKey: 'app_demo',
          appSecret: 'sk_live_demo',
          purchaseLicenseTypes: ['domain'],
          licenseCount: 1,
          recentVersion: '1.8.0',
          versionCount: 1,
          enabled: true,
          licenseRequired: true,
          remark: '',
          repo: 'acme/auth-system',
          createdAt: '2026-09-29 10:00'
        }
      ])
      return
    }
    if (url.pathname === '/api/store/account' || url.pathname === '/api/store/refresh') {
      await ok({
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
      })
      return
    }
    if (url.pathname.endsWith('/app-repos/token')) {
      await ok({
        ready: true,
        location: '主存储',
        message: '使用存储管理中的 GitHub 主存储，密钥已保存'
      })
      return
    }
    if (url.pathname.endsWith('/app-repos/suggest')) {
      await ok({ repo: 'acme/customer-portal', tokenReady: true })
      return
    }
    if (url.pathname.endsWith('/app-repos/github')) {
      if (scene === 'token') {
        await ok({
          status: 'token',
          message: 'GitHub 令牌无效或已过期。请到存储管理更新令牌。',
          list: []
        })
        return
      }
      if (scene === 'timeout') {
        await ok({ status: 'timeout', message: '连接超时，请稍后再试。', list: [] })
        return
      }
      const q = url.searchParams.get('q') || ''
      if (scene === 'search' && q) {
        await new Promise<void>((resolve) => pending.push(resolve))
      }
      const list = repos.filter((item) => !q || item.repo.includes(q))
      await ok({ status: list.length || q ? 'ok' : 'empty', message: '', list })
      return
    }
    if (url.pathname.includes('/notifications')) {
      await ok({ list: [], total: 0, count: 0, unread: 0 })
      return
    }
    await ok({})
  })
  return release
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
  }, productVersion)
}

async function openBind(page: Page) {
  await page.goto('/license/apps')
  await page.getByRole('button', { name: '新增应用' }).click()
  await page.getByPlaceholder('请输入应用名称').fill('客户门户')
  await page.getByRole('button', { name: '下一步' }).click()
  await expect(page.getByText('密钥已保存')).toBeVisible()
  await page.getByText('绑定已有仓库', { exact: true }).click()
}

async function shot(page: Page, file: string) {
  await page.addStyleTag({
    content:
      '#__vue-devtools-container__, .vue-devtools__anchor, [class*="vue-devtools"] { display: none !important; }'
  })
  await page.screenshot({ path: file, animations: 'disabled' })
}

function repoSelect(page: Page) {
  return page.locator('.repo-search .el-select')
}

async function openList(page: Page) {
  await repoSelect(page).click()
  await expect(page.locator('.repo-search-popper').getByText('acme/paid-packages')).toBeVisible()
}

for (const viewport of [
  { name: 'desktop', width: 1440, height: 900 },
  { name: 'phone', width: 390, height: 844 }
] as const) {
  test(`仓库搜索下拉 ${viewport.name}`, async ({ page }) => {
    test.setTimeout(120_000)
    await page.setViewportSize({ width: viewport.width, height: viewport.height })
    await login(page)

    let release = await mockApis(page, 'list')
    await openBind(page)
    await openList(page)
    await shot(page, `${shotDir}/repo-list-${viewport.name}.png`)
    await expect(page.locator('.repo-search-popper').getByText('建议改为私有')).toBeVisible()

    await page.unrouteAll({ behavior: 'ignoreErrors' })
    release = await mockApis(page, 'search')
    await page.reload()
    await openBind(page)
    await repoSelect(page).click()
    await page.keyboard.type('shop')
    await expect(page.getByText('正在搜索…')).toBeVisible()
    await shot(page, `${shotDir}/repo-search-${viewport.name}.png`)
    release()

    await page.unrouteAll({ behavior: 'ignoreErrors' })
    await mockApis(page, 'list')
    await page.reload()
    await openBind(page)
    await openList(page)
    const taken = page.locator('.repo-search-popper').getByText('已被应用「授权系统」占用')
    await taken.scrollIntoViewIfNeeded()
    await expect(taken).toBeVisible()
    await shot(page, `${shotDir}/repo-taken-${viewport.name}.png`)

    await page.unrouteAll({ behavior: 'ignoreErrors' })
    await mockApis(page, 'token')
    await page.reload()
    await openBind(page)
    await expect(page.getByRole('button', { name: '去存储管理' })).toBeVisible()
    await expect(page.getByText('GitHub 令牌无效或已过期。请到存储管理更新令牌。')).toBeVisible()
    await shot(page, `${shotDir}/repo-token-${viewport.name}.png`)

    await page.unrouteAll({ behavior: 'ignoreErrors' })
    await mockApis(page, 'timeout')
    await page.reload()
    await openBind(page)
    await expect(page.getByRole('button', { name: '重试' })).toBeVisible()
    await expect(page.getByText('连接超时，请稍后再试。')).toBeVisible()
    await shot(page, `${shotDir}/repo-timeout-${viewport.name}.png`)

    await page.unrouteAll({ behavior: 'ignoreErrors' })
    await mockApis(page, 'list')
    await page.reload()
    await openBind(page)
    await repoSelect(page).click()
    await page.keyboard.type('zzz')
    await expect(page.getByText('没有找到这个仓库，可以手动填写。')).toBeVisible()
    await page.getByPlaceholder('所有者/仓库').fill('不是仓库')
    await expect(page.getByText('请填写仓库，格式为 所有者/仓库')).toBeVisible()
    await shot(page, `${shotDir}/repo-manual-${viewport.name}.png`)
  })
}
