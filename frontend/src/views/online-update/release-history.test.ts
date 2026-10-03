import assert from 'node:assert/strict'
import {
  filterReleases,
  groupOpenByDefault,
  groupReleases,
  layoutGroups,
  majorGroupKey,
  releaseOpenByDefault
} from './release-history'

const release = (version: string, notes: string[] = []) => ({
  version,
  channel: 'stable',
  releasedAt: '',
  notes
})
const all = [
  release('1.8.6', ['更新包带官方签名']),
  release('1.8.5', ['购买商业版改成按应用']),
  release('1.8.4'),
  release('1.7.9', ['修复授权列表']),
  release('1.7.8'),
  release('1.6.0', ['首页模板'])
]

assert.equal(majorGroupKey('v1.8.6'), '1.8.x')
assert.equal(majorGroupKey('1.10.0'), '1.10.x')
assert.equal(majorGroupKey('nightly'), 'nightly')

assert.equal(filterReleases(all, '').length, 6)
assert.deepEqual(
  filterReleases(all, 'v1.7').map((r) => r.version),
  ['1.7.9', '1.7.8']
)
assert.deepEqual(
  filterReleases(all, '签名').map((r) => r.version),
  ['1.8.6']
)
assert.deepEqual(
  filterReleases(all, '授权列表').map((r) => r.version),
  ['1.7.9']
)
assert.equal(filterReleases(all, '不存在').length, 0)

const groups = groupReleases(all)
assert.deepEqual(
  groups.map((g) => [g.key, g.releases.length]),
  [
    ['1.8.x', 3],
    ['1.7.x', 2],
    ['1.6.x', 1]
  ]
)
// 收起的组不渲染版本；展开的组各自按页数显示，没显示完的记下还剩几个
const firstOnly = layoutGroups(
  groups,
  (_g, i) => i === 0,
  () => 2
)
assert.deepEqual(
  firstOnly.map((v) => [v.shown, v.hidden]),
  [
    [2, 1],
    [0, 0],
    [0, 0]
  ]
)
const allOpen = layoutGroups(
  groups,
  () => true,
  (g) => (g.key === '1.7.x' ? 1 : 10)
)
assert.deepEqual(
  allOpen.map((v) => [v.shown, v.hidden]),
  [
    [3, 0],
    [1, 1],
    [1, 0]
  ]
)

assert.equal(releaseOpenByDefault('v1.8.6', '1.8.6', '1.8.4'), true)
assert.equal(releaseOpenByDefault('1.8.4', '1.8.6', '1.8.4'), true)
assert.equal(releaseOpenByDefault('1.8.5', '1.8.6', '1.8.4'), false)
assert.equal(releaseOpenByDefault('', '', ''), false)

const full = groupReleases(all)
assert.equal(groupOpenByDefault(full[0], 0, '1.8.6', '1.8.6'), true)
assert.equal(groupOpenByDefault(full[1], 1, '1.8.6', '1.8.6'), false)
assert.equal(groupOpenByDefault(full[1], 1, '1.8.6', '1.7.8'), true)
assert.equal(groupOpenByDefault(full[2], 2, '1.8.6', '1.8.6', true), true)

console.log('release-history tests passed')
