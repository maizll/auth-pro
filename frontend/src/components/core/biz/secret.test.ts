import assert from 'node:assert/strict'
import { copySecretText, secretDisplay } from './secret'

assert.equal(secretDisplay('abcd', false, '—'), '••••••••••••••••')
assert.equal(secretDisplay('abcd', true, '—'), 'abcd')
assert.equal(secretDisplay('', false, '—'), '—')
assert.equal(secretDisplay('  ', true, '暂无'), '暂无')

let written = ''
const ok = await copySecretText('secret-value', async (text) => {
  written = text
})
assert.equal(ok, true)
assert.equal(written, 'secret-value')

const empty = await copySecretText('   ', async () => {
  throw new Error('should not copy empty')
})
assert.equal(empty, false)

const failed = await copySecretText('secret-value', async () => {
  throw new Error('clipboard denied')
})
assert.equal(failed, false)
