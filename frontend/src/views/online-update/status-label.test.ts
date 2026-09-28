import assert from 'node:assert/strict'
import { latestVersionStatus, updateChannelLabel } from './status-label'

assert.equal(updateChannelLabel('stable'), '正式版')
assert.equal(updateChannelLabel(''), '正式版')
assert.equal(updateChannelLabel('auth_pro'), '正式版')
assert.equal(updateChannelLabel('beta'), '测试版')
assert.equal(latestVersionStatus({}), '未检查')
assert.equal(latestVersionStatus({ unreachable: true }), '暂时连不上官网')
assert.equal(latestVersionStatus({ version: '1.7.1' }), '已是最新')
assert.equal(latestVersionStatus({ version: '1.7.2', updateAvailable: true }), '可更新')
assert.notEqual(latestVersionStatus({}), '已是最新')
