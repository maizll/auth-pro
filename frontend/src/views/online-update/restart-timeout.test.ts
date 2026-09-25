import assert from 'node:assert/strict'
import {
  UPDATE_RESTART_TIMEOUT_MS,
  clearRestartStart,
  rememberRestartStart,
  restartTimedOut
} from './restart-timeout'

const memory = new Map<string, string>()
const storage = {
  getItem: (key: string) => memory.get(key) ?? null,
  setItem: (key: string, value: string) => {
    memory.set(key, value)
  },
  removeItem: (key: string) => {
    memory.delete(key)
  }
}

assert.equal(restartTimedOut(1_000, 1_000 + UPDATE_RESTART_TIMEOUT_MS - 1, 'restarting'), false)
assert.equal(restartTimedOut(1_000, 1_000 + UPDATE_RESTART_TIMEOUT_MS, 'restarting'), true)
assert.equal(restartTimedOut(1_000, 1_000 + UPDATE_RESTART_TIMEOUT_MS + 5_000, 'success'), false)
assert.equal(restartTimedOut(0, 99_999, 'restarting'), false)

const started = rememberRestartStart('job-1', 5_000, storage)
assert.equal(started, 5_000)
assert.equal(rememberRestartStart('job-1', 9_000, storage), 5_000)
clearRestartStart('job-1', storage)
assert.equal(rememberRestartStart('job-1', 9_000, storage), 9_000)

console.log('restart-timeout tests passed')
