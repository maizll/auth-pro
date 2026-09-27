// 购买窗口怎么判断「可以付款」和「必须重绑」。只看接口字段，不弹提示。

export const buyerRebindMessage = '之前的绑定已在源站删除，请重新绑定账号后继续购买'

export const buyerTokenInvalidMessage = '绑定令牌已失效，请重新绑定账号后继续购买'

const buyerTerminalReasons = new Set([
  'license_deleted',
  'binding_deleted',
  'binding_revoked',
  'binding_expired',
  'license_not_found',
  'license_revoked',
  'license_expired',
  'token_invalid'
])

interface PurchaseBindInput {
  bound?: boolean
  explicitRevoked?: boolean
  bindingInvalid?: boolean
  bindingInvalidReason?: string
  sourceVerified?: boolean
  edition?: string
  permanent?: boolean
  domainMismatch?: boolean
}

export function storePayloadRebind(data: unknown) {
  if (!data || typeof data !== 'object') return false
  const row = data as { rebind?: boolean; reason?: string }
  if (row.rebind) return true
  return buyerTerminalReasons.has((row.reason || '').trim())
}

/** 源站已经确认这条绑定还能用，才允许提示已绑定并进入付款。 */
export function sourceConfirmedBound(account: PurchaseBindInput | null | undefined) {
  return (
    !!account &&
    !!account.sourceVerified &&
    !!account.bound &&
    !account.explicitRevoked &&
    !account.bindingInvalid &&
    !account.domainMismatch
  )
}

export function shouldAnnounceBound(account: PurchaseBindInput | null | undefined) {
  if (!sourceConfirmedBound(account)) return false
  if (account?.edition === 'commercial' && account.permanent) return false
  return true
}

export function purchaseNeedsRebind(
  account: PurchaseBindInput | null | undefined,
  rebindRequired = false
) {
  if (rebindRequired) return true
  if (!account?.bound) return true
  return !!account.explicitRevoked || !!account.bindingInvalid
}

/** 第一次绑定不提示「已被删除」。只有源站判定失效时才说明原因。 */
export function purchaseRebindNotice(
  account: PurchaseBindInput | null | undefined,
  rebindRequired = false
) {
  const invalid = !!account?.bindingInvalid || !!account?.explicitRevoked || rebindRequired
  if (!invalid) return ''
  if (account?.bindingInvalidReason === 'token_invalid') return buyerTokenInvalidMessage
  return buyerRebindMessage
}

export function logoutFailureIsAlreadyGone(error: unknown) {
  if (!error || typeof error !== 'object') return false
  const row = error as { data?: unknown; message?: string }
  if (payloadAsksRebind(row.data)) return true
  const message = row.message || ''
  return (
    message.includes(buyerRebindMessage) ||
    message.includes(buyerTokenInvalidMessage) ||
    message.includes('绑定不存在') ||
    message.includes('绑定令牌已失效')
  )
}

function payloadAsksRebind(data: unknown) {
  if (storePayloadRebind(data)) return true
  if (!data || typeof data !== 'object' || !('data' in data)) return false
  return storePayloadRebind((data as { data?: unknown }).data)
}
