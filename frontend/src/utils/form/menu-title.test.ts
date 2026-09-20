import assert from 'node:assert/strict'
import { isDemoMenuName, resolveMenuTitle, stripDemoMenus } from './menu-title'

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

console.log('menu-title ok')
