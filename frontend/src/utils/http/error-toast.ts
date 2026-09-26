/**
 * 错误提示只弹一次。
 *
 * 拦截器展示过的 HttpError 带 displayed=true，页面 catch 再调用 showCaughtError 时不再弹。
 * 同一条文案在短时间内的第二次 ElMessage.error 也会被丢掉，用来兜住还没改到的页面。
 */

const DEFAULT_DEDUPE_WINDOW_MS = 400

export function caughtErrorText(
  error: unknown,
  fallback: string,
  alreadyShown: boolean
): string | null {
  if (alreadyShown) return null
  const message = readErrorMessage(error)
  return message || fallback
}

export function errorAlreadyToasted(error: unknown): boolean {
  return !!error && typeof error === 'object' && (error as { displayed?: boolean }).displayed === true
}

export function claimErrorToast(
  message: string,
  now: number,
  ledger: Map<string, number>,
  windowMs = DEFAULT_DEDUPE_WINDOW_MS
): boolean {
  const previous = ledger.get(message)
  if (previous != null && now - previous < windowMs) return false
  ledger.set(message, now)
  return true
}

export function errorToastText(message: unknown): string {
  if (typeof message === 'string') return message
  if (message && typeof message === 'object' && 'message' in message) {
    const text = (message as { message?: unknown }).message
    return typeof text === 'string' ? text : ''
  }
  return ''
}

export function showCaughtError(error: unknown, fallback: string): void {
  const text = caughtErrorText(error, fallback, errorAlreadyToasted(error))
  if (!text) return
  ElMessage.error(text)
}

function readErrorMessage(error: unknown): string {
  if (error instanceof Error) return error.message.trim()
  if (error && typeof error === 'object' && 'message' in error) {
    const message = (error as { message?: unknown }).message
    if (typeof message === 'string') return message.trim()
  }
  return ''
}
