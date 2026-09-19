import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  catalogItemActions,
  catalogVersionActions,
  isDeprecatedCatalogStatus
} from './catalog-actions.ts'

test('catalog item actions follow sourceTransitionAllowed without no-ops', () => {
  assert.deepEqual(catalogItemActions('draft'), ['approve', 'reject', 'shelf'])
  assert.deepEqual(catalogItemActions('review'), ['approve', 'reject', 'shelf'])
  assert.deepEqual(catalogItemActions('approved'), ['shelf', 'deprecate'])
  assert.deepEqual(catalogItemActions('published'), ['unshelf', 'deprecate'])
  assert.deepEqual(catalogItemActions('hidden'), ['shelf', 'deprecate'])
  assert.deepEqual(catalogItemActions('rejected'), [])
  assert.deepEqual(catalogItemActions('deprecated'), [])
  assert.equal(isDeprecatedCatalogStatus('deprecated'), true)
  assert.equal(isDeprecatedCatalogStatus('draft'), false)
})

test('catalog version actions follow sourceVersionTransitionAllowed without no-ops', () => {
  assert.deepEqual(catalogVersionActions('draft'), ['approve'])
  assert.deepEqual(catalogVersionActions('pending'), ['approve', 'reject'])
  assert.deepEqual(catalogVersionActions('published', false), ['latest', 'deprecate'])
  assert.deepEqual(catalogVersionActions('published', true), ['deprecate'])
  assert.deepEqual(catalogVersionActions('deprecated'), [])
})
