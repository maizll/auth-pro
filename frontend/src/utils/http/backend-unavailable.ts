/** 后端连不上时跳转的静态说明页，随前端构建产物发布。 */
export const BACKEND_UNAVAILABLE_PAGE = '/backend-unavailable.html'

/** 连续这么多次网络错误或 502/503/504 后离开当前页。 */
export const BACKEND_UNAVAILABLE_THRESHOLD = 3

const unreachableStatuses = new Set([502, 503, 504])

let redirectPaused = false

export function isBackendUnreachableFailure(status: number | undefined, hasResponse: boolean): boolean {
  if (!hasResponse) return true
  return status != null && unreachableStatuses.has(status)
}

export class BackendUnreachableTracker {
  private consecutive = 0
  private readonly threshold: number

  constructor(threshold = BACKEND_UNAVAILABLE_THRESHOLD) {
    this.threshold = threshold
  }

  /** 返回 true 表示已经达到跳转阈值。 */
  record(unreachable: boolean): boolean {
    if (!unreachable) {
      this.consecutive = 0
      return false
    }
    this.consecutive += 1
    return this.consecutive >= this.threshold
  }

  reset(): void {
    this.consecutive = 0
  }
}

export const backendUnreachableTracker = new BackendUnreachableTracker()

/** 在线更新重启期间暂停跳转，让更新页自己轮询到超时。 */
export function setBackendUnreachableRedirectPaused(paused: boolean): void {
  redirectPaused = paused
  if (paused) backendUnreachableTracker.reset()
}

export function isBackendUnreachableRedirectPaused(): boolean {
  return redirectPaused
}

export function shouldRedirectToBackendUnavailable(pagePath: string): boolean {
  if (redirectPaused) return false
  const path = pagePath.split('?')[0].split('#')[0]
  return path !== BACKEND_UNAVAILABLE_PAGE && !path.endsWith('/backend-unavailable.html')
}
