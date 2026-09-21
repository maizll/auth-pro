import assert from 'node:assert/strict'
import { resolveBizStatus } from './status'

const reviewPending = resolveBizStatus({ domain: 'review', status: 'pending' })
assert.equal(reviewPending.label, '待审核')
assert.equal(reviewPending.type, 'warning')
assert.equal(reviewPending.known, true)

const adApproved = resolveBizStatus({ domain: 'ad', status: 'approved' })
assert.equal(adApproved.label, '已通过')
assert.equal(adApproved.type, 'success')

const licenseDisabled = resolveBizStatus({ domain: 'license', status: 'disabled' })
assert.equal(licenseDisabled.label, '已禁用')
assert.equal(licenseDisabled.type, 'danger')

const licenseExpiring = resolveBizStatus({ domain: 'license', status: 'expiring' })
assert.equal(licenseExpiring.label, '即将到期')
assert.equal(licenseExpiring.type, 'warning')

const campaignActive = resolveBizStatus({ domain: 'campaign', status: 'active' })
assert.equal(campaignActive.label, '进行中')
assert.equal(campaignActive.type, 'success')

const campaignDisabled = resolveBizStatus({ domain: 'campaign', status: 'disabled' })
assert.equal(campaignDisabled.label, '已禁用')
assert.equal(campaignDisabled.type, 'warning')

const withServerLabel = resolveBizStatus({
  domain: 'license',
  status: 'active',
  label: '正常'
})
assert.equal(withServerLabel.label, '正常')
assert.equal(withServerLabel.type, 'success')

const unknown = resolveBizStatus({ domain: 'review', status: 'mystery-state' })
assert.equal(unknown.known, false)
assert.equal(unknown.label, 'mystery-state')
assert.equal(unknown.type, 'info')

const unknownKeepsLabel = resolveBizStatus({
  domain: 'license',
  status: 'mystery-state',
  label: '接口原文'
})
assert.equal(unknownKeepsLabel.label, '接口原文')
assert.equal(unknownKeepsLabel.type, 'info')

const empty = resolveBizStatus({ domain: 'ad', status: '' })
assert.equal(empty.label, '-')
assert.equal(empty.type, 'info')
assert.equal(empty.known, false)

assert.notEqual(
  resolveBizStatus({ domain: 'license', status: 'active' }).label,
  resolveBizStatus({ domain: 'campaign', status: 'active' }).label
)
