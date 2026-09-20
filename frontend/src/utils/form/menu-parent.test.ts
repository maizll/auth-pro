import assert from 'node:assert/strict'
import {
  buildParentMenuOptions,
  collectSelfAndDescendantIds,
  isInvalidMenuParent
} from './menu-parent'

const menus = [
  {
    id: 8,
    name: 'Sdk',
    title: '接入开发',
    children: [
      { id: 801, name: 'SdkIndex', title: 'SDK 示例' },
      { id: 210, name: 'PluginStore', title: '应用商店' }
    ]
  },
  { id: 9, name: 'SourceStation', title: '源站运营' }
]

assert.deepEqual([...collectSelfAndDescendantIds(menus, 8)].sort((a, b) => a - b), [8, 210, 801])
assert.equal(isInvalidMenuParent(8, 8, menus), true)
assert.equal(isInvalidMenuParent(8, 801, menus), true)
assert.equal(isInvalidMenuParent(8, 210, menus), true)
assert.equal(isInvalidMenuParent(210, 0, menus), false)
assert.equal(isInvalidMenuParent(210, 9, menus), false)
assert.equal(isInvalidMenuParent(210, 8, menus), false)

const options = buildParentMenuOptions(menus, 8)
assert.equal(options[0].id, 0)
assert.equal(options[0].label, '无（顶级）')
assert.equal(
  options[0].children?.some((item) => item.id === 8),
  false
)
assert.equal(
  options[0].children?.some((item) => item.id === 9),
  true
)

const createOptions = buildParentMenuOptions(menus)
assert.equal(
  createOptions[0].children?.some((item) => item.id === 8),
  true
)

console.log('menu-parent ok')
