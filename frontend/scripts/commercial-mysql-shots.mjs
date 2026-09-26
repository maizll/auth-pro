import { readFileSync } from 'node:fs'
import { chromium } from '@playwright/test'

const state = JSON.parse(readFileSync(process.env.AUTH_PRO_E2E_STATE, 'utf8'))
const shot = (name) => `${state.artifacts}/${name}`
const viewport = { width: 390, height: 844 }

const browser = await chromium.launch({ headless: true })
const context = await browser.newContext({
  viewport,
  ignoreHTTPSErrors: true,
  deviceScaleFactor: 1,
  isMobile: true,
  hasTouch: true
})
const page = await context.newPage()
page.setDefaultTimeout(20000)

async function login(base) {
  await page.goto(`${base}/admin`, { waitUntil: 'domcontentloaded' })
  await page.locator('input').nth(0).fill(state.adminUser)
  await page.locator('input[type="password"]').fill(state.adminPass)
  await page.getByRole('button', { name: '登录' }).click()
  await page.waitForURL((url) => !url.pathname.startsWith('/admin'), { timeout: 20000 })
  await page.waitForTimeout(800)
}

async function openMenu() {
  const title = page.getByText('源站运营', { exact: true }).first()
  if (!(await title.isVisible().catch(() => false))) {
    const menuButton = page.locator('button').filter({ has: page.locator('svg, i') }).first()
    await page.locator('.ml-3, .max-sm\\:ml-\\[7px\\]').first().click({ timeout: 5000 }).catch(async () => {
      await menuButton.click()
    })
    await page.waitForTimeout(400)
  }
  await title.click()
  await page.waitForTimeout(400)
}

try {
  if (process.env.AUTH_PRO_E2E_RECAPTURE !== '1') {
  console.log('buyer login')
  await login(state.buyer)
  await page.goto(`${state.buyer}/license/apps`, { waitUntil: 'domcontentloaded' })
  await page.getByRole('button', { name: '升级商业版' }).click()
  await page.getByRole('dialog', { name: '升级商业版' }).waitFor()
  await page.getByPlaceholder('邮箱或账号').fill(state.buyerEmail)
  await page.getByPlaceholder('仅用于本次登录，不会保存').fill(state.buyerPass)
  await page.getByRole('button', { name: '登录并绑定' }).click()
  await page.getByText('已绑定').first().waitFor()
  await page.waitForFunction(() => {
    const button = [...document.querySelectorAll('button')].find((item) => item.textContent?.includes('生成付款码'))
    return button && !button.disabled
  })
  await page.getByRole('button', { name: '生成付款码' }).click()
  await page.locator('.upgrade-qr canvas, .upgrade-qr svg').first().waitFor()
  await page.screenshot({ path: shot('phone-buyer-pay-qr.png') })
  await page.getByRole('dialog', { name: '升级商业版' }).waitFor({ state: 'hidden', timeout: 40000 })
  await page.goto(`${state.buyer}/plugin-store`, { waitUntil: 'domcontentloaded' })
  await page.getByText('商业版 · 永久').first().waitFor()
  await page.getByText('浏览并安装插件和首页模板').waitFor()
  await page.getByRole('button', { name: '查看授权' }).waitFor()
  if (await page.getByRole('button', { name: '升级商业版' }).isVisible().catch(() => false)) {
    throw new Error('商业版仍显示升级按钮')
  }
  await page.screenshot({ path: shot('phone-buyer-commercial-v2.png') })
  await page.screenshot({ path: shot('phone-buyer-commercial.png') })
  await page.goto(`${state.buyer}/license/apps`, { waitUntil: 'domcontentloaded' })
  await page.getByText('买家第二个应用').waitFor()
  await page.getByText('买家主应用').waitFor()
  await page.getByRole('button', { name: '查看授权' }).waitFor()
  if (await page.getByRole('button', { name: '升级商业版' }).isVisible().catch(() => false)) {
    throw new Error('应用管理仍显示升级商业版')
  }
  if (await page.getByText('免费版仅支持').isVisible().catch(() => false)) {
    throw new Error('免费版限制提示还在')
  }
  }

  console.log('source shots')
  await login(state.source)
  await page.goto(`${state.source}/license/apps`, { waitUntil: 'domcontentloaded' })
  await page.getByText('商业版产品').first().waitFor()
  await page.getByText('可售').first().waitFor()
  await page.screenshot({ path: shot('phone-app-sale-status.png') })
  await page.getByRole('button', { name: '编辑' }).first().click()
  const saleSwitch = page.getByText('作为本站商业版出售')
  await saleSwitch.waitFor()
  await saleSwitch.evaluate((el) => el.scrollIntoView({ block: 'start' }))
  const advanced = page.getByText('高级设置', { exact: true })
  if (await advanced.isVisible().catch(() => false)) await advanced.click()
  await page.getByText('离线宽限天数').waitFor()
  await page.getByText('离线宽限天数').evaluate((el) => el.scrollIntoView({ block: 'nearest' }))
  await page.screenshot({ path: shot('phone-app-sale-switch.png') })
  await page.keyboard.press('Escape')

  await page.goto(`${state.source}/license/plans`, { waitUntil: 'domcontentloaded' })
  await page.getByText('永久商业版').first().waitFor()
  await page.screenshot({ path: shot('phone-license-plans.png') })

  await page.goto(`${state.source}/license/list`, { waitUntil: 'domcontentloaded' })
  await page.getByText('商店购买').first().waitFor()
  const expand = page.locator('.filter-toggle')
  if (await expand.isVisible().catch(() => false)) await expand.click()
  const sourceItem = page.locator('.el-form-item').filter({ hasText: '来源' }).first()
  await sourceItem.locator('.el-select').click()
  await page.getByRole('option', { name: '商店绑定' }).waitFor()
  await page.getByRole('option', { name: '商店购买' }).waitFor()
  await page.screenshot({ path: shot('phone-license-source.png') })
  await page.keyboard.press('Escape')

  await page.goto(`${state.source}/order-list`, { waitUntil: 'domcontentloaded' })
  await page.getByText('商业版').first().waitFor()
  await page.getByText(/商业版已支付合计/).waitFor()
  await page.screenshot({ path: shot('phone-payment-orders.png') })

  await login(state.upgrade)
  await page.goto(`${state.upgrade}/license/apps`, { waitUntil: 'domcontentloaded' })
  await openMenu()
  await page.locator('.menu-left .el-scrollbar__wrap').evaluate((wrap) => {
    const item = [...wrap.querySelectorAll('.el-sub-menu__title, .el-menu-item')].find((el) =>
      el.textContent?.includes('源站运营')
    )
    if (item) wrap.scrollTop = item.offsetTop
  })
  await page.getByText('软件目录', { exact: true }).waitFor()
  await page.getByText('入驻审核', { exact: true }).waitFor()
  await page.getByText('公开目录', { exact: true }).waitFor()
  await page.getByText('广告投放', { exact: true }).waitFor()
  await page.getByText('源站设置', { exact: true }).first().waitFor()
  for (const retired of ['商业版设置', '商店订单', '主授权与权益', '商业版收入']) {
    if (await page.getByText(retired, { exact: true }).isVisible().catch(() => false)) {
      throw new Error(`旧菜单仍在侧栏: ${retired}`)
    }
  }
  await page.screenshot({ path: shot('phone-source-menu-zh.png') })
  console.log('screenshots ok')
} catch (error) {
  await page.screenshot({ path: shot('phone-failure.png') }).catch(() => {})
  console.error(error)
  console.error('url', page.url())
  process.exitCode = 1
} finally {
  await browser.close()
}
