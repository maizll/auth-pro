import assert from 'node:assert/strict'
import { caughtErrorText, claimErrorToast } from './error-toast'

const shown = { message: '产品应用不存在或未启用', displayed: true }
const fresh = { message: '产品应用不存在或未启用', displayed: false }

assert.equal(caughtErrorText(shown, '绑定失败', true), null)
assert.equal(caughtErrorText(fresh, '绑定失败', false), '产品应用不存在或未启用')
assert.equal(caughtErrorText({ message: '' }, '绑定失败', false), '绑定失败')
assert.equal(caughtErrorText(new Error('登录失败'), '操作失败', false), '登录失败')
assert.equal(caughtErrorText('not-an-error', '操作失败', false), '操作失败')

const ledger = new Map<string, number>()
assert.equal(claimErrorToast('产品应用不存在或未启用', 1000, ledger, 400), true)
assert.equal(claimErrorToast('产品应用不存在或未启用', 1200, ledger, 400), false)
assert.equal(claimErrorToast('产品应用不存在或未启用', 1400, ledger, 400), true)
assert.equal(claimErrorToast('另一条错误', 1400, ledger, 400), true)
