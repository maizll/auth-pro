import assert from 'node:assert/strict'
import { shouldAutoRetry } from './network-retry'

assert.equal(shouldAutoRetry('GET', true), true)
assert.equal(shouldAutoRetry('get', true), true)
assert.equal(shouldAutoRetry(undefined, true), true)
assert.equal(shouldAutoRetry('GET', false), false)
assert.equal(shouldAutoRetry('POST', true), false)
assert.equal(shouldAutoRetry('DELETE', true), false)

console.log('network-retry tests passed')
