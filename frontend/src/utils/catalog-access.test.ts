import assert from 'node:assert/strict'
import { catalogItemAccess } from './catalog-access'

const commercialFace = catalogItemAccess({
  priceCents: 100,
  access: { party: 'official', commercialIncluded: true, owned: true, grant: 'commercial' }
})
assert.equal(commercialFace.party, 'official')
assert.equal(commercialFace.commercialIncluded, true)
assert.equal(commercialFace.owned, true)
assert.equal(commercialFace.needsPurchase, false)
assert.equal(commercialFace.badge?.text, '商业版免费')

const freeFace = catalogItemAccess({
  priceCents: 100,
  access: { party: 'official', commercialIncluded: true, owned: false, grant: '' }
})
assert.equal(freeFace.needsPurchase, true)
assert.equal(freeFace.party, 'official')
assert.equal(freeFace.badge?.text, '商业版免费')

const third = catalogItemAccess({
  priceCents: 100,
  access: { party: 'third', commercialIncluded: false, owned: false, grant: '' }
})
assert.equal(third.party, 'third')
assert.equal(third.commercialIncluded, false)
assert.equal(third.needsPurchase, true)
assert.equal(third.badge, null)

const bought = catalogItemAccess({
  priceCents: 5000,
  access: { party: 'third', commercialIncluded: false, owned: true, grant: 'purchase' }
})
assert.equal(bought.needsPurchase, false)
assert.equal(bought.owned, true)
assert.equal(bought.badge?.text, '已包含')

const officialSeparate = catalogItemAccess({
  priceCents: 100,
  access: { party: 'official', commercialIncluded: false, owned: false, grant: '' }
})
assert.equal(officialSeparate.party, 'official')
assert.equal(officialSeparate.commercialIncluded, false)
assert.equal(officialSeparate.needsPurchase, true)
assert.equal(officialSeparate.badge, null)
