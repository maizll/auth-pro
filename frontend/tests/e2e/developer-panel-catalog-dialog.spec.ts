/**
 * developer-panel 登记插件 / 登记模板抽屉，以及新增版本信息弹框，在 ~375px 内完整可见。
 * 运行：npx playwright test tests/e2e/developer-panel-catalog-dialog.spec.ts
 */
import { expect, test, type Locator, type Page, type Route } from '@playwright/test'

const samplePlugin = {
  id: 'demo-plugin',
  name: '示例插件',
  version: '1.0.0',
  status: 'draft',
  appId: 1,
  category: 'other',
  updatedAt: '2026-09-22 10:00:00'
}

const sampleTemplate = {
  id: 'demo-template',
  name: '示例模板',
  version: '1.0.0',
  status: 'draft',
  appId: 1,
  category: 'home-template',
  updatedAt: '2026-09-22 10:00:00'
}

async function mockDeveloperAPIs(page: Page) {
  await page.route(/^https?:\/\/[^/]+\/api\//, async (route: Route) => {
    const { pathname } = new URL(route.request().url())
    if (pathname === '/api/install/status') {
      await route.fulfill({ status: 200, json: { installed: true } })
      return
    }
    if (pathname === '/api/system-config/public') {
      await route.fulfill({
        status: 200,
        json: {
          code: 200,
          msg: '',
          data: {
            siteName: '授权管理系统',
            siteSubtitle: '',
            siteLogo: '',
            registrationEnabled: true
          }
        }
      })
      return
    }
    if (pathname === '/api/v1/source/developer/me') {
      await route.fulfill({
        status: 200,
        json: {
          code: 200,
          msg: '',
          data: { username: 'dev', displayName: '测试开发者' }
        }
      })
      return
    }
    if (pathname === '/api/v1/source/developer/items') {
      await route.fulfill({
        status: 200,
        json: {
          code: 200,
          msg: '',
          data: { plugins: [samplePlugin], homeTemplates: [sampleTemplate] }
        }
      })
      return
    }
    if (pathname === '/api/v1/source/developer/apps') {
      await route.fulfill({
        status: 200,
        json: {
          code: 200,
          msg: '',
          data: { list: [{ id: 1, name: '演示应用', appKey: 'demo' }], total: 1 }
        }
      })
      return
    }
    if (pathname === '/api/v1/source/developer/categories') {
      await route.fulfill({
        status: 200,
        json: {
          code: 200,
          msg: '',
          data: {
            list: [
              { key: 'other', label: '其他', kind: 'plugin', builtin: true },
              { key: 'home-template', label: '首页模板', kind: 'template', builtin: true }
            ],
            total: 2
          }
        }
      })
      return
    }
    if (pathname.includes('/versions')) {
      await route.fulfill({
        status: 200,
        json: { code: 200, msg: '', data: { list: [], total: 0 } }
      })
      return
    }
    if (pathname.includes('/notifications')) {
      await route.fulfill({
        status: 200,
        json: { code: 200, msg: '', data: { list: [], unread: 0 } }
      })
      return
    }
    if (pathname === '/api/v1/source/developer/packages/upload') {
      const sha = 'ab'.repeat(32)
      await route.fulfill({
        status: 200,
        json: {
          code: 200,
          msg: 'ZIP 已保存到本站，地址和校验码已生成',
          data: {
            url: `/api/v1/public/source-packages/${sha}.zip`,
            sha256: sha,
            kind: 'plugin',
            id: 'demo-plugin',
            name: '演示插件',
            version: '1.2.0',
            description: '上传回填',
            stored: true
          }
        }
      })
      return
    }
    await route.fulfill({ status: 200, json: { code: 200, msg: '', data: {} } })
  })
}

async function openCatalog(page: Page, path: string, width: number) {
  await mockDeveloperAPIs(page)
  await page.addInitScript(() => {
    localStorage.setItem('developer_panel_token', 'e2e-dev-token')
    localStorage.setItem(
      'developer_panel_info',
      JSON.stringify({ username: 'dev', displayName: '测试开发者' })
    )
  })
  await page.setViewportSize({ width, height: 812 })
  await page.goto(path)
  await expect(page.locator('.developer-catalog')).toBeVisible()
}

async function boxOf(locator: Locator) {
  const box = await locator.boundingBox()
  expect(box, '元素应有布局盒').toBeTruthy()
  return box!
}

async function settleOverlay(locator: Locator) {
  await expect(locator).toBeVisible()
  await expect
    .poll(async () => locator.evaluate((el) => getComputedStyle(el).transform))
    .toBe('none')
}

async function expectInsideViewport(page: Page, locator: Locator) {
  const viewport = page.viewportSize()
  expect(viewport).toBeTruthy()
  const box = await boxOf(locator)
  expect(box.x).toBeGreaterThanOrEqual(-1)
  expect(box.y).toBeGreaterThanOrEqual(-1)
  expect(box.x + box.width).toBeLessThanOrEqual(viewport!.width + 1)
  expect(box.y + box.height).toBeLessThanOrEqual(viewport!.height + 1)
}

test.describe('developer-panel catalog dialogs on phone', () => {
  test('登记插件抽屉和新增版本弹框留在 375px 内，主按钮可点', async ({ page }) => {
    await openCatalog(page, '/developer-panel/plugins', 375)

    await page.getByRole('button', { name: '登记插件' }).click()
    const formDrawer = page.locator('.el-drawer').filter({ hasText: '登记插件' })
    await settleOverlay(formDrawer)
    await expectInsideViewport(page, formDrawer)
    await expectInsideViewport(
      page,
      formDrawer.locator('.el-form-item__label', { hasText: '应用' })
    )

    const submit = formDrawer
      .locator('.el-drawer__footer')
      .getByRole('button', { name: '提交审核' })
    await expectInsideViewport(page, submit)

    const hashBtn = formDrawer.getByRole('button', { name: '自动计算' })
    await hashBtn.scrollIntoViewIfNeeded()
    await expectInsideViewport(page, hashBtn)
    await expectInsideViewport(page, submit)

    await formDrawer.getByRole('button', { name: '取消' }).click()
    await expect(formDrawer).toBeHidden()

    await page.getByRole('button', { name: '版本' }).click()
    const versionDrawer = page.locator('.el-drawer').filter({ hasText: '示例插件 版本' })
    await settleOverlay(versionDrawer)
    await expectInsideViewport(page, versionDrawer)

    await versionDrawer.getByRole('button', { name: '新增版本' }).click()
    const versionDialog = page.locator('.el-dialog').filter({ hasText: '新增版本' })
    await settleOverlay(versionDialog)
    await expectInsideViewport(page, versionDialog)
    const saveDraft = versionDialog.getByRole('button', { name: '保存草稿' })
    await expectInsideViewport(page, saveDraft)
    await expectInsideViewport(
      page,
      versionDialog.locator('.el-form-item__label', { hasText: '版本' })
    )
  })

  test('登记模板抽屉留在 375px 内，主按钮可点', async ({ page }) => {
    await openCatalog(page, '/developer-panel/templates', 375)

    await page.getByRole('button', { name: '登记模板' }).click()
    const formDrawer = page.locator('.el-drawer').filter({ hasText: '登记模板' })
    await settleOverlay(formDrawer)
    await expectInsideViewport(page, formDrawer)
    await expectInsideViewport(
      page,
      formDrawer.locator('.el-form-item__label', { hasText: '模板地址' })
    )

    const submit = formDrawer
      .locator('.el-drawer__footer')
      .getByRole('button', { name: '提交审核' })
    await expectInsideViewport(page, submit)
    await formDrawer.getByRole('button', { name: '取消' }).click()
    await expect(formDrawer).toBeHidden()

    await page.getByRole('button', { name: '版本' }).click()
    const versionDrawer = page.locator('.el-drawer').filter({ hasText: '示例模板 版本' })
    await settleOverlay(versionDrawer)
    await expectInsideViewport(page, versionDrawer)
    await versionDrawer.getByRole('button', { name: '新增版本' }).click()
    const versionDialog = page.locator('.el-dialog').filter({ hasText: '新增版本' })
    await settleOverlay(versionDialog)
    await expectInsideViewport(page, versionDialog)
    await expectInsideViewport(page, versionDialog.getByRole('button', { name: '保存草稿' }))
  })

  test('上传 ZIP 后回填地址和校验码，外链才显示自动计算', async ({ page }) => {
    await openCatalog(page, '/developer-panel/plugins', 1280)
    await page.getByRole('button', { name: '登记插件' }).click()
    const formDrawer = page.locator('.el-drawer').filter({ hasText: '登记插件' })
    await settleOverlay(formDrawer)

    await expect(formDrawer.getByRole('button', { name: '自动计算' })).toBeVisible()
    await formDrawer.getByText('上传 ZIP（本站托管）', { exact: true }).click()
    await expect(formDrawer.getByRole('button', { name: '选择 ZIP' })).toBeVisible()
    await expect(formDrawer.getByRole('button', { name: '自动计算' })).toHaveCount(0)

    await formDrawer.locator('input[type="file"]').setInputFiles({
      name: 'demo-plugin.zip',
      mimeType: 'application/zip',
      buffer: Buffer.from('PK\x03\x04demo')
    })
    const sha = 'ab'.repeat(32)
    await expect(formDrawer.locator('input[placeholder="上传 ZIP 后自动填写"]')).toHaveValue(
      `/api/v1/public/source-packages/${sha}.zip`
    )
    await expect(formDrawer.locator('input[placeholder="64 位十六进制，可稍后补"]')).toHaveValue(sha)
    await expect(formDrawer.locator('input[placeholder="上传 ZIP 后自动填写"]')).toBeDisabled()
  })

  test('宽屏登记抽屉保持 560px', async ({ page }) => {
    await openCatalog(page, '/developer-panel/plugins', 1280)
    await page.getByRole('button', { name: '登记插件' }).click()
    const formDrawer = page.locator('.el-drawer').filter({ hasText: '登记插件' })
    await settleOverlay(formDrawer)
    const box = await boxOf(formDrawer)
    expect(box.width).toBeGreaterThanOrEqual(540)
    expect(box.width).toBeLessThanOrEqual(580)
  })
})
