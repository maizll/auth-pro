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

console.log('menu-form ok')
