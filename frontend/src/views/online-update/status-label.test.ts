import assert from 'node:assert/strict'
import { latestVersionStatus, stoppedBeforeInstall, updateChannelLabel } from './status-label'

assert.equal(updateChannelLabel('stable'), '正式版')
assert.equal(updateChannelLabel(''), '正式版')
assert.equal(updateChannelLabel('auth_pro'), '正式版')
assert.equal(updateChannelLabel('beta'), '测试版')
assert.equal(latestVersionStatus({}), '未检查')
assert.equal(latestVersionStatus({ unreachable: true }), '暂时连不上官网')
assert.equal(latestVersionStatus({ version: '1.7.1' }), '已是最新')
assert.equal(latestVersionStatus({ version: '1.7.2', updateAvailable: true }), '可更新')
assert.notEqual(latestVersionStatus({}), '已是最新')

// 下载/验签阶段失败：没动网站
assert.equal(stoppedBeforeInstall(30), true)
assert.equal(stoppedBeforeInstall(50), true)
// 安装阶段失败且原因没说线上未改动：按回滚处理
assert.equal(stoppedBeforeInstall(95), false)
assert.equal(stoppedBeforeInstall(95, '新版本启动失败，已回滚'), false)
// 更新脚本在动文件之前失败（建不出备份目录），原因写明线上目录未改动
assert.equal(stoppedBeforeInstall(95, '无法创建前端备份目录，线上目录未改动'), true)
assert.equal(stoppedBeforeInstall(95, '网站没有任何改动'), true)
