import assert from 'node:assert/strict'
import {
  formatManageMenuName,
  isDemoMenuName,
  resolveManageMenuTree,
  resolveMenuTitle,
  stripDemoMenus
} from './menu-title'

assert.equal(resolveMenuTitle('menus.system.epayConfig'), '支付配置')
assert.equal(resolveMenuTitle('menus.integration.store'), '应用商店')
assert.equal(resolveMenuTitle('menus.integration.update'), '在线更新')
assert.equal(resolveMenuTitle('menus.integration.title'), '接入开发')
assert.equal(resolveMenuTitle('我的商店'), '我的商店')
assert.equal(resolveMenuTitle(''), '')
assert.equal(resolveMenuTitle('', 'PluginStore'), 'PluginStore')
assert.equal(resolveMenuTitle(undefined, 'PluginStore'), 'PluginStore')
assert.equal(resolveMenuTitle('menus.unknown.leaf'), 'leaf')

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

console.log('menu-title ok')
