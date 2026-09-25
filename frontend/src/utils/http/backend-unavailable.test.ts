import assert from 'node:assert/strict'
import {
  BackendUnreachableTracker,
  isBackendUnreachableFailure,
  setBackendUnreachableRedirectPaused,
  shouldRedirectToBackendUnavailable
} from './backend-unavailable'

assert.equal(isBackendUnreachableFailure(undefined, false), true)
assert.equal(isBackendUnreachableFailure(502, true), true)
assert.equal(isBackendUnreachableFailure(504, true), true)
assert.equal(isBackendUnreachableFailure(503, true), true)
assert.equal(isBackendUnreachableFailure(500, true), false)
assert.equal(isBackendUnreachableFailure(401, true), false)
assert.equal(isBackendUnreachableFailure(400, true), false)

const tracker = new BackendUnreachableTracker(3)
assert.equal(tracker.record(true), false)
assert.equal(tracker.record(true), false)
assert.equal(tracker.record(true), true)
tracker.record(false)
assert.equal(tracker.record(true), false)

assert.equal(shouldRedirectToBackendUnavailable('/admin/online-update'), true)
assert.equal(shouldRedirectToBackendUnavailable('/backend-unavailable.html'), false)
setBackendUnreachableRedirectPaused(true)
assert.equal(shouldRedirectToBackendUnavailable('/'), false)
setBackendUnreachableRedirectPaused(false)
assert.equal(shouldRedirectToBackendUnavailable('/'), true)

console.log('backend-unavailable tests passed')
