import assert from 'node:assert/strict'
import {
  buyerRebindMessage,
  buyerTokenInvalidMessage,
  logoutFailureIsAlreadyGone,
  purchaseNeedsRebind,
  purchaseRebindNotice,
  shouldAnnounceBound,
  sourceConfirmedBound,
  storePayloadRebind
} from './commercial-bind'

const valid = {
  bound: true,
  sourceVerified: true,
  explicitRevoked: false,
  bindingInvalid: false,
  edition: 'free',
  permanent: false,
  domainMismatch: false
}

assert.equal(sourceConfirmedBound(valid), true)
assert.equal(shouldAnnounceBound(valid), true)
assert.equal(purchaseNeedsRebind(valid, false), false)
assert.equal(purchaseRebindNotice(valid, false), '')
assert.equal(
  storePayloadRebind({
    bound: true,
    sourceVerified: true,
    bindingInvalid: false,
    reason: 'free'
  }),
  false
)

const deleted = {
  bound: false,
  sourceVerified: false,
  explicitRevoked: false,
  bindingInvalid: true,
  bindingInvalidReason: 'binding_deleted',
  edition: 'free'
}

assert.equal(sourceConfirmedBound(deleted), false)
assert.equal(shouldAnnounceBound(deleted), false)
assert.equal(purchaseNeedsRebind(deleted, false), true)
assert.equal(purchaseRebindNotice(deleted, false), buyerRebindMessage)
assert.equal(
  storePayloadRebind({
    bound: false,
    bindingInvalid: true,
    bindingInvalidReason: 'binding_deleted',
    reason: 'unbound'
  }),
  false
)
assert.equal(
  logoutFailureIsAlreadyGone({
    message: buyerRebindMessage,
    data: { data: { rebind: true, reason: 'binding_deleted' } }
  }),
  true
)

const tokenInvalid = {
  bound: false,
  sourceVerified: false,
  bindingInvalid: true,
  bindingInvalidReason: 'token_invalid',
  explicitRevoked: false
}

assert.equal(shouldAnnounceBound(tokenInvalid), false)
assert.equal(purchaseNeedsRebind(tokenInvalid, false), true)
assert.equal(purchaseRebindNotice(tokenInvalid, false), buyerTokenInvalidMessage)
assert.equal(storePayloadRebind({ reason: 'token_invalid', revoked: true, rebind: true }), true)
assert.equal(
  storePayloadRebind({ bindingInvalid: true, bindingInvalidReason: 'token_invalid', reason: '' }),
  false
)
assert.equal(
  logoutFailureIsAlreadyGone({
    message: buyerTokenInvalidMessage,
    data: { reason: 'token_invalid', rebind: true }
  }),
  true
)

assert.equal(purchaseNeedsRebind({ bound: false }, false), true)
assert.equal(purchaseRebindNotice({ bound: false }, false), '')
assert.equal(shouldAnnounceBound({ ...valid, sourceVerified: false }), false)
assert.equal(shouldAnnounceBound({ ...valid, edition: 'commercial', permanent: true }), false)
assert.equal(logoutFailureIsAlreadyGone(new Error('创建订单失败')), false)
