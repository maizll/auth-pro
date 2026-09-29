import assert from 'node:assert/strict'
import { commercialHeaderTerm } from './commercial-header-term'

const expire = Math.floor(new Date(2027, 8, 30, 12, 0, 0).getTime() / 1000)

assert.equal(
  commercialHeaderTerm({
    permanent: false,
    editionExpireAt: expire
  }),
  '2027-09-30 到期'
)
assert.equal(
  commercialHeaderTerm({
    permanent: true,
    editionExpireAt: expire
  }),
  '永久'
)
assert.equal(commercialHeaderTerm({ permanent: false, editionExpireAt: 0 }), '永久')
assert.equal(commercialHeaderTerm(null), '')

const dated = commercialHeaderTerm({ permanent: false, editionExpireAt: expire })
assert.equal(dated.includes('后台开通'), false)
assert.equal(dated.includes('·'), false)
