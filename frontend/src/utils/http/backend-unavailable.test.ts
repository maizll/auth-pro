import assert from 'node:assert/strict'
import {
  BackendUnreachableTracker,
  isBackendUnreachableFailure,
  beginOnlineUpdateWindow,
  endOnlineUpdateWindow,
  onlineUpdateWindow,
  quietDuringOnlineUpdate,
  shouldRedirectToBackendUnavailable,
  UPDATE_WINDOW_MAX_MS
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
assert.equal(quietDuringOnlineUpdate(502, true), false)
beginOnlineUpdateWindow('U1', '1.8.7')
assert.equal(shouldRedirectToBackendUnavailable('/'), false)
assert.equal(quietDuringOnlineUpdate(502, true), true)
assert.equal(quietDuringOnlineUpdate(undefined, false), true)
assert.equal(quietDuringOnlineUpdate(500, true), false)
assert.equal(onlineUpdateWindow(Date.now() + UPDATE_WINDOW_MAX_MS + 1), null)
beginOnlineUpdateWindow('U1', '1.8.7')
endOnlineUpdateWindow()
assert.equal(shouldRedirectToBackendUnavailable('/'), true)
assert.equal(quietDuringOnlineUpdate(502, true), false)

console.log('backend-unavailable tests passed')
