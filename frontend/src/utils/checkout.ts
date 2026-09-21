export type CheckoutPayload = {
  orderNo?: string
  payUrl?: string
  qrCode?: string
  checkoutMode?: string
  amount?: string | number
}

export function isQrCheckout(data: CheckoutPayload | null | undefined): boolean {
  if (!data) return false
  return data.checkoutMode === 'qrcode' || Boolean(data.qrCode)
}
