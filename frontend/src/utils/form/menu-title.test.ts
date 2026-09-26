import assert from 'node:assert/strict'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import {
  formatManageMenuName,
  isDemoMenuName,
  resolveManageMenuTree,
  resolveMenuTitle,
  stripDemoMenus
} from './menu-title'

const MISSING_MENU_TITLE_ZH = '未命名菜单'

assert.equal(resolveMenuTitle('menus.integration.store'), '应用商店')
assert.equal(resolveMenuTitle('menus.integration.update'), '在线更新')
assert.equal(resolveMenuTitle('menus.integration.title'), '接入开发')
assert.equal(resolveMenuTitle('menus.sourceStation.edition'), '商业版设置')
assert.equal(resolveMenuTitle('menus.sourceStation.storeOrders'), '商店订单')
assert.equal(resolveMenuTitle('menus.sourceStation.storeLicenses'), '主授权与权益')
assert.equal(resolveMenuTitle('menus.sourceStation.storeRevenue'), '商业版收入')
assert.equal(resolveMenuTitle('我的商店'), '我的商店')
assert.equal(resolveMenuTitle(''), '')
assert.equal(resolveMenuTitle('', 'PluginStore'), 'PluginStore')
assert.equal(resolveMenuTitle(undefined, 'PluginStore'), 'PluginStore')
assert.equal(resolveMenuTitle('menus.unknown.leaf'), MISSING_MENU_TITLE_ZH)
assert.notEqual(resolveMenuTitle('menus.unknown.leaf'), 'leaf')

assert.equal(isDemoMenuName('Result'), true)
assert.equal(isDemoMenuName('Exception404'), true)
assert.equal(isDemoMenuName('PluginStore'), false)

const tree = stripDemoMenus([
  { name: 'Sdk', children: [{ name: 'SdkIndex' }] },
  { name: 'Result', children: [{ name: 'ResultSuccess' }] },
  { name: 'Exception', children: [{ name: 'Exception404' }] },
  { name: 'System' }
])
assert.deepEqual(
  tree.map((item) => item.name),
  ['Sdk', 'System']
)

assert.equal(formatManageMenuName({ title: 'menus.integration.store', name: 'PluginStore' }), '应用商店')
assert.equal(formatManageMenuName({ title: '', name: 'PluginStore' }), 'PluginStore')
assert.equal(formatManageMenuName({ title: '在线更新', name: 'OnlineUpdate' }), '在线更新')

const resolved = resolveManageMenuTree([
  {
    name: 'Sdk',
    title: 'menus.integration.title',
    children: [{ name: 'PluginStore', title: 'menus.integration.store', children: [] }]
  }
])
assert.equal(resolved[0].title, '接入开发')
assert.equal(resolved[0].children?.[0].title, '应用商店')

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../../..')

function collectFiles(dir: string, predicate: (file: string) => boolean, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    const full = path.join(dir, entry)
    if (statSync(full).isDirectory()) {
      collectFiles(full, predicate, out)
      continue
    }
    if (predicate(full)) out.push(full)
  }
  return out
}

function collectMenuKeys(text: string): string[] {
  return [...text.matchAll(/['"]menus(?:\.[A-Za-z0-9_]+)+['"]/g)].map((match) =>
    match[0].slice(1, -1)
  )
}

function isChineseMenuTitle(title: string): boolean {
  return /[\u4e00-\u9fff]/.test(title) || /^\d+$/.test(title)
}

const sources = [
  ...collectFiles(path.join(repoRoot, 'frontend/src/router'), (file) => file.endsWith('.ts')),
  path.join(repoRoot, 'backend/handler/menu_spec.go'),
  path.join(repoRoot, 'backend/handler/menu_seed.sql')
]
const missing: string[] = []
const seen = new Set<string>()
for (const file of sources) {
  const text = readFileSync(file, 'utf8')
  for (const key of collectMenuKeys(text)) {
    if (seen.has(key)) continue
    seen.add(key)
    const title = resolveMenuTitle(key)
    const leaf = key.split('.').pop() || key
    if (!isChineseMenuTitle(title) || title === key || title === leaf || title === MISSING_MENU_TITLE_ZH) {
      missing.push(`${key} => ${title} (${path.relative(repoRoot, file)})`)
    }
  }
}
assert.equal(missing.length, 0, `菜单标题缺中文:\n${missing.join('\n')}`)

console.log('menu-title ok')
