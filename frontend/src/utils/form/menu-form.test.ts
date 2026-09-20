import assert from 'node:assert/strict'
import { mapManageRowToForm, toMenuSavePayload } from './menu-form'

const form = mapManageRowToForm({
  id: 210,
  parentId: 0,
  name: 'PluginStore',
  path: '/plugin-store',
  component: '/plugin-store/index',
  title: 'menus.integration.store',
  icon: 'ri:store-2-line',
  sort: 8,
  enabled: true
})

assert.ok(form)
assert.equal(form.name, '应用商店')
assert.equal(form.label, 'PluginStore')
assert.equal(form.parentId, 0)
assert.equal(form.sort, 8)
assert.equal(form.id, 210)

const blankTitle = mapManageRowToForm({
  id: 8,
  parentId: 0,
  name: 'Sdk',
  title: ''
})
assert.equal(blankTitle?.name, 'Sdk')

assert.equal(mapManageRowToForm(null), null)

const payload = toMenuSavePayload({
  parentId: null,
  label: 'PluginStore',
  path: '/plugin-store',
  name: '应用商店',
  sort: 8,
  isEnable: true
})
assert.equal(payload.parentId, 0)
assert.equal(payload.title, '应用商店')
assert.equal(payload.name, 'PluginStore')
assert.equal(payload.sort, 8)
assert.equal(payload.enabled, true)

const zeroSort = mapManageRowToForm({ id: 1, name: 'Dash', title: '工作台', sort: 0 })
assert.equal(zeroSort?.sort, 0)

const alreadyChinese = mapManageRowToForm({
  id: 211,
  name: 'OnlineUpdate',
  title: '在线更新',
  parentId: 0
})
assert.equal(alreadyChinese?.name, '在线更新')

const fromMeta = mapManageRowToForm({
  id: 8,
  name: 'Sdk',
  meta: { title: 'menus.integration.title' }
})
assert.equal(fromMeta?.name, '接入开发')

const saved = toMenuSavePayload({
  label: 'PluginStore',
  path: '/plugin-store',
  name: '   '
})
assert.equal(saved.title, '')

// 编辑路径：列表行只有 title/name → 打开弹窗回填中文 → 保存非空 title → 再打开仍非空
const listRow = {
  id: 210,
  parentId: 0,
  name: 'PluginStore',
  path: '/plugin-store',
  title: 'menus.integration.store'
}
const opened = mapManageRowToForm(listRow)
assert.ok(opened)
assert.equal(opened.name, '应用商店')
assert.ok(opened.name.trim())
const persist = toMenuSavePayload(opened)
assert.equal(persist.title, '应用商店')
assert.ok(persist.title)
const reopened = mapManageRowToForm({ ...listRow, title: persist.title })
assert.equal(reopened?.name, '应用商店')

console.log('menu-form ok')
