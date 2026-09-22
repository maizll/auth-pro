/**
 * 一次性校验：developer-panel 窄屏导航开合（对照 #53 user/agent 行为）。
 * 运行：npx playwright test tests/e2e/developer-panel-mobile-nav.spec.ts
 */
import { expect, test, type Page, type Route } from '@playwright/test'

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
    if (pathname.includes('/notifications')) {
      await route.fulfill({
        status: 200,
        json: { code: 200, msg: '', data: { list: [], unread: 0 } }
      })
      return
    }
    await route.fulfill({ status: 200, json: { code: 200, msg: '', data: {} } })
  })
}

async function openDeveloperShell(page: Page) {
  await mockDeveloperAPIs(page)
  await page.addInitScript(() => {
    localStorage.setItem('developer_panel_token', 'e2e-dev-token')
    localStorage.setItem(
      'developer_panel_info',
      JSON.stringify({ username: 'dev', displayName: '测试开发者' })
    )
  })
  await page.setViewportSize({ width: 375, height: 812 })
  await page.goto('/developer-panel/dashboard')
  await expect(page.locator('.developer-panel-layout')).toBeVisible()
}

test.describe('developer-panel mobile nav', () => {
  test('汉堡打开、遮罩关闭、Esc、菜单跳转后关闭', async ({ page }) => {
    await openDeveloperShell(page)

    const layout = page.locator('.developer-panel-layout')
    const sidebar = page.locator('#panel-sidebar')
    const burger = page.locator('button.collapse-btn')
    const mask = page.locator('button.panel-nav-mask')

    await expect(layout).not.toHaveClass(/is-nav-open/)
    await expect(sidebar).toHaveAttribute('aria-hidden', 'true')

    await burger.click()
    await expect(layout).toHaveClass(/is-nav-open/)
    await expect(sidebar).toHaveAttribute('aria-hidden', 'false')
    await expect(mask).toHaveClass(/is-visible/)

    // 侧栏盖住遮罩左侧，点右侧可见暗区关闭（贴近真机点按）
    await mask.click({ position: { x: 340, y: 400 } })
    await expect(layout).not.toHaveClass(/is-nav-open/)

    await burger.click()
    await expect(layout).toHaveClass(/is-nav-open/)
    await page.locator('button.sidebar-close').click()
    await expect(layout).not.toHaveClass(/is-nav-open/)

    await burger.click()
    await page.keyboard.press('Escape')
    await expect(layout).not.toHaveClass(/is-nav-open/)

    await burger.click()
    await page.locator('.sidebar-menu .el-menu-item', { hasText: '我的插件' }).click()
    await expect(page).toHaveURL(/\/developer-panel\/plugins/)
    await expect(layout).not.toHaveClass(/is-nav-open/)
  })

  test('宽屏折叠后侧栏宽度约为 64px', async ({ page }) => {
    await mockDeveloperAPIs(page)
    await page.addInitScript(() => {
      localStorage.setItem('developer_panel_token', 'e2e-dev-token')
    })
    await page.setViewportSize({ width: 1280, height: 800 })
    await page.goto('/developer-panel/dashboard')
    await expect(page.locator('.developer-panel-layout')).toBeVisible()

    const layout = page.locator('.developer-panel-layout')
    const sidebar = page.locator('#panel-sidebar')
    await page.locator('button.collapse-btn').click()
    await expect(layout).toHaveClass(/is-collapsed/)
    // 等待 width 过渡结束，避免量到动画中间态
    await expect
      .poll(async () => (await sidebar.boundingBox())?.width ?? 0, { timeout: 2000 })
      .toBeLessThanOrEqual(70)
    const box = await sidebar.boundingBox()
    expect(box?.width).toBeGreaterThanOrEqual(60)
  })

  test('窄屏面包屑不换行且隐藏站点名前缀', async ({ page }) => {
    await openDeveloperShell(page)
    const crumb = page.locator('.panel-header .el-breadcrumb')
    await expect(crumb).toBeVisible()
    const box = await crumb.boundingBox()
    expect(box?.height ?? 99).toBeLessThanOrEqual(24)
    await expect(crumb.locator('.el-breadcrumb__item').first()).toBeHidden()
  })
})
