/** 后端连不上时跳转的静态说明页，随前端构建产物发布。 */
export const BACKEND_UNAVAILABLE_PAGE = '/backend-unavailable.html'

/** 跳转前记下原地址，说明页等后端恢复后回到这里。public/backend-unavailable.html 读同一个键。 */
const BACKEND_UNAVAILABLE_RETURN_KEY = 'auth-pro-unavailable-from'

/** 连续这么多次网络错误或 502/503/504 后离开当前页。 */
export const BACKEND_UNAVAILABLE_THRESHOLD = 3

const unreachableStatuses = new Set([502, 503, 504])

/**
 * 在线更新窗口：从点「立即更新」到新版本启动完成。
 * 这段时间后端会重启，502/503/504 和无响应都是正常现象：请求层不弹错误、不跳说明页，只显示「正在更新」卡片。
 * 记在 localStorage：重启中刷新页面、切到别的页面、同一浏览器开着的其他标签页都认得；
 * 超过 UPDATE_WINDOW_MAX_MS 自动失效，不会永远静音。
 */
interface OnlineUpdateWindow {
  jobId: string
  version: string
  since: number
}

export const UPDATE_WINDOW_KEY = 'auth-pro-update-window'
/** 比更新页的总超时（10 分钟）多留 5 分钟。 */
export const UPDATE_WINDOW_MAX_MS = 15 * 60 * 1000

let memoryWindow: OnlineUpdateWindow | null = null
/** localStorage 写不进去（隐私模式）时只能靠本页内存。 */
let storageWritable = true

function windowStorage(): Storage | null {
  try {
    return typeof window !== 'undefined' ? window.localStorage : null
  } catch {
    return null
  }
}

export function beginOnlineUpdateWindow(jobId: string, version: string, now = Date.now()): void {
  const current = onlineUpdateWindow(now)
  memoryWindow = {
    jobId,
    version,
    since: current && current.jobId === jobId ? current.since : now
  }
  backendUnreachableTracker.reset()
  try {
    windowStorage()?.setItem(UPDATE_WINDOW_KEY, JSON.stringify(memoryWindow))
    storageWritable = true
  } catch {
    storageWritable = false
  }
}

export function endOnlineUpdateWindow(): void {
  memoryWindow = null
  try {
    windowStorage()?.removeItem(UPDATE_WINDOW_KEY)
  } catch {
    // 忽略
  }
}

/** 当前的在线更新窗口；没有或已过期返回 null。 */
export function onlineUpdateWindow(now = Date.now()): OnlineUpdateWindow | null {
  // 优先读存储，别的标签页结束窗口后这里也跟着结束；存储不可用时退回本页内存。
  let current: OnlineUpdateWindow | null = memoryWindow
  const storage = windowStorage()
  if (storage && storageWritable) {
    try {
      const raw = storage.getItem(UPDATE_WINDOW_KEY)
      current = raw ? (JSON.parse(raw) as OnlineUpdateWindow) : null
    } catch {
      current = memoryWindow
    }
  }
  if (!current || !current.jobId || !(now - current.since < UPDATE_WINDOW_MAX_MS)) {
    if (current) endOnlineUpdateWindow()
    memoryWindow = null
    return null
  }
  memoryWindow = current
  return current
}

export function isBackendUnreachableFailure(
  status: number | undefined,
  hasResponse: boolean
): boolean {
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

/** 在线更新窗口内，连不上后端（无响应、502/503/504）不提示、不跳说明页。 */
export function quietDuringOnlineUpdate(status: number | undefined, hasResponse: boolean): boolean {
  return onlineUpdateWindow() !== null && isBackendUnreachableFailure(status, hasResponse)
}

/** 记下当前地址再跳到说明页。sessionStorage 不可用时只是回不到原页，不影响跳转。 */
export function rememberBackendUnavailableReturn(storage: Storage, location: Location): void {
  try {
    storage.setItem(
      BACKEND_UNAVAILABLE_RETURN_KEY,
      `${location.pathname}${location.search}${location.hash}`
    )
  } catch {
    // 隐私模式等情况下写不进去，忽略。
  }
}

export function shouldRedirectToBackendUnavailable(pagePath: string): boolean {
  if (onlineUpdateWindow() !== null) return false
  const path = pagePath.split('?')[0].split('#')[0]
  return path !== BACKEND_UNAVAILABLE_PAGE && !path.endsWith('/backend-unavailable.html')
}
