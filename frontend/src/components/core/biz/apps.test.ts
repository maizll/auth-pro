import assert from 'node:assert/strict'
import { appLoadErrorText, appOptionValue, normalizeAppOptions, panelAppPresets } from './apps'

assert.deepEqual(normalizeAppOptions(null), [])
assert.deepEqual(normalizeAppOptions({ id: 1 }), [])
assert.deepEqual(
  normalizeAppOptions([
    { id: 1, name: '商城', appKey: 'shop' },
    { id: '2', name: '论坛' },
    { name: '缺 id' },
    { id: 3 }
  ]),
  [
    { id: 1, name: '商城', appKey: 'shop' },
    { id: '2', name: '论坛' }
  ]
)

assert.equal(appOptionValue({ id: 1, name: '商城', appKey: 'shop' }, 'id'), 1)
assert.equal(appOptionValue({ id: 1, name: '商城', appKey: 'shop' }, 'appKey'), 'shop')
assert.equal(appOptionValue({ id: 1, name: '商城' }, 'appKey'), undefined)

assert.equal(appLoadErrorText('user-panel'), '加载应用列表失败')
assert.equal(appLoadErrorText('agent-panel'), undefined)
assert.equal(appLoadErrorText('license-options'), undefined)
assert.equal(appLoadErrorText('promotion'), undefined)
assert.equal(appLoadErrorText('license-apps'), undefined)
assert.equal(panelAppPresets['user-panel'].url, '/api/user-panel/apps')
assert.equal(panelAppPresets['user-panel'].tokenKey, 'user_panel_token')
assert.equal(panelAppPresets['agent-panel'].url, '/api/agent-panel/apps')
assert.equal(panelAppPresets['agent-panel'].tokenKey, 'agent_panel_token')
