import { expect, test, type Page, type Route } from '@playwright/test'

test.describe.configure({ mode: 'serial' })
test.setTimeout(60_000)

const publicSystemConfig = {
  code: 200,
  msg: '',
  data: {
    siteName: 'Nova License',
    siteSubtitle: '安全、稳定的软件授权服务',
    siteLogo: '',
    registrationEnabled: true
  }
}

const templateDocuments = {
  'cartoon-blue': {
    schemaVersion: 1,
    stylePreset: 'cartoon-blue',
    theme: {
      primaryColor: '#168fe5',
      backgroundColor: '#f1faff',
      textColor: '#15334a'
    },
    hero: {
      title: '授权服务，也可以',
      highlight: '简单又亲切',
      primaryAction: { label: '进入用户中心', type: 'login' }
    }
  },
  'fintech-gold': {
    schemaVersion: 1,
    stylePreset: 'fintech-gold',
    theme: {
      primaryColor: '#f3ba2f',
      backgroundColor: '#090b10',
      textColor: '#f4f6fa'
    },
    hero: {
      title: '让授权管理保持',
      highlight: '清晰、快速、可信',
      primaryAction: { label: '登录控制台', type: 'login' }
    }
  }
} as const

async function mockAdminAndTemplateAPIs(page: Page, templateFailure = false) {
  let activeTemplate: 'default' | keyof typeof templateDocuments = 'cartoon-blue'
  const enableRequests: string[] = []

  await page.route(/^https?:\/\/[^/]+\/api\//, async (route: Route) => {
    const requestURL = new URL(route.request().url())
    const path = requestURL.pathname

    if (path === '/api/install/status') {
      await route.fulfill({ status: 200, json: { installed: true } })
      return
    }
    if (path === '/api/system-config/public') {
      await route.fulfill({ status: 200, json: publicSystemConfig })
      return
    }
    if (path === '/api/auth/login') {
      await route.fulfill({
        status: 200,
        json: {
          code: 200,
          msg: '登录成功',
          data: { token: 'admin-test-token', refreshToken: 'admin-refresh-token' }
        }
      })
      return
    }
    if (path === '/api/user/info') {
      await route.fulfill({
        status: 200,
        json: {
          code: 200,
          msg: '',
          data: {
            buttons: [],
            roles: ['R_SUPER'],
            userId: 1,
            userName: 'preview-admin',
            email: 'preview@example.test'
          }
        }
      })
      return
    }
    if (path === '/api/system/plugins') {
      await route.fulfill({
        status: 200,
        json: {
          code: 200,
          msg: '',
          data: {
            categories: [
              {
                category: 'payment',
                title: '支付插件',
                plugins: [
                  {
                    id: 'epay',
                    category: 'payment',
                    name: '易支付 V1',
                    description: '授权系统本地支付插件',
                    homepage: '',
                    icon: 'ri:bank-card-line',
                    version: '1.0.0',
                    official: true,
                    enabled: true,
                    configured: true,
                    local: true,
                    remote: false,
                    source: 'builtin',
                    downloadUrl: ''
                  }
                ]
              }
            ],
            sources: [
              {
                id: 1,
                name: 'Local Template Source',
                url: 'http://127.0.0.1:18080/index.local.json',
                state: 'ok'
              }
            ]
          }
        }
      })
      return
    }
    if (path === '/api/system/home-templates') {
      if (templateFailure) {
        await route.fulfill({
          status: 200,
          json: { code: 503, msg: '软件源目录 API Key 未配置' }
        })
        return
      }
      await route.fulfill({
        status: 200,
        json: {
          code: 200,
          msg: '',
          data: {
            list: [
              {
                id: 'default',
                templateId: 'default',
                name: '默认首页模板',
                description: '系统内置首页模板',
                version: 'builtin',
                source: 'builtin',
                enabled: activeTemplate === 'default',
                installed: true,
                available: true
              },
              {
                id: 11,
                templateId: 'cartoon-blue',
                name: '圆趣蓝白红',
                description: '蓝白红圆趣主题',
                version: '1.0.0',
                source: 'Local Template Source',
                sourceType: 'json',
                enabled: activeTemplate === 'cartoon-blue',
                installed: true,
                available: true
              },
              {
                id: 12,
                templateId: 'fintech-gold',
                name: '黑金金融科技',
                description: '黑金金融科技主题',
                version: '1.0.0',
                source: 'Local Template Source',
                sourceType: 'json',
                enabled: activeTemplate === 'fintech-gold',
                installed: true,
                available: true
              }
            ]
          }
        }
      })
      return
    }
    const enableMatch = path.match(/^\/api\/system\/home-templates\/(default|11|12)\/enable$/)
    if (enableMatch) {
      enableRequests.push(path)
      activeTemplate =
        enableMatch[1] === '11'
          ? 'cartoon-blue'
          : enableMatch[1] === '12'
            ? 'fintech-gold'
            : 'default'
      await route.fulfill({ status: 200, json: { code: 200, msg: '首页模板已启用' } })
      return
    }
    if (path === '/api/home-template/active') {
      if (activeTemplate === 'default') {
        await route.fulfill({
          status: 200,
          json: {
            code: 200,
            msg: '',
            data: {
              id: 'default',
              templateId: 'default',
              name: '默认首页模板',
              version: 'builtin',
              isDefault: true
            }
          }
        })
        return
      }
      const templateID = activeTemplate === 'cartoon-blue' ? 11 : 12
      const templateName = activeTemplate === 'cartoon-blue' ? '圆趣蓝白红' : '黑金金融科技'
      await route.fulfill({
        status: 200,
        json: {
          code: 200,
          msg: '',
          data: {
            id: templateID,
            templateId: activeTemplate,
            name: templateName,
            version: '1.0.0',
            isDefault: false,
            schemaVersion: 1,
            document: templateDocuments[activeTemplate]
          }
        }
      })
      return
    }

    await route.fulfill({ status: 200, json: { code: 404, msg: 'not found' } })
  })

  return enableRequests
}

async function submitAdminLogin(page: Page) {
  await page.getByPlaceholder('请输入账号').fill('preview-admin')
  await page.getByPlaceholder('请输入密码').fill('preview-password')

  // The public-config fixture disables Geetest; the legacy local slider no longer exists.
  const loginResponse = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname === '/api/auth/login' && response.status() === 200
  )
  await page.getByRole('button', { name: '登录', exact: true }).click()
  await loginResponse
}

async function loginAsAdmin(page: Page) {
  await page.goto('/plugin-store')
  await expect(page).toHaveURL(/\/admin/)
  await submitAdminLogin(page)
  await expect(page).toHaveURL(/\/plugin-store$/, { timeout: 15_000 })
  await expect(page.getByRole('heading', { name: '应用商店', exact: true })).toBeVisible({
    timeout: 30_000
  })
  await page.waitForTimeout(300)
}

test('后台登录忽略根路径回跳并进入管理控制台', async ({ page }) => {
  await mockAdminAndTemplateAPIs(page)
  await page.goto('/admin?redirect=%2F')
  await submitAdminLogin(page)
  await expect(page).toHaveURL(/\/dashboard\/console$/, { timeout: 15_000 })
})

test('后台登录允许返回独立应用商店子路由', async ({ page }) => {
  await mockAdminAndTemplateAPIs(page)
  await page.route('**/admin/app-store/templates', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'text/html',
      body: '<!doctype html><title>App Store</title><main>独立应用商店入口</main>'
    })
  })
  await page.goto('/admin?redirect=%2Fadmin%2Fapp-store%2Ftemplates')
  await submitAdminLogin(page)
  await expect(page).toHaveURL(/\/admin\/app-store\/templates$/, { timeout: 15_000 })
})

test('模板 API Key 缺失不影响原插件商店', async ({ page }) => {
  await mockAdminAndTemplateAPIs(page, true)
  await loginAsAdmin(page)
  await expect(page.getByText('易支付 V1', { exact: true })).toBeVisible()
  await expect(
    page.getByText(/首页模板加载失败：软件源目录 API Key 未配置。插件商店不受影响/)
  ).toBeVisible()
})

test('应用商店可启用远程模板并切回默认首页', async ({ page }) => {
  const enableRequests = await mockAdminAndTemplateAPIs(page)
  await loginAsAdmin(page)

  const fintechCard = page.locator('.template-card').filter({ hasText: '黑金金融科技' })
  await expect(fintechCard).toBeVisible()
  await fintechCard.getByRole('button', { name: '启用', exact: true }).click({ force: true })
  await page
    .locator('.el-message-box')
    .getByRole('button', { name: '启用', exact: true })
    .click({ force: true })
  await expect(fintechCard.getByText('已启用', { exact: true })).toBeVisible()
  expect(enableRequests).toContain('/api/system/home-templates/12/enable')

  await page.goto('/user/login')
  await expect(page.locator('.gold-home')).toBeVisible()
  await expect(page).toHaveURL('http://127.0.0.1:4175/user/login')

  await page.goto('/plugin-store')
  await expect(page.getByRole('heading', { name: '应用商店', exact: true })).toBeVisible({
    timeout: 30_000
  })
  await page.waitForTimeout(300)
  const defaultCard = page.locator('.template-card').filter({ hasText: '默认首页模板' })
  await defaultCard.getByRole('button', { name: '启用', exact: true }).click({ force: true })
  await page
    .locator('.el-message-box')
    .getByRole('button', { name: '启用', exact: true })
    .click({ force: true })
  await expect(defaultCard.getByText('已启用', { exact: true })).toBeVisible()
  expect(enableRequests).toContain('/api/system/home-templates/default/enable')

  await page.goto('/user/login')
  await expect(page.locator('.license-home')).toBeVisible()
  await expect(page.locator('.remote-home')).toHaveCount(0)
  await expect(page).toHaveURL('http://127.0.0.1:4175/user/login')
})

test('应用商店支持上传 ZIP 安装后手动启用', async ({ page }, testInfo) => {
  await mockAdminAndTemplateAPIs(page)
  let uploaded = false
  let enabled = false
  let enableCalls = 0
  let uploadAttempts = 0
  await page.route('**/api/system/home-templates', async (route) => {
    await route.fulfill({
      status: 200,
      json: {
        code: 200,
        data: {
          list: [
            {
              id: 'default',
              templateId: 'default',
              name: '默认首页模板',
              version: 'builtin',
              source: 'builtin',
              enabled: !enabled,
              installed: true,
              available: true
            },
            ...(uploaded
              ? [
                  {
                    id: 99,
                    templateId: 'upload-12345',
                    name: '我的 ZIP 首页',
                    description: '自定义首页',
                    version: '1.0.0',
                    source: '本地上传',
                    sourceType: 'upload',
                    enabled,
                    installed: true,
                    available: true
                  }
                ]
              : [])
          ]
        }
      }
    })
  })
  await page.route('**/api/system/home-templates/upload', async (route) => {
    expect(route.request().method()).toBe('POST')
    expect(route.request().headers()['content-type']).toContain('multipart/form-data; boundary=')
    expect(route.request().headers().authorization).toContain('admin-test-token')
    expect(route.request().postDataBuffer()?.toString()).toContain('filename="custom-home.zip"')
    uploadAttempts++
    if (uploadAttempts === 1) {
      await route.fulfill({ status: 200, json: { code: 500, msg: '登记模板安装状态失败' } })
      return
    }
    uploaded = true
    await route.fulfill({ status: 200, json: { code: 200, data: { id: 99 } } })
  })
  await page.route('**/api/system/home-templates/99/enable', async (route) => {
    enabled = true
    enableCalls++
    await route.fulfill({ status: 200, json: { code: 200 } })
  })
  await loginAsAdmin(page)
  await page.getByRole('button', { name: '上传首页模板 ZIP' }).click()
  const dialog = page.getByRole('dialog', { name: '上传首页模板 ZIP' })
  await expect(dialog.locator('.el-upload-dragger')).toBeVisible()
  await expect(dialog.locator('.el-upload-dragger')).toHaveCSS('border-style', 'dashed')
  await expect(dialog.locator('input[type="file"]')).not.toBeVisible()
  await dialog
    .locator('input[type="file"]')
    .setInputFiles({ name: 'bad.txt', mimeType: 'text/plain', buffer: Buffer.from('bad') })
  await expect(dialog.getByText('请选择不超过 20 MiB 的非空 ZIP 文件')).toBeVisible()
  await dialog.locator('input[type="file"]').setInputFiles({
    name: 'custom-home.zip',
    mimeType: 'application/zip',
    buffer: Buffer.from(
      'UEsDBBQAAAAAAJRiKF1JdWPAJwAAACcAAAAKAAAAaW5kZXguaHRtbDwhZG9jdHlwZSBodG1sPjxoMT5DdXN0b20gWklQIGhvbWU8L2gxPlBLAQIUABQAAAAAAJRiKF1JdWPAJwAAACcAAAAKAAAAAAAAAAAAAACAAQAAAABpbmRleC5odG1sUEsFBgAAAAABAAEAOAAAAE8AAAAAAA==',
      'base64'
    )
  })
  await dialog.getByPlaceholder('默认使用 ZIP 文件名').fill('我的 ZIP 首页')
  await expect(dialog.locator('.upload-file-name')).toContainText('custom-home.zip')
  await dialog.screenshot({ path: testInfo.outputPath('upload-picker.png') })
  await dialog.getByRole('button', { name: '上传并安装' }).click()
  await expect(dialog.getByText('登记模板安装状态失败', { exact: true })).toBeVisible()
  await expect(dialog.locator('.upload-file-name')).toContainText('custom-home.zip')
  await dialog.getByRole('button', { name: '上传并安装' }).click()
  await expect(dialog).not.toBeVisible()
  expect(uploadAttempts).toBe(2)
  const card = page.locator('.template-card').filter({ hasText: '我的 ZIP 首页' })
  await expect(card.getByText('已安装', { exact: true })).toBeVisible()
  expect(enableCalls).toBe(0)
  await card.getByRole('button', { name: '启用', exact: true }).click({ force: true })
  await page
    .locator('.el-message-box')
    .getByRole('button', { name: '启用', exact: true })
    .click({ force: true })
  await expect(card.getByText('已启用', { exact: true })).toBeVisible()
  expect(enableCalls).toBe(1)
})
