import assert from 'node:assert/strict'
import { UPDATE_RESTART_TIMEOUT_MS } from './restart-timeout'
import {
  UPDATE_OVERALL_TIMEOUT_MS,
  clearUpdateWait,
  interpretUpdatePoll,
  isUnreachableUpdateResponse,
  markUpdateReloaded,
  rememberRestartingSince,
  rememberUpdateWaitStart,
  sampleFromVersionHTTP,
  updateAlreadyReloaded,
  versionPollURL
} from './restart-watch'

const started = 1_000_000
const clock = { startedAt: started, restartingSince: 0 }

assert.equal(isUnreachableUpdateResponse(undefined, '', ''), true)
assert.equal(isUnreachableUpdateResponse(502, 'text/html', '<html>502</html>'), true)
assert.equal(isUnreachableUpdateResponse(504, 'text/html', 'gateway'), true)
assert.equal(isUnreachableUpdateResponse(200, 'text/html; charset=utf-8', '<html>nginx</html>'), true)
assert.equal(isUnreachableUpdateResponse(200, 'application/json', '{"code":200}'), false)

const down = sampleFromVersionHTTP(502, 'text/html', '<html>bad gateway</html>')
assert.equal(down.reachable, false)
const html200 = sampleFromVersionHTTP(200, 'text/html', '<html><h1>502 Bad Gateway</h1></html>')
assert.equal(html200.reachable, false)
const network = sampleFromVersionHTTP(undefined, undefined, '', true)
assert.equal(network.reachable, false)

const waiting = interpretUpdatePoll(down, '1.6.0', clock, started + 5_000)
assert.equal(waiting.action, 'wait')
if (waiting.action === 'wait') {
  assert.equal(waiting.restarting, true)
  assert.equal(waiting.restartingSince, started + 5_000)
}

const restarted = interpretUpdatePoll(
  down,
  '1.6.0',
  { startedAt: started, restartingSince: started + 5_000 },
  started + 5_000 + UPDATE_RESTART_TIMEOUT_MS
)
assert.equal(restarted.action, 'timeout')

const ready = sampleFromVersionHTTP(
  200,
  'application/json',
  JSON.stringify({ code: 200, data: { version: 'v1.6.0' } })
)
const updated = interpretUpdatePoll(ready, '1.6.0', clock, started + 8_000)
assert.equal(updated.action, 'reload')

const oldDuringDownload = sampleFromVersionHTTP(
  200,
  'application/json',
  JSON.stringify({ code: 200, data: { version: '1.5.9' } })
)
const stillDownloading = interpretUpdatePoll(oldDuringDownload, '1.6.0', clock, started + 20_000)
assert.equal(stillDownloading.action, 'wait')
if (stillDownloading.action === 'wait') assert.equal(stillDownloading.restarting, false)

const rolled = sampleFromVersionHTTP(
  200,
  'application/json',
  JSON.stringify({
    code: 200,
    data: {
      version: '1.5.9',
      update: { rolledBack: true, reason: '新版本没有健康启动，已回滚到 1.5.9。旧版本已重新拉起。' }
    }
  })
)
const rollback = interpretUpdatePoll(rolled, '1.6.0', clock, started + 30_000)
assert.equal(rollback.action, 'rollback')
if (rollback.action === 'rollback') {
  assert.match(rollback.reason, /已回滚/)
}

const overall = interpretUpdatePoll(oldDuringDownload, '1.6.0', clock, started + UPDATE_OVERALL_TIMEOUT_MS)
assert.equal(overall.action, 'timeout')

assert.equal(versionPollURL(42), '/api/system/version?_=42')

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
assert.equal(rememberUpdateWaitStart('job', 10, storage), 10)
assert.equal(rememberUpdateWaitStart('job', 99, storage), 10)
assert.equal(rememberRestartingSince('job', 20, storage), 20)
markUpdateReloaded('job', storage)
assert.equal(updateAlreadyReloaded('job', storage), true)
clearUpdateWait('job', storage)
assert.equal(updateAlreadyReloaded('job', storage), false)

console.log('restart-watch tests passed')
