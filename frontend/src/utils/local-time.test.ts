import assert from 'node:assert/strict'

process.env.TZ = 'Asia/Shanghai'
const { formatLocalDateTime } = await import('./local-time')

// 存储检查记的是 UTC，北京时间要加 8 小时
assert.equal(formatLocalDateTime('2026-10-03T04:18:04Z'), '2026-10-03 12:18:04')
assert.equal(formatLocalDateTime('2026-10-03T04:18:04.123456789Z', false), '2026-10-03 12:18')
assert.equal(formatLocalDateTime('2026-10-03T12:18:04+08:00'), '2026-10-03 12:18:04')
assert.equal(formatLocalDateTime('2026-10-02T23:30:00-05:00'), '2026-10-03 12:30:00')
// 不带时区的站点时间原样整理
assert.equal(formatLocalDateTime('2026-10-03 12:18:04'), '2026-10-03 12:18:04')
assert.equal(formatLocalDateTime('2026-10-03'), '2026-10-03')
assert.equal(formatLocalDateTime('0001-01-01T00:00:00Z'), '')
assert.equal(formatLocalDateTime(''), '')

console.log('local-time tests passed')
